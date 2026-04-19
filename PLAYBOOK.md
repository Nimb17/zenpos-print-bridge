# Local Bridge Playbook

> **Cómo conectar una SaaS web (corriendo en el navegador) con hardware local del cliente (impresoras, balanzas, lectores, cajones, básculas, etc.) en Windows — de forma robusta, segura y auto-instalable.**

Este documento es un **plano replicable** extraído del proyecto `zenpos-print-bridge`. Si necesitas llevar este mismo patrón a otro dominio (ej. un puente para lectores RFID, balanzas, POS bancarios, etc.), seguí esta guía paso a paso.

---

## 1. El problema

Un navegador web **no puede**:
- Hablar con impresoras USB/térmicas (WebUSB tiene gaps, WebSerial limita a COM, WebBluetooth es inestable)
- Acceder a cajones de dinero
- Leer balanzas serie
- Acceder a lectores específicos sin drivers

**Y sin embargo** tu SaaS vive en el navegador porque eso te da:
- Deploy instantáneo
- Multi-plataforma
- Sin instaladores para el core del producto
- Usuarios en móvil/tablet funcionan igual

**La solución clásica** (electron, PWA, extensión Chrome) rompe alguna de esas ventajas.

---

## 2. La solución: Local Bridge Pattern

Un **proceso liviano nativo** corre en la PC del cliente y expone una **API HTTP local** (`http://localhost:PORT`) que el navegador consume con `fetch`.

```
┌────────────────────────────────────────────┐
│  Browser (SaaS web en Vercel/Netlify/...)  │
│  ┌──────────────────────────────────────┐  │
│  │   React + service + hook + modal     │  │
│  └────────────────┬─────────────────────┘  │
└───────────────────┼────────────────────────┘
                    │ HTTPS → https://tu-app.com
                    │ fetch('http://localhost:7777')  ← cross-origin OK si el bridge manda CORS *
                    ▼
┌────────────────────────────────────────────┐
│  Local PC del cliente (Windows 10/11)      │
│  ┌──────────────────────────────────────┐  │
│  │  Bridge (Go, ~6MB, Windows Service)  │  │
│  │  - HTTP server :7777                 │  │
│  │  - Device discovery                  │  │
│  │  - Config persistida                 │  │
│  │  - Auto-arranca con Windows          │  │
│  └────────────────┬─────────────────────┘  │
│                   │ USB / Serial / TCP     │
│                   ▼                        │
│           ┌───────────────┐                │
│           │   Hardware    │                │
│           │ (impresora /  │                │
│           │  lector /     │                │
│           │  balanza...)  │                │
│           └───────────────┘                │
└────────────────────────────────────────────┘
```

### Ventajas

- **La SaaS sigue siendo browser-first** — tablet/móvil funcionan sin el bridge (con fallback WebBluetooth o sin hardware)
- **Un único install** — el cliente corre el `.exe` una vez y se olvida
- **Cero configuración de red** — todo es `localhost:7777`, no hay firewall corporativo que lo bloquee
- **Deploy independiente** — actualizas la SaaS sin tocar el bridge (y viceversa)
- **Cross-origin libre** — CORS `*` en loopback es seguro porque sólo el browser del mismo PC puede alcanzarlo

### Desventajas (sé consciente)

- **Sólo Windows por default** — Mac/Linux requieren re-target (Go compila cross-platform pero APIs nativas cambian)
- **SmartScreen en el primer install** — sin cert firmado, Windows asusta al usuario con "editor desconocido" hasta ~10 instalaciones o hasta que firmes
- **Upgrade del bridge = cliente baja un .exe** — no es auto-update sin extra trabajo

---

## 3. Stack tecnológico (y por qué)

| Capa | Elección | Por qué |
|---|---|---|
| Bridge lang | **Go** | Binario único, cross-compile trivial, stdlib rica (HTTP, serial, TCP), sin runtime |
| Service mgr | **kardianos/service** | Abstrae Windows Service / launchd / systemd con una API |
| HTTP server | **stdlib `net/http`** | No necesitas gorilla; `net/http` es suficiente y cero deps |
| Installer | **Inno Setup 6** | Gratis, estándar en Windows, genera `.exe` único, compilable en CI |
| CI/CD | **GitHub Actions** | Runners Windows gratis, integra con releases, Chocolatey disponible |
| Frontend | **React + TypeScript** | Ya estaba en zenPOS; el patrón funciona igual con Vue/Svelte |
| Signing | **Authenticode (OV/EV)** o **Azure Trusted Signing** | Necesario para production, opcional para MVP |

