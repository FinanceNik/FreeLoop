; The Windows installer (NFR-XPLAT-003), one per architecture, built by
; `make windows` through build/windows/Taskfile.yml. Adapted from the Wails v3.0.0
; NSIS template, self-contained: per user (no UAC prompt, under
; %LOCALAPPDATA%\Programs), offers the WebView2 runtime when it is missing, and
; registers an uninstaller under Apps. Unsigned for now, like the ad-hoc signed
; Mac build (OQ-009).
;
; makensis runs with -NOCD and is given, from the Taskfile (absolute paths):
;   ARCH          amd64 or arm64
;   VERSION       the app's version, as in build/config.yml (0.1.0-beta.1)
;   VERSION4      the same as four numbers for the version resource (0.1.0.0)
;   BINARY        the Freeloop.exe to install
;   NOTICES       its THIRD_PARTY_NOTICES.txt
;   LICENSE_FILE  the repository's LICENSE
;   WEBVIEW2      MicrosoftEdgeWebview2Setup.exe, the bootstrapper Wails ships
;   ICON          build/windows/icon.ico
;   OUTFILE       where to write the installer
;
; Uninstalling removes the program, its shortcuts and its registry entry; it keeps
; the user's data (%APPDATA%\freeloop and the WebView2 cache), as the Mac app's
; removal does.

Unicode true
SetCompressor /SOLID lzma
ManifestDPIAware true
RequestExecutionLevel user

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "WinVer.nsh"
!include "x64.nsh"
!include "FileFunc.nsh"

!define PRODUCT_NAME "Freeloop"
!define COMPANY_NAME "The Freeloop Authors"
!define EXECUTABLE   "Freeloop.exe"
!define UNINST_KEY   "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}"
; The WebView2 runtime's client key (Microsoft's documented detection).
!define WEBVIEW2_KEY "Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

!macro Need NAME
  !ifndef ${NAME}
    !error "makensis needs -D${NAME}=...; build/windows/Taskfile.yml passes it"
  !endif
!macroend
!insertmacro Need ARCH
!insertmacro Need VERSION
!insertmacro Need VERSION4
!insertmacro Need BINARY
!insertmacro Need NOTICES
!insertmacro Need LICENSE_FILE
!insertmacro Need WEBVIEW2
!insertmacro Need ICON
!insertmacro Need OUTFILE

Name "${PRODUCT_NAME}"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\${PRODUCT_NAME}"
InstallDirRegKey HKCU "${UNINST_KEY}" "InstallLocation"
ShowInstDetails show
ShowUninstDetails show
BrandingText "${PRODUCT_NAME} ${VERSION} (${ARCH})"

VIProductVersion "${VERSION4}"
VIFileVersion    "${VERSION4}"
VIAddVersionKey "CompanyName"     "${COMPANY_NAME}"
VIAddVersionKey "FileDescription" "${PRODUCT_NAME} Installer"
VIAddVersionKey "ProductName"     "${PRODUCT_NAME}"
VIAddVersionKey "ProductVersion"  "${VERSION}"
VIAddVersionKey "FileVersion"     "${VERSION}"
VIAddVersionKey "LegalCopyright"  "Copyright 2026 ${COMPANY_NAME}. Apache-2.0."

!define MUI_ICON   "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Start ${PRODUCT_NAME}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${LICENSE_FILE}"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Function .onInit
  SetShellVarContext current
  ; Windows 10 1809 (build 17763) or later: ConPTY and AF_UNIX sockets.
  ${IfNot} ${AtLeastWin10}
  ${OrIfNot} ${AtLeastBuild} 17763
    MessageBox MB_OK|MB_ICONSTOP "${PRODUCT_NAME} needs Windows 10 version 1809 or later." /SD IDOK
    SetErrorLevel 64
    Quit
  ${EndIf}
  ; One installer per architecture; say which one to take instead.
  !if "${ARCH}" == "arm64"
    ${IfNot} ${IsNativeARM64}
      MessageBox MB_OK|MB_ICONSTOP "This is the ${PRODUCT_NAME} installer for ARM64 Windows. Download the x64 (amd64) installer for this computer." /SD IDOK
      SetErrorLevel 65
      Quit
    ${EndIf}
  !else
    ${IfNot} ${IsNativeAMD64}
      MessageBox MB_OK|MB_ICONSTOP "This is the ${PRODUCT_NAME} installer for x64 Windows. Download the ARM64 installer for this computer." /SD IDOK
      SetErrorLevel 65
      Quit
    ${EndIf}
  !endif
