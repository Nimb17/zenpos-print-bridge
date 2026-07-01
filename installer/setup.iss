; =============================================================================
;  Mivy Print Bridge - Inno Setup script
;  Builds a single-file installer that:
;    1. Copies mivy-bridge.exe to Program Files
;    2. Registers a Windows Service so it auto-starts with the PC
;    3. Opens the localhost:7777 firewall rule (inbound loopback only)
;    4. Starts the service right after install
;    5. On uninstall, stops + removes the service and deletes shared config
;    6. On upgrade, cleans up any legacy zenPOS Print Bridge install automatically
;
;  Build:
;      iscc installer\setup.iss
;  Requires Inno Setup 6.2+ (https://jrsoftware.org/isdl.php).
; =============================================================================

#define AppName        "Mivy Print Bridge"
; AppVersion se puede sobrescribir desde la línea de comandos con /DAppVersion=x.y.z
; (build.ps1 lo hace para inyectar la versión del tag de git). El default aplica
; solo cuando se compila ad-hoc sin pasar la versión.
#ifndef AppVersion
  #define AppVersion "1.1.0"
#endif
#define AppPublisher   "Mivy"
#define AppURL         "https://mivy.cl"
#define ExeName        "mivy-bridge.exe"
; Must match svcName constant in main.go
#define ServiceName    "mivyPrintBridge"
#define ServiceDisplay "Mivy Print Bridge"
; Legacy service/directory names (for auto-cleanup on upgrade)
#define LegacyServiceName "zenposPrintBridge"
#define LegacyAppDir      "zenPOS\PrintBridge"
#define LegacyExeName     "zenpos-bridge.exe"

[Setup]
AppId={{C4B1F1B6-7E4A-4E3A-90E3-8B83B1AE4F10}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}
AppUpdatesURL={#AppURL}
DefaultDirName={autopf}\Mivy\PrintBridge
DefaultGroupName=Mivy
DisableProgramGroupPage=yes
OutputDir=..\build
OutputBaseFilename=Mivy-PrintBridge-Setup-{#AppVersion}
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
Source: "..\{#ExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.md";  DestDir: "{app}"; DestName: "README.txt"; Flags: ignoreversion

[Dirs]
Name: "{commonappdata}\Mivy\PrintBridge"; Permissions: users-modify

[Icons]
Name: "{group}\Print Bridge (abrir carpeta)"; Filename: "{app}"
Name: "{group}\Desinstalar Print Bridge";    Filename: "{uninstallexe}"

[Run]
; Register the Windows service using the binary's own installer
; (kardianos/service). This sets StartType=automatic, display name,
; description and recovery policy in a single step.
Filename: "{app}\{#ExeName}"; Parameters: "-action=install"; Flags: runhidden waituntilterminated; StatusMsg: "Registrando servicio..."
; Firewall rule: allow inbound on localhost:7777 (loopback only)
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""Mivy Print Bridge"" dir=in action=allow protocol=TCP localport=7777 profile=any"; Flags: runhidden; StatusMsg: "Configurando firewall..."
; (service is started automatically by -action=install)

[UninstallRun]
Filename: "{app}\{#ExeName}"; Parameters: "-action=uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "UninstallService"
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""Mivy Print Bridge"""; Flags: runhidden; RunOnceId: "DeleteFirewallRule"

[UninstallDelete]
Type: filesandordirs; Name: "{commonappdata}\Mivy\PrintBridge"

[Code]
// Stop and remove any pre-existing service before copying the new binary.
// Prevents "file in use" errors on upgrade. Handles both:
//   - Current brand (Mivy Print Bridge) — typical upgrade
//   - Legacy brand (zenPOS Print Bridge) — one-time migration from the old
//     installer. Its firewall rule is deleted too so no orphan rules remain.
function InitializeSetup(): Boolean;
var
  ResultCode: Integer;
  OldExe, LegacyExe: String;
begin
  // Clean up current-brand install (upgrade path)
  OldExe := ExpandConstant('{autopf}\Mivy\PrintBridge\{#ExeName}');
  if FileExists(OldExe) then
    Exec(OldExe, '-action=uninstall', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);

  // Clean up legacy zenPOS install if present
  LegacyExe := ExpandConstant('{autopf}\{#LegacyAppDir}\{#LegacyExeName}');
  if FileExists(LegacyExe) then
    Exec(LegacyExe, '-action=uninstall', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);

  // Raw fallback for stale registrations (either brand)
  Exec(ExpandConstant('{sys}\sc.exe'), 'stop {#ServiceName}',         '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\sc.exe'), 'delete {#ServiceName}',       '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\sc.exe'), 'stop {#LegacyServiceName}',   '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\sc.exe'), 'delete {#LegacyServiceName}', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\netsh.exe'),
    'advfirewall firewall delete rule name="zenPOS Print Bridge"',
    '', SW_HIDE, ewWaitUntilTerminated, ResultCode);

  Result := True;
end;