---

## 4. Layout de repos

Mantené **dos repos separados** (o un monorepo con 2 carpetas top-level):

```
tu-org/
├── tu-saas/                    ← frontend + backend de tu producto
│   ├── components/
│   │   ├── HardwareBanner.tsx          ← banner descarga
│   │   ├── HardwareSetupModal.tsx      ← modal config
│   │   └── ...
│   ├── services/
│   │   └── hardwareService.ts          ← cliente HTTP del bridge
│   ├── src/hooks/
│   │   └── useBridgeDetection.ts       ← hook SO + ping
│   └── ...
│
└── tu-saas-bridge/             ← Puente local
    ├── main.go                         ← entry + service setup
    ├── config/config.go                ← persistencia en %PROGRAMDATA%
    ├── server/server.go                ← HTTP handlers
    ├── <dominio>/                      ← lógica del hardware (printer/, scale/, scanner/)
    ├── installer/
    │   ├── setup.iss                   ← Inno Setup
    │   ├── build.ps1                   ← build script
    │   └── README.md
    ├── .github/workflows/
    │   ├── ci.yml                      ← tests en PRs
    │   └── release.yml                 ← build + release por tag
    ├── go.mod
    ├── README.md
    └── RELEASING.md
```

**Por qué separados**: el ciclo de release del bridge (meses) es muy distinto al de la SaaS (días). Mezclarlos obliga a redeployar todo por cualquier cambio.

---

## 5. PARTE 1 — Construir el bridge (Go)

### 5.1 Inicializar

```powershell
mkdir tu-saas-bridge
cd tu-saas-bridge
go mod init github.com/tu-org/tu-saas-bridge
go get github.com/kardianos/service
# Si vas a usar puertos serie:
go get go.bug.st/serial
```

### 5.2 `main.go` (plantilla)

```go
package main

import (
    "flag"
    "fmt"
    "log"
    "os"
    "path/filepath"

    "github.com/kardianos/service"
    "github.com/tu-org/tu-saas-bridge/config"
    srv "github.com/tu-org/tu-saas-bridge/server"
    dom "github.com/tu-org/tu-saas-bridge/<dominio>"
)

const (
    svcName        = "tuSaasBridge"          // sin espacios, sin mayúsculas en el start
    svcDisplayName = "Tu SaaS Bridge"
    svcDescription = "Permite que tu-saas hable con hardware local."
)

type program struct {
    cfg     *config.Config
    manager *dom.Manager
    server  *srv.Server
}

func (p *program) Start(s service.Service) error { go p.run(); return nil }

func (p *program) run() {
    // Reconectar al hardware configurado la última vez
    if err := p.manager.RestoreFrom(p.cfg); err != nil {
        log.Printf("advertencia: no se pudo restaurar hardware: %v", err)
    }
    if err := p.server.Start(); err != nil {
        log.Printf("servidor HTTP terminó: %v", err)
    }
}

func (p *program) Stop(s service.Service) error {
    p.server.Stop()
    p.manager.Disconnect()
    return nil
}

func setupLogger() {
    base := os.Getenv("PROGRAMDATA")
    if base == "" { base = os.Getenv("APPDATA") }
    if base == "" { return }
    dir := filepath.Join(base, "TuSaaS", "Bridge")
    os.MkdirAll(dir, 0755)
    f, err := os.OpenFile(filepath.Join(dir, "bridge.log"),
        os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
    if err == nil { log.SetOutput(f) }
}

func main() {
    action := flag.String("action", "", "install | uninstall | run")
    flag.Parse()

    log.SetFlags(log.LstdFlags | log.Lmsgprefix)
    log.SetPrefix("[bridge] ")
    if !service.Interactive() { setupLogger() }

    cfg, _ := config.Load()
    mgr := dom.NewManager(cfg)
    server := srv.New(mgr, cfg)
    prg := &program{cfg: cfg, manager: mgr, server: server}

    s, err := service.New(prg, &service.Config{
        Name:        svcName,
        DisplayName: svcDisplayName,
        Description: svcDescription,
        Option:      service.KeyValue{"StartType": "automatic"},
    })
    if err != nil { log.Fatalf("error creando servicio: %v", err) }

    switch *action {
    case "install":
        if err := s.Install(); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
        fmt.Println("Servicio instalado.")
        _ = s.Start()
    case "uninstall":
        _ = s.Stop()
        if err := s.Uninstall(); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
        fmt.Println("Servicio desinstalado.")
    case "run", "":
        if service.Interactive() {
            fmt.Printf("Tu SaaS Bridge — http://localhost:%d\n", cfg.HTTPPort)
        }
        if err := s.Run(); err != nil { log.Fatalf("%v", err) }
    default:
        fmt.Fprintf(os.Stderr, "Acción desconocida: %s\n", *action); os.Exit(1)
    }
}
```

