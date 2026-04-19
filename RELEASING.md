# Releasing Mivy Print Bridge

Guía paso a paso para publicar una nueva versión del Print Bridge.

---

## Flujo normal: release por tag

Es la forma recomendada — todo corre automático en GitHub Actions.

```bash
# 1. Asegúrate que todo esté commiteado y pusheado a main
git status

# 2. Crea un tag semver (v + major.minor.patch)
git tag v1.0.0
git push origin v1.0.0
```

Eso dispara `.github/workflows/release.yml` que en **~3-5 min**:

1. Compila `mivy-bridge.exe` con Go (Windows amd64, stripped)
2. Instala Inno Setup vía Chocolatey en el runner
3. Genera el installer `Mivy-PrintBridge-Setup-1.0.0.exe`
4. Si hay secrets de firma configurados → firma binario + installer
5. Calcula SHA-256
6. Crea la GitHub Release y sube el `.exe` + notas auto-generadas

Al terminar, la URL de descarga queda en:
`https://github.com/<org>/<repo>/releases/latest/download/Mivy-PrintBridge-Setup-<version>.exe`

Esa misma URL es la que consume el banner del frontend (`VITE_BRIDGE_URL`).

---

## Release manual (sin tag)

Desde la UI de GitHub → **Actions** → **Release** → **Run workflow**:

- **Version**: `1.0.1` (opcional — si lo dejas vacío usa fecha YYYY.MM.DD)
- **Prerelease**: marca si es beta/RC

---

## Setup de firma de código (opcional pero recomendado)

Sin firma, Windows muestra **SmartScreen: "Editor desconocido"** al ejecutar el installer. Los clientes pueden asustarse. Para quitarlo:

### 1. Obtener un certificado Authenticode

| Opción | Precio | SmartScreen | Setup |
|---|---|---|---|
| **Certificado OV** (Sectigo, DigiCert) | ~USD 250/año | Se limpia tras ~1 semana de uso | Medio |
| **Certificado EV** | ~USD 400/año | Limpio desde el día 1 | Complejo (HSM o nube) |
| **Azure Trusted Signing** | USD 10/mes | Rápido (~días) | Fácil (cloud) |

Para MVP: **OV** está bien. Para producción: **Azure Trusted Signing** es el mejor costo/beneficio.

### 2. Convertir el `.pfx` a base64 (para guardarlo como secret)

```powershell
# Codifica el .pfx a base64 (no ponas espacios ni saltos de línea)
$b64 = [Convert]::ToBase64String([IO.File]::ReadAllBytes("C:\ruta\a\tu-cert.pfx"))
$b64 | Set-Clipboard
# Ahora el clipboard tiene el string base64 listo para pegar en GitHub
```

### 3. Agregar secrets en GitHub

**Settings** → **Secrets and variables** → **Actions** → **New repository secret**:

| Nombre | Valor |
|---|---|
| `SIGNING_CERT_BASE64` | El string base64 del paso 2 |
| `SIGNING_CERT_PASSWORD` | La contraseña del `.pfx` |
| `SIGNING_TIMESTAMP_URL` | *(opcional)* `http://timestamp.digicert.com` por defecto |

El workflow detecta automáticamente si están presentes y activa el firmado. Si no están, sigue compilando sin firma (sin romper nada).

### 4. Verificar firma post-release

En tu PC, tras descargar el `.exe`:

```powershell
Get-AuthenticodeSignature .\Mivy-PrintBridge-Setup-1.0.0.exe | Format-List
```

Debe mostrar `Status: Valid` y el `SignerCertificate` con tu nombre.

---

## CI (validación en PRs)

`.github/workflows/ci.yml` corre en cada push y PR a `main`/`master`:

- `go vet ./...`
- Compila el binario
- Lanza el bridge y verifica que `/status` y `/printers` respondan
- Sube el `.exe` como artifact (7 días de retención)

Si un PR rompe el build, el tag no debería ni crearse.

---

## Versionado

Usamos **semver** estricto:

- `v1.0.0` → primera release estable
- `v1.1.0` → nuevo feature (ej. soporte para impresoras de otra marca)
- `v1.0.1` → bugfix (ej. reconexión USB mejorada)
- `v2.0.0` → breaking change (ej. API incompatible con frontend viejo)
- `v1.0.0-beta.1` → pre-release (marca `prerelease: true`)

---

## Post-release checklist

Tras una release exitosa:

- [ ] Descargar el `.exe` de la release y verificar que instala en un PC limpio
- [ ] Probar el flujo completo desde zenPOS (banner → download → modal → imprimir)
- [ ] Actualizar `VITE_BRIDGE_URL` en tu deploy si apuntaba a una versión fija (en general mejor usar `/releases/latest/download/...`)
- [ ] Anunciar a los clientes existentes si hay cambios importantes (cambio de puerto, reconfiguración requerida, etc.)

---

## Rollback

Si una release rompe algo en producción:

1. **No borres el tag** — rompes el historial y enlaces de descarga
2. Marca la release como **"This is not a current release"** en GitHub (pre-release)
3. Crea un hotfix tag nuevo (`v1.0.1`) con el fix
4. Los clientes con el banner visible descargan la versión nueva al volver a entrar a zenPOS

Si la versión vieja YA está en muchos PCs y causa daño (raro, pero posible):

- Publica un release `v1.0.2` que fuerce `-action=uninstall` al arranque y luego se auto-elimine
- Documenta en las release notes cómo removerlo manualmente vía `Settings → Apps → zenPOS Print Bridge → Desinstalar`

---

## Preguntas frecuentes

**¿Por qué Windows solo?**
Porque el 95% de los POS en Chile corren en PC Windows. Mac/Linux pueden agregarse después (el bridge es Go puro, compila cross-platform — solo hay que reemplazar `winspool.drv` por CUPS).

**¿Por qué no auto-updater?**
Es extra complejidad. Con banner + "Ya lo instalé" + releases claros, el cliente actualiza en ~30 segundos. Si llegamos a miles de instalaciones, lo evaluamos.

**¿Puedo probar una release antes de publicarla?**
Sí — pushea un tag `v1.2.0-rc.1` o usa **Run workflow** con `prerelease: true`. Los banners no mostrarán pre-releases si usas `/releases/latest/download/...` (GitHub excluye prereleases de "latest" automáticamente).
