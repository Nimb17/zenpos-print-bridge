; zenPOS Print Bridge — Inno Setup Script
; Compilar con: iscc build\installer.iss (desde la raíz del proyecto)
; Requiere: Inno Setup 6.x (https://jrsoftware.org/isinfo.php)

#define AppName "zenPOS Print Bridge"
#define AppVersion "1.0.0"
#define AppPublisher "zenPOS"
#define AppURL "https://zenpos.cl"
#define AppExeName "zenpos-bridge.exe"
#define ServiceName "zenposPrintBridge"

[Setup]
AppId={{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}
DefaultDirName={autopf}\zenPOS\PrintBridge
DefaultGroupName=zenPOS
AllowNoIcons=yes
OutputDir=..\dist
OutputBaseFilename=zenpos-print-bridge-setup
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
UninstallDisplayIcon={app}\{#AppExeName}
CloseApplications=yes

[Languages]
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"

[Tasks]
Name: "startservice"; Description: "Iniciar el servicio automáticamente con Windows"; GroupDescription: "Opciones adicionales:"; Flags: checked

[Files]
Source: "..\zenpos-bridge.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExeName}"
Name: "{group}\Desinstalar {#AppName}"; Filename: "{uninstallexe}"

[Run]
; Instalar como servicio Windows
Filename: "{app}\{#AppExeName}"; Parameters: "-action install"; Flags: runhidden waituntilterminated; StatusMsg: "Instalando servicio..."; Tasks: startservice

[UninstallRun]
; Detener y desinstalar el servicio antes de borrar archivos
Filename: "{app}\{#AppExeName}"; Parameters: "-action uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "UninstallService"

[Code]
procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then begin
    MsgBox('zenPOS Print Bridge instalado correctamente.' + #13#10 +
           'El servicio arrancará automáticamente con Windows.' + #13#10 + #13#10 +
           'Ahora puedes usar zenPOS para imprimir en tu impresora térmica.' + #13#10 +
           'Configura la impresora desde Configuración → Impresora en zenPOS.',
           mbInformation, MB_OK);
  end;
end;