### 5.3 `config/config.go` (template)

Regla de oro: **config compartida en `%PROGRAMDATA%`** con fallback a `%APPDATA%`. Esto hace que cualquier usuario de Windows en la misma máquina (cajero 1, cajero 2) vea la misma config.

```go
package config

import (
    "encoding/json"
    "os"
    "path/filepath"
)

type Config struct {
    HTTPPort int    `json:"httpPort"`
    // ... tus campos de dominio aquí
}

func DefaultConfig() *Config { return &Config{HTTPPort: 7777} }

func configDir() (string, error) {
    if pd := os.Getenv("PROGRAMDATA"); pd != "" {
        dir := filepath.Join(pd, "TuSaaS", "Bridge")
        if err := os.MkdirAll(dir, 0755); err == nil {
            probe := filepath.Join(dir, ".writetest")
            if err := os.WriteFile(probe, []byte{}, 0644); err == nil {
                os.Remove(probe); return dir, nil
            }
        }
    }
    ad := os.Getenv("APPDATA")
    if ad == "" { h, _ := os.UserHomeDir(); ad = h }
    dir := filepath.Join(ad, "TuSaaS", "Bridge")
    return dir, os.MkdirAll(dir, 0755)
}

func Load() (*Config, error) {
    dir, err := configDir()
    if err != nil { return DefaultConfig(), nil }
    data, err := os.ReadFile(filepath.Join(dir, "config.json"))
    if err != nil { return DefaultConfig(), nil }
    cfg := DefaultConfig()
    _ = json.Unmarshal(data, cfg)
    if cfg.HTTPPort == 0 { cfg.HTTPPort = 7777 }
    return cfg, nil
}

func (c *Config) Save() error {
    dir, err := configDir()
    if err != nil { return err }
    data, _ := json.MarshalIndent(c, "", "  ")
    return os.WriteFile(filepath.Join(dir, "config.json"), data, 0644)
}
```

### 5.4 `server/server.go` (template)

Puntos clave:
- **Escucha SOLO en `127.0.0.1`** (nunca `0.0.0.0`) — previene que otro PC de la LAN acceda
- **CORS `*`** — está bien porque SOLO el browser del mismo PC puede alcanzar loopback
- **Timeout cortos** — si el bridge se cuelga, el browser no queda bloqueado

```go
package server

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/tu-org/tu-saas-bridge/config"
    dom "github.com/tu-org/tu-saas-bridge/<dominio>"
)

type Server struct {
    manager *dom.Manager
    cfg     *config.Config
    http    *http.Server
}

func New(mgr *dom.Manager, cfg *config.Config) *Server { return &Server{manager: mgr, cfg: cfg} }

func (s *Server) Start() error {
    mux := http.NewServeMux()
    mux.HandleFunc("/status",    s.cors(s.handleStatus))
    mux.HandleFunc("/devices",   s.cors(s.handleDevices))
    mux.HandleFunc("/configure", s.cors(s.handleConfigure))
    mux.HandleFunc("/action",    s.cors(s.handleAction))

    s.http = &http.Server{
        Addr:         fmt.Sprintf("127.0.0.1:%d", s.cfg.HTTPPort),
        Handler:      mux,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  60 * time.Second,
    }
    return s.http.ListenAndServe()
}

func (s *Server) Stop() { if s.http != nil { _ = s.http.Close() } }

func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        if r.Method == http.MethodOptions { w.WriteHeader(http.StatusNoContent); return }
        next(w, r)
    }
}

func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
    writeJSON(w, status, map[string]string{"error": msg})
}
```

