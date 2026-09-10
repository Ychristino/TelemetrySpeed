; Inno Setup script for TelemetrySpeed.
;
; Expects the release workflow (.github/workflows/release.yml) to have
; already assembled a complete, ready-to-run copy of the app under
; ..\client\dist\ — TelemetryView.exe, engine.exe, pgbin\ (bundled Postgres
; binaries, so first launch needs no internet — see engine/cmd/engine's
; postgresBinariesPath), and version.txt. This script just packages that
; folder into one Setup.exe; it doesn't build anything itself.
;
; MyAppVersion is passed on the command line: iscc /DMyAppVersion=1.2.3 TelemetrySpeed.iss
#ifndef MyAppVersion
  #define MyAppVersion "0.0.0"
#endif

#define MyAppName "TelemetrySpeed"
#define MyAppPublisher "Ychristino"
#define MyAppURL "https://github.com/Ychristino/TelemetrySpeed"
#define MyAppExeName "TelemetryView.exe"

[Setup]
; Fixed GUID — must stay the same across every release so each new Setup.exe
; upgrades the existing install in place instead of installing side-by-side.
AppId={{9070913F-D616-4F8C-8138-86BB61DC2C91}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}/releases
; Per-user install under %LOCALAPPDATA% — no admin rights required, and
; matches the "download and just run it" experience the app already has.
DefaultDirName={localappdata}\Programs\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
OutputBaseFilename=TelemetrySpeedSetup
OutputDir=..\installer-output
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
; Lets a silent re-run (the in-app auto-updater launches this with
; /VERYSILENT) close the currently-running app itself before overwriting
; its files, instead of failing because the exe is locked.
CloseApplications=yes
RestartApplications=no
UninstallDisplayIcon={app}\{#MyAppExeName}

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop icon"; GroupDescription: "Additional icons:"

[Files]
Source: "..\client\dist\*"; DestDir: "{app}"; Flags: recursesubdirs createallsubdirs

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
; No "skipifsilent" — the in-app auto-updater installs with /VERYSILENT and
; relies on this to relaunch the app afterward. Without it, a silent
; install just finishes invisibly and nothing ever reopens.
Filename: "{app}\{#MyAppExeName}"; Description: "Launch {#MyAppName}"; Flags: nowait postinstall
