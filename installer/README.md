# Mivy Print Bridge — Installer

Este directorio contiene los archivos necesarios para generar el instalador `.exe` que se distribuye a los clientes.

## Qué hace el instalador

1. Copia `mivy-bridge.exe` a `C:\Program Files\Mivy\PrintBridge\`.
2. Crea la carpeta de configuración compartida en `C:\ProgramData\Mivy\PrintBridge\` con permisos de escritura para todos los usuarios.
3. Registra un **servicio de Windows** (`mivyPrintBridge`) que inicia automáticamente con el PC.
   - Si existe un install previo de `zenPOS Print Bridge`, se desinstala y limpia automáticamente.
4. Abre el puerto **TCP 7777** en el firewall local (solo loopback).
5. Arranca el servicio inmediatamente tras la instalación.

Al **desinstalar**, detiene el servicio, lo elimina, quita la regla de firewall y borra la config compartida.

---

## Requisitos para compilar

- **Go 1.22+** — [go.dev](https://go.dev/dl/)
- **Inno Setup 6.2+** — [jrsoftware.org](https://jrsoftware.org/isdl.php)
- *(Opcional)* **Windows SDK signtool.exe** para firmar el ejecutable y el instalador.

## Compilar

Desde la raíz del repo del bridge:

```powershell
# Build simple (sin firma)
.\installer\build.ps1

# Especificar versión
.\installer\build.ps1 -Version 1.1.0

# Firmar binario e instalador (recomendado para producción)
.\installer\build.ps1 `
    -Version 1.1.0 `
    -SignCert "C:\certs\mivy-codesign.pfx" `
    -SignPassword "tu-password"
```

El instalador queda en `.\build\Mivy-PrintBridge-Setup-<version>.exe`.

---

## Flujo de distribución

1. El cliente hace clic en **"Descargar Print Bridge"** desde el banner de cualquier app del ecosistema Mivy (zenPOS, MesaDigital…).
2. Esto lo lleva a la URL configurada en `VITE_BRIDGE_URL` (por defecto: última release en GitHub).
3. Descarga el `.exe`, lo ejecuta, acepta UAC.
4. En ~10 segundos el servicio ya está corriendo.
5. Vuelve a la app web → el botón "Ya lo instalé" refresca la detección → aparece el modal de configuración de impresora.
6. Mismo binario funciona para TODAS las apps del ecosistema — el cliente solo instala una vez.

---

## Configuración en producción

El instalador **no requiere** que el cliente toque nada:

- **Config path:** `C:\ProgramData\Mivy\PrintBridge\config.json` (se crea al primer `/configure`).
- **Service:** `mivyPrintBridge` — corre como `LocalSystem`, arranque automático.
- **Logs:** `C:\ProgramData\Mivy\PrintBridge\bridge.log`.
- **Puerto:** `7777` en loopback (`127.0.0.1`). El firewall solo permite conexiones locales.

---

## Firmado de código (para evitar SmartScreen)

Sin firma, Windows muestra **"Editor desconocido"** al ejecutar el instalador. Para producción necesitas un certificado **Authenticode**:

### Opciones
1. **Certificado OV** (~USD 250/año, ej. Sectigo, DigiCert). Toma ~1 semana en calentar la reputación.
2. **Certificado EV** (~USD 400/año). Reputación inmediata, sin SmartScreen desde el día 1. Requiere hardware HSM o nube.
3. **Azure Trusted Signing** (USD 10/mes). Nube, setup simple, reputación rápida.

Una vez con el certificado, el build script ya hace el sign:
```powershell
.\installer\build.ps1 -SignCert "ruta.pfx" -SignPassword "..."
```

---

## Alternativa: solo ejecutable sin installer

Si necesitas probar rápido sin crear el installer, solo distribuye `mivy-bridge.exe` y ejecútalo manualmente. **No** se registra como servicio — hay que relanzarlo en cada reinicio. Es útil para demos, no para producción.

---

## Troubleshooting

| Síntoma | Causa probable | Solución |
|---|---|---|
| ISCC.exe no encontrado | Inno Setup no instalado | Instalar desde jrsoftware.org |
| `signtool.exe not found` | Falta Windows SDK | Instalar Windows 10/11 SDK o pasar `-SignCert ""` |
| El servicio no arranca | Puerto 7777 ocupado | Cambiar `HTTPPort` en el config y reinstalar |
| SmartScreen bloquea el instalador | Binario sin firmar | Firmar con cert EV/OV o indicar al usuario "Más info → Ejecutar de todas formas" |
| El cliente ya tenía una versión vieja | Upgrade en curso | El script `InitializeSetup` detiene y borra el servicio viejo antes de copiar |