### 5.5 Device discovery (Windows API)

El "truco" que te ahorra horas de pelea: **enumerar los devices nativos con `syscall` en Windows** en lugar de obligar al usuario a saber su COM port.

Para impresoras, `EnumPrintersW` via `winspool.drv`. Para otros devices hay `SetupAPI` (para enumerar USB genéricos), `WMI` (para todo), `AddrOf` serial ports.

**Patrón general**:

```go
// windows_discover.go
//go:build windows

package dom

import (
    "syscall"
    "unsafe"
)

var (
    modWinspool       = syscall.NewLazyDLL("winspool.drv")
    procEnumPrintersW = modWinspool.NewProc("EnumPrintersW")
)

// ListInstalled returns installed Windows printers. Pattern applies to any
// Windows enumeration API: call once with nil to get size, alloc, call again.
func ListInstalled() ([]Device, error) {
    const PRINTER_ENUM_LOCAL = 2
    const level = 2
    var needed, returned uint32

    // First call: get size
    procEnumPrintersW.Call(
        uintptr(PRINTER_ENUM_LOCAL), 0, uintptr(level),
        0, 0,
        uintptr(unsafe.Pointer(&needed)),
        uintptr(unsafe.Pointer(&returned)),
    )
    if needed == 0 { return nil, nil }

    // Second call: fill buffer
    buf := make([]byte, needed)
    ret, _, _ := procEnumPrintersW.Call(
        uintptr(PRINTER_ENUM_LOCAL), 0, uintptr(level),
        uintptr(unsafe.Pointer(&buf[0])), uintptr(needed),
        uintptr(unsafe.Pointer(&needed)),
        uintptr(unsafe.Pointer(&returned)),
    )
    if ret == 0 { return nil, syscall.GetLastError() }

    // Parse returned structs (layout depends on level)
    // ... interpretá `buf` según la struct PRINTER_INFO_2
    return devices, nil
}
```

**Heurística `isLikely`**: cuando detectas hardware, marca cuáles son "probablemente lo que el usuario quiere". Filtrá por keywords: impresoras térmicas suelen tener `POS`, `THERMAL`, `58`, `80`, `RAW`. Virtuales como `OneNote`, `PDF`, `XPS`, `Fax` → descarta.

### 5.6 Endpoints mínimos

Tu bridge debe exponer **como mínimo**:

| Endpoint | Método | Para qué |
|---|---|---|
| `/status` | GET | ping + qué device está conectado |
| `/devices` | GET | lista de hardware detectado (para el modal) |
| `/configure` | POST | guardar el device elegido + reconectar |
| `/action` | POST | la acción de dominio (imprimir / leer / pesar / abrir cajón) |

**Buena práctica**: `/status` debe responder en **<1s siempre**, aunque el hardware esté muerto. El frontend lo usa para detectar si el bridge está vivo. Si bloquea esperando el device, rompes la UX.

---

## 6. PARTE 2 — Integración frontend

### 6.1 Service class (`services/hardwareService.ts`)

Capa HTTP que encapsula el bridge. **Expón tipos TS explícitos** que reflejen la API del bridge — esto es tu contrato cross-repo.

```ts
const BRIDGE_URL = 'http://localhost:7777';
const BRIDGE_TIMEOUT_MS = 800;

export interface BridgeDevice { name: string; port: string; isLikely: boolean; }
export interface BridgeStatus { connected: boolean; deviceName: string; version: string; }
export interface ConfigureInput { type: 'usb' | 'serial' | 'network'; port?: string; ip?: string; }

export const listDevices = async (): Promise<BridgeDevice[]> => {
  const res = await fetch(`${BRIDGE_URL}/devices`, { signal: AbortSignal.timeout(3000) });
  if (!res.ok) throw new Error('Bridge no respondió');
  return res.json();
};

export const getBridgeStatus = async (): Promise<BridgeStatus> => {
  const res = await fetch(`${BRIDGE_URL}/status`, { signal: AbortSignal.timeout(BRIDGE_TIMEOUT_MS) });
  if (!res.ok) throw new Error('Bridge no respondió');
  return res.json();
};

export const configureDevice = async (input: ConfigureInput): Promise<BridgeStatus> => {
  const res = await fetch(`${BRIDGE_URL}/configure`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
    signal: AbortSignal.timeout(5000),
  });
  const body = await res.json().catch(() => ({ error: 'Respuesta inválida' }));
  if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
  return body;
};

export const isBridgeRunning = async (): Promise<boolean> => {
  try {
    const res = await fetch(`${BRIDGE_URL}/status`, { signal: AbortSignal.timeout(BRIDGE_TIMEOUT_MS) });
    return res.ok;
  } catch { return false; }
};
```

