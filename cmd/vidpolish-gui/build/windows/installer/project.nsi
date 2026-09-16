Unicode true

####
## Customized from Wails' default NSIS template to also stage the vidpolish
## CLI binary and put it on PATH, alongside the native GUI app.
##
## Build with: `make gui-build-windows` (see repo root Makefile), which
## copies the CLI's windows/amd64 build into build/bin/vidpolish.exe before
## invoking `wails build -platform windows/amd64 -nsis` here.
####

## PRODUCT_EXECUTABLE would otherwise default to "${INFO_PROJECTNAME}.exe"
## ("vidpolish.exe", since wails.json's "name" is "vidpolish") - the exact
## filename we also stage the CLI binary as below. Without this override,
## `wails.files` renames the GUI binary on install to that same name,
## the CLI's `File` line then overwrites it, and every shortcut/PATH entry
## silently ends up pointing at the CLI instead of the GUI.
!define PRODUCT_EXECUTABLE "vidpolish-gui.exe"

## Installs per-machine (Program Files, all-users Start Menu/Desktop) and
## requires an admin elevation prompt to do so - this is standard NSIS
## behavior for a shared, all-users install (see wails.setShellContext
## below, which maps this to `SetShellVarContext all`), not a "just for
## me" per-user install.
!define REQUEST_EXECUTION_LEVEL "admin"

!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING
!define MUI_COMPONENTSPAGE_NODESC

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
ShowInstDetails show

; HWND_BROADCAST/WM_SETTINGCHANGE (used below to notify Explorer/new shells
; of the PATH change without a reboot) come from MUI.nsh's WinMessages.nsh,
; already included above.

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

Section "-Core" SecCore
    ; Leading "-" hides this from the components list and makes it
    ; mandatory - the app itself, not an optional extra.
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    ; Stage the vidpolish CLI alongside the GUI app (copied into
    ; build/bin/vidpolish.exe by the gui-build-windows Makefile target).
    File "..\..\bin\vidpolish.exe"

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    ; Add the install dir to the machine PATH so `vidpolish` works from any
    ; terminal (requires admin, which wails.setShellContext already assumes
    ; for a per-machine install).
    ReadRegStr $0 HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path"
    StrCpy $1 "$0;$INSTDIR"
    WriteRegExpandStr HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path" "$1"
    SendMessage ${HWND_BROADCAST} ${WM_SETTINGCHANGE} 0 "STR:Environment" /TIMEOUT=5000

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "Create a Desktop shortcut" SecDesktop
    !insertmacro wails.setShellContext
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    ; Note: deliberately not trimming $INSTDIR back out of the machine PATH
    ; here. Hand-rolled PATH string surgery in NSIS is a well-known source
    ; of subtle bugs (encoding, delimiter edge cases) and this repo has no
    ; Windows box to verify it against; a stale PATH entry pointing at a
    ; now-empty directory is harmless. Re-installing simply re-adds it
    ; (duplicates are harmless too - Windows tolerates a repeated PATH
    ; entry).

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