FunctionEnd

; The app's window is WebView2. Windows 11 ships the runtime; Windows 10 may not.
; When it is missing, offer Microsoft's bootstrapper, which downloads and installs
; it (per user when run without elevation). A silent install takes the offer.
Function InstallWebView2
  SetRegView 64
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\${WEBVIEW2_KEY}" "pv"
  ${If} $0 != ""
  ${AndIf} $0 != "0.0.0.0"
    Return
  ${EndIf}
  ReadRegStr $0 HKCU "Software\${WEBVIEW2_KEY}" "pv"
  ${If} $0 != ""
  ${AndIf} $0 != "0.0.0.0"
    Return
  ${EndIf}
  MessageBox MB_YESNO|MB_ICONQUESTION "${PRODUCT_NAME} needs the Microsoft Edge WebView2 Runtime, which is not installed.$\r$\n$\r$\nDownload and install it from Microsoft now?" /SD IDYES IDYES install
    DetailPrint "WebView2 Runtime not installed; ${PRODUCT_NAME} will not start until it is."
    Return
  install:
  DetailPrint "Installing the Microsoft Edge WebView2 Runtime"
  InitPluginsDir
  File "/oname=$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" "${WEBVIEW2}"
  ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install' $1
  ${If} $1 != 0
    DetailPrint "The WebView2 bootstrapper exited with $1."
    MessageBox MB_OK|MB_ICONEXCLAMATION "The WebView2 Runtime could not be installed (exit code $1). Install it from https://developer.microsoft.com/microsoft-edge/webview2/ before starting ${PRODUCT_NAME}." /SD IDOK
  ${EndIf}
FunctionEnd

Section "Install"
  SetShellVarContext current
  Call InstallWebView2

  SetOutPath "$INSTDIR"
  File "/oname=${EXECUTABLE}" "${BINARY}"
  File "/oname=THIRD_PARTY_NOTICES.txt" "${NOTICES}"
  File "/oname=LICENSE.txt" "${LICENSE_FILE}"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}.lnk" "$INSTDIR\${EXECUTABLE}"

  SetRegView 64
  WriteRegStr   HKCU "${UNINST_KEY}" "DisplayName"          "${PRODUCT_NAME}"
  WriteRegStr   HKCU "${UNINST_KEY}" "DisplayVersion"       "${VERSION}"
  WriteRegStr   HKCU "${UNINST_KEY}" "Publisher"            "${COMPANY_NAME}"
  WriteRegStr   HKCU "${UNINST_KEY}" "DisplayIcon"          "$INSTDIR\${EXECUTABLE}"
  WriteRegStr   HKCU "${UNINST_KEY}" "InstallLocation"      "$INSTDIR"
  WriteRegStr   HKCU "${UNINST_KEY}" "URLInfoAbout"         "https://github.com/FinanceNik/FreeLoop"
  WriteRegStr   HKCU "${UNINST_KEY}" "UninstallString"      '"$INSTDIR\uninstall.exe"'
  WriteRegStr   HKCU "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" "$0"
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  ; File by file, never RMDir /r: the folder may have been chosen by hand.
  Delete "$INSTDIR\${EXECUTABLE}"
  Delete "$INSTDIR\THIRD_PARTY_NOTICES.txt"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\${PRODUCT_NAME}.lnk"
  SetRegView 64
  DeleteRegKey HKCU "${UNINST_KEY}"
SectionEnd