### 6.2 Detection hook (`src/hooks/useBridgeDetection.ts`)

Detecta SO + si el bridge corre. Clave: **re-probea cuando la pestaña gana foco** para detectar cuando el usuario acaba de instalar el bridge.

```ts
import { useCallback, useEffect, useState } from 'react';
import { isBridgeRunning } from '../services/hardwareService';

export type OSKind = 'windows' | 'mac' | 'linux' | 'android' | 'ios' | 'unknown';

const detectOS = (): OSKind => {
  const ua = navigator.userAgent.toLowerCase();
  const platform = ((navigator as any).userAgentData?.platform || navigator.platform || '').toLowerCase();
  if (/android/.test(ua)) return 'android';
  if (/iphone|ipad|ipod/.test(ua)) return 'ios';
  if (/win/.test(platform)) return 'windows';
  if (/mac/.test(platform)) return 'mac';
  if (/linux/.test(platform)) return 'linux';
  return 'unknown';
};

export const useBridgeDetection = () => {
  const [os] = useState(() => detectOS());
  const [bridgeRunning, setBridgeRunning] = useState(false);
  const [loading, setLoading] = useState(true);

  const probe = useCallback(async () => {
    setBridgeRunning(await isBridgeRunning());
    setLoading(false);
  }, []);

  useEffect(() => {
    probe();
    const onFocus = () => probe();
    window.addEventListener('focus', onFocus);
    return () => window.removeEventListener('focus', onFocus);
  }, [probe]);

  return {
    os, bridgeRunning, loading, recheck: probe,
    canInstallBridge: os === 'windows' && !bridgeRunning,
  };
};
```

### 6.3 Download banner (`components/BridgeDownloadBanner.tsx`)

Muestra el banner **solo** en Windows desktop sin bridge. Mobile/Mac/Linux → invisible o nota discreta. Persiste dismiss en `localStorage`.

URL de descarga desde env:
```ts
const DEFAULT_DOWNLOAD_URL = import.meta.env.VITE_BRIDGE_URL
  || 'https://github.com/tu-org/tu-saas-bridge/releases/latest';
```

Dos botones: **"Descargar"** (abre la release de GitHub) y **"Ya lo instalé"** (re-probe).

### 6.4 Setup modal (`components/BridgeSetupModal.tsx`)

3 tabs típicos:
1. **Instalada en Windows** — lista de `/devices` filtrada por `isLikely` + checkbox "Mostrar todas"
2. **Puerto COM** — selector + baud rate
3. **Red** — IP + puerto TCP

Clave UX:
- **Auto-abre** cuando el bridge corre pero no tiene device configurado (primer uso)
- **"Imprimir/Ejecutar prueba"** en el footer — crítico para confianza del usuario
- Mensaje de error visible + botón "¿Tu dispositivo no aparece?" con checklist

### 6.5 Integración en la pantalla relevante

En la pantalla donde el usuario usa el hardware:

```tsx
<BridgeDownloadBanner compact />
{bridgeRunning && !deviceConfigured && <BridgeSetupModal open onConfigured={…} />}
<button onClick={doAction}>Ejecutar acción</button>
```

---

## 7. PARTE 3 — Installer (Inno Setup)

### 7.1 `installer/setup.iss` — template

