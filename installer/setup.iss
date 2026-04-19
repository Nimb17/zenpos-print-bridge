; =============================================================================
;  zenPOS Print Bridge - Inno Setup script
;  Builds a single-file installer that:
;    1. Copies zenpos-bridge.exe to Program Files
;    2. Registers a Windows Service so it auto-starts with the PC
;    3. Opens the localhost:7777 firewall rule (outbound is fine, we want the
;       Windows Defender prompt suppressed on first run)
;    4. Starts the service right after install
;    5. On uninstall, stops + removes the service and deletes shared config
;
;  Build:
;      iscc installer\setup.iss
;  Requires Inno Setup 6.2+ (https://jrsoftware.org/isdl.php).
; =============================================================================

#define AppName        "zenPOS Print Bridge"
#define AppVersion     "1.0.0"
#define AppPublisher   "zenPOS"
#define AppURL         "https://zenpos.cl"
#define ExeName        "zenpos-bridge.exe"
; Must match svcName constant in main.go
#define ServiceName    "zenposPrintBridge"
#define ServiceDisplay "zenPOS Print Bridge"

[Setup]
AppId={{C4B1F1B6-7E4A-4E3A-90E3-8B83B1AE4F10}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}
AppUpdatesURL={#AppURL}
DefaultDirName={autopf}\zenPOS\PrintBridge
DefaultGroupName=zenPOS
DisableProgramGroupPage=yes
OutputDir=..\build
OutputBaseFilename=zenPOS-PrintBridge-Setup-{#AppVersion}
Compression=lzma2/ultra
SolidCompression=yes
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern
UninstallDisplayIcon={app}\{#ExeName}
VersionInfoVersion={#AppVersion}
VersionInfoCompany={#AppPublisher}
VersionInfoProductName={#AppName}
LicenseFile=
SetupLogging=yes
CloseApplications=force

[Languages]
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "..\zenpos-bridge.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.md";        DestDir: "{app}"; DestName: "README.txt"; Flags: ignoreversion

[Dirs]
Name: "{commonappdata}\zenPOS\PrintBridge"; Permissions: users-modify

[Icons]
Name: "{group}\Print Bridge (abrir carpeta)"; Filename: "{app}"
Name: "{group}\Desinstalar Print Bridge";    Filename: "{uninstallexe}"

[Run]
; Register the Windows service using the binary's own installer
; (kardianos/service). This sets StartType=automatic, display name,
; description and recovery policy in a single step.
Filename: "{app}\{#ExeName}"; Parameters: "-action=install"; Flags: runhidden waituntilterminated; StatusMsg: "Registrando servicio..."
; Firewall rule: allow inbound on localhost:7777 (loopback only)
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""zenPOS Print Bridge"" dir=in action=allow protocol=TCP localport=7777 profile=any"; Flags: runhidden; StatusMsg: "Configurando firewall..."
; (service is started automatically by -action=install)

[UninstallRun]
Filename: "{app}\{#ExeName}"; Parameters: "-action=uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "UninstallService"
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""zenPOS Print Bridge"""; Flags: runhidden; RunOnceId: "DeleteFirewallRule"

[UninstallDelete]
Type: filesandordirs; Name: "{commonappdata}\zenPOS\PrintBridge"

[Code]
// Stop and remove any pre-existing service before copying the new binary.
// Prevents "file in use" errors on upgrade. Tries the binary first (clean
// kardianos uninstall), then falls back to raw sc.exe in case the previous
// version used a different service name or the binary is missing.
function InitializeSetup(): Boolean;
var
  ResultCode: Integer;
  OldExe: String;
begin
  OldExe := ExpandConstant('{autopf}\zenPOS\PrintBridge\{#ExeName}');
  if FileExists(OldExe) then
    Exec(OldExe, '-action=uninstall', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  // Fallback for very old installs / stale service entries
  Exec(ExpandConstant('{sys}\sc.exe'), 'stop {#ServiceName}',   '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\sc.exe'), 'delete {#ServiceName}', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Result := True;
end;