```ini
#define AppName        "Tu SaaS Bridge"
#define AppVersion     "1.0.0"
#define ExeName        "tu-saas-bridge.exe"
#define ServiceName    "tuSaasBridge"   ; DEBE coincidir con svcName en main.go

[Setup]
AppId={{PONER-UN-GUID-UNICO-AQUI}}     ; Generá uno con [guid]::NewGuid() en PS
AppName={#AppName}
AppVersion={#AppVersion}
DefaultDirName={autopf}\TuSaaS\Bridge
OutputDir=..\build
OutputBaseFilename=TuSaaS-Bridge-Setup-{#AppVersion}
Compression=lzma2/ultra
SolidCompression=yes
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern
CloseApplications=force

[Languages]
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"

[Files]
Source: "..\{#ExeName}"; DestDir: "{app}"; Flags: ignoreversion

[Dirs]
; Carpeta compartida de config con permisos de escritura para todos
Name: "{commonappdata}\TuSaaS\Bridge"; Permissions: users-modify

[Run]
Filename: "{app}\{#ExeName}"; Parameters: "-action=install"; Flags: runhidden waituntilterminated; StatusMsg: "Registrando servicio..."
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""{#AppName}"" dir=in action=allow protocol=TCP localport=7777 profile=any"; Flags: runhidden

[UninstallRun]
Filename: "{app}\{#ExeName}"; Parameters: "-action=uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "UninstallService"
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""{#AppName}"""; Flags: runhidden; RunOnceId: "DeleteFirewallRule"

[UninstallDelete]
Type: filesandordirs; Name: "{commonappdata}\TuSaaS\Bridge"

[Code]
; Limpieza pre-install para upgrades sin reiniciar
function InitializeSetup(): Boolean;
var ResultCode: Integer; OldExe: String;
begin
  OldExe := ExpandConstant('{autopf}\TuSaaS\Bridge\{#ExeName}');
  if FileExists(OldExe) then
    Exec(OldExe, '-action=uninstall', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\sc.exe'), 'stop {#ServiceName}',   '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\sc.exe'), 'delete {#ServiceName}', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Result := True;
end;
```

**Clave**: registrar el servicio con `tu-saas-bridge.exe -action=install` (kardianos hace todo bien) en lugar de `sc.exe create` a mano. Evita bugs de service name mismatch.

### 7.2 `installer/build.ps1`

Ver el de este repo — se adapta cambiando nombres. Flujo:
1. `go build -trimpath -ldflags "-s -w -X main.version=$Version"`
2. (opcional) `signtool sign /f $cert /p $pw ...`
3. `ISCC.exe installer/setup.iss`
4. (opcional) `signtool sign` sobre el installer
5. Output en `.\build\`

---

## 8. PARTE 4 — GitHub Actions

### 8.1 `.github/workflows/ci.yml` — validación en PRs

```yaml
name: CI
on:
  push:   { branches: [main] }
  pull_request: { branches: [main] }
jobs:
  build:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22', cache: true }
      - run: go vet ./...
      - shell: pwsh
        run: |
          $env:CGO_ENABLED="0"; $env:GOOS="windows"; $env:GOARCH="amd64"
          go build -trimpath -ldflags "-s -w" -o bridge.exe .
      - shell: pwsh
        run: |
          $p = Start-Process .\bridge.exe -PassThru
          Start-Sleep 3
          $s = Invoke-RestMethod http://localhost:7777/status -TimeoutSec 5
          if (-not $s) { throw "No status" }
          Stop-Process -Id $p.Id -Force
```

### 8.2 `.github/workflows/release.yml` — build + release por tag

Ver `.github/workflows/release.yml` en este repo. Template resumido:

```yaml
name: Release
on:
  push: { tags: ['v*.*.*'] }
  workflow_dispatch:
    inputs: { version: { type: string } }
permissions: { contents: write }
jobs:
  build:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - shell: pwsh
        run: choco install innosetup --no-progress -y
      - shell: pwsh
        run: .\installer\build.ps1 -Version "${{ steps.version.outputs.v }}"
      - uses: softprops/action-gh-release@v2
        with:
          tag_name: v${{ steps.version.outputs.v }}
          files: build/*.exe
          generate_release_notes: true
```

### 8.3 Signing en CI (opcional pero recomendado)

Secrets en GitHub repo settings:
- `SIGNING_CERT_BASE64` = `[Convert]::ToBase64String([IO.File]::ReadAllBytes("cert.pfx"))`
- `SIGNING_CERT_PASSWORD` = password del PFX

El workflow detecta si existen y firma automáticamente. Sin secrets → build sin firma (funcional pero SmartScreen muestra "Editor desconocido").

---

## 9. Checklist replicable (copiar-pegar y tachar)

### Fase A: Bridge (1-2 días)

- [ ] Crear repo `tu-saas-bridge` en GitHub
- [ ] `go mod init` + deps (`kardianos/service`, `go.bug.st/serial` si aplica)
- [ ] Copiar `main.go`, `config/config.go`, `server/server.go` de este playbook
- [ ] Implementar el paquete `<dominio>/` con tu lógica de hardware
- [ ] Implementar device discovery nativo (EnumPrinters / SetupDiEnumDeviceInfo / WMI según corresponda)
- [ ] Implementar endpoints `/status`, `/devices`, `/configure`, `/action`
- [ ] Probar localmente: `go run . -action=run` → `curl localhost:7777/status`
- [ ] `go build` → ~6 MB, sin deps externas

### Fase B: Frontend (1 día)

- [ ] Crear `services/hardwareService.ts` con tipos + funciones HTTP
- [ ] Crear `src/hooks/useBridgeDetection.ts`
- [ ] Crear `components/BridgeDownloadBanner.tsx`
- [ ] Crear `components/BridgeSetupModal.tsx` con tabs + test action
- [ ] Integrar en la pantalla relevante (auto-open modal si bridge corre pero no hay device)
- [ ] Setear `VITE_BRIDGE_URL` en `.env.local` (apuntar a `http://localhost:7777` no es necesario — el BRIDGE_URL está en el service)

### Fase C: Installer (½ día)

- [ ] Crear `installer/setup.iss` adaptando el template (cambiar AppId, nombres)
- [ ] Crear `installer/build.ps1`
- [ ] Instalar Inno Setup local y correr `.\installer\build.ps1`
- [ ] Validar en una PC limpia (VM Windows si tenés): el servicio arranca, el frontend lo detecta, el modal funciona

### Fase D: CI/CD (½ día)

- [ ] Crear `.github/workflows/ci.yml`
- [ ] Crear `.github/workflows/release.yml`
- [ ] Push a main → ver que el CI pasa
- [ ] `git tag v1.0.0 && git push --tags` → ver la release generada
- [ ] Descargar el `.exe` de la release → instalar → probar end-to-end

### Fase E: Producción (½ día)

- [ ] Setear `VITE_BRIDGE_URL=https://github.com/tu-org/tu-saas-bridge/releases/latest` en el deploy
- [ ] Redeploy la SaaS
- [ ] En una PC real del cliente: click banner → descarga → install → volver a la SaaS → modal → config → prueba → OK

### Fase F: Hardening (opcional, cuando tengas 10+ clientes)

- [ ] Conseguir certificado Authenticode (OV ~USD 250/año o Azure Trusted Signing ~USD 10/mes)
- [ ] Guardar secrets en GitHub
- [ ] Release firmado → SmartScreen deja de asustar
- [ ] (opcional) auto-updater con `github.com/inconshreveable/go-update` o dejar que el banner haga su trabajo

---

## 10. Pitfalls & troubleshooting

### Bridge

**Puerto 7777 ya en uso.**
Docker Desktop, algún otro dev server, o el propio bridge corriendo duplicado. Para resolución: permití override vía env (`BRIDGE_HTTP_PORT`) o config. Default 7777 es arbitrario — cambialo si choca con algo común en tu sector.

**`EnumPrinters` devuelve vacío.**
El servicio corre bajo `LocalSystem` que tiene su propia sesión; las impresoras instaladas "para el usuario actual" no se ven. Solución: instalar la impresora "para todos los usuarios" (checkbox en el wizard de Windows) o usar `EnumPrinters` con `PRINTER_ENUM_CONNECTIONS`.

**El service no arranca y no hay log.**
Probablemente `setupLogger` falló silenciosamente. Debuggeá corriendo `zenpos-bridge.exe -action=run` interactivo primero — así ves los logs en stdout. Cuando funcione interactivo, registrá como servicio.

**CORS falla en el browser.**
Asegurate que el handler responde al **OPTIONS preflight** (Method check en el middleware). Si fetchas con `Content-Type: application/json`, el browser dispara OPTIONS antes del POST.

### Frontend

**Bridge detection siempre falla después de instalar.**
El user instaló el bridge, pero el hook no lo detecta. Casos típicos:
- El listener `focus` no dispara (la pestaña nunca perdió foco). Agregá `visibilitychange` también.
- Hay un Service Worker cacheando la respuesta de 404. Excluí `localhost:7777` del SW.
- El antivirus bloqueó el `.exe`. Decirle al usuario que lo permita en Defender.

**El modal se abre en loop.**
El `useEffect` que auto-abre depende de un state que cambia cuando el modal se abre. Usá un `useRef` o un flag `setupPromptShown` para disparar **una sola vez**.

### Installer

**SmartScreen bloquea el `.exe`.**
Solución temporal para MVP: decirle al usuario "Más info → Ejecutar de todas formas". Solución real: firmar con cert EV o Azure Trusted Signing.

**Service no se registra.**
99% es que el `.exe` del installer no está corriendo como admin. Inno Setup lo pide vía `PrivilegesRequired=admin` pero si el user canceló el UAC, queda instalado sin servicio. El `[Run]` falla silencioso. Agregá verificación en `AfterInstall`.

**Upgrade en caliente falla con "file in use".**
El servicio viejo sigue corriendo cuando Inno intenta sobreescribir el `.exe`. Solución: `InitializeSetup` detiene y desregistra el servicio **antes** de copiar archivos (ver template arriba).

### CI/CD

**El CI smoke test falla con timeout.**
El runner Windows es lento los primeros segundos. Aumentá el `Start-Sleep 3` a 5, o usá un retry loop contra `/status`.

**Chocolatey install de Inno Setup falla.**
Ocasionalmente timeouts. Solución: `--timeout 300` o cachear el installer en el artifact. Rara vez hace falta.

### Producción

**Clientes se quejan de que "el botón imprimir no hace nada".**
Causa 99%: instalaron el bridge, pero no abrieron el modal para configurar qué impresora usar. Solución: banner post-install "Bridge instalado. Elegí tu impresora" que abre el modal a la fuerza.

**El cliente tiene 2 cajeros en el mismo PC y cada uno quiere su config.**
Si vos mantenés config en `%PROGRAMDATA%` (compartida), ambos ven lo mismo. Si necesitás config por usuario, mové a `%APPDATA%` y asumí que el servicio debe correr bajo la cuenta del usuario, no `LocalSystem`. Trade-off.

---

## 11. Variaciones del patrón

**Para Mac/Linux**: Go compila cross-platform. El wrapper de service (kardianos) ya soporta `launchd` y `systemd`. Cambiá `winspool.drv` por CUPS (Mac/Linux imprimen todos vía CUPS) y el `.pkg` (Mac) o `.deb` (Linux) reemplaza al `.exe`. Pero sé realista: 95% de tus clientes en LatAm son Windows.

**Para Bluetooth clásico (BLE)**: el navegador tiene Web Bluetooth. No necesitás bridge. Pero tené en cuenta que BLE es inestable y no soporta gran throughput.

**Para lectores de código de barras**: la mayoría son HID (teclado emulado). Se leen directo en el `<input>` del form — no necesitás bridge.

**Para balanzas con protocolo no estándar**: bridge obligatorio. El patrón es el mismo — reemplazá `printer/` por `scale/` y expón `/read` que devuelve `{weight, unit, stable}`.

**Para cajones de dinero**: suelen colgar de la impresora (un comando ESC/POS los abre). Si ya tenés el bridge de impresora, agregá un endpoint `/open-drawer` que mande el comando — cero hardware extra.

---

## 12. Cuándo NO usar este patrón

- Si tu hardware tiene **WebUSB funcional** oficial (ej. Ledger): usá WebUSB, menos fricción.
- Si tu app puede ser **Electron**: evaluá si el install extra vale la pena. Con Electron ya tenés Node.js nativo dentro.
- Si tu SaaS es **100% móvil**: los clientes no tienen PC, el bridge no aplica.
- Si el hardware es **propio** (fabricado por vos): mejor un driver dedicado que soportes directamente.

---

## 13. Créditos

Patrón extraído de `zenpos-print-bridge` — un puente para impresoras térmicas ESC/POS desde zenPOS (SaaS web de punto de venta).

Componentes reutilizables:
- Bridge Go con kardianos/service
- Auto-discovery vía Windows API
- Frontend React (service + hook + modal + banner)
- Inno Setup installer con firewall + autostart
- GitHub Actions con signing opcional

Tiempo total de construcción de la primera versión: **~3 días** de un dev familiarizado con Go + React. Replicar para otro dominio de hardware: **1-2 días** siguiendo este playbook.
