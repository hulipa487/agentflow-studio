; Per-user NSIS installer for AgentFlow Studio (Windows amd64 & arm64).
; Pass -DVERSION=... -DARCH=... -DBINARY=... -DICON=... -DOUTFILE=...

Unicode true
!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"

!ifndef VERSION
  !define VERSION "0.1.0"
!endif
!ifndef ARCH
  !define ARCH "amd64"
!endif
!ifndef BINARY
  !define BINARY "..\..\agentflow-studio.exe"
!endif
!ifndef ICON
  !define ICON "app.ico"
!endif
!ifndef OUTFILE
  !define OUTFILE "..\dist\agentflow-studio-${VERSION}-windows-${ARCH}-setup.exe"
!endif

!define APP_NAME "AgentFlow Studio"
!define EXE_NAME "agentflow-studio.exe"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\AgentFlowStudio"

Name "${APP_NAME}"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\AgentFlow Studio"
InstallDirRegKey HKCU "Software\AgentFlowStudio" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma

!define MUI_ABORTWARNING
!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE_NAME}"
!define MUI_FINISHPAGE_RUN_TEXT "Launch ${APP_NAME}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Section "WebView2 Runtime Check" SecWebView2
  StrCpy $0 ""
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 == ""
    ReadRegStr $0 HKLM "SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${EndIf}
  ${If} $0 == ""
    ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${EndIf}
  ${If} $0 == ""
    DetailPrint "Installing Microsoft Edge WebView2 Runtime..."
    InitPluginsDir
    nsExec::ExecToLog `powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "Invoke-WebRequest -UseBasicParsing https://go.microsoft.com/fwlink/p/?LinkId=2124703 -OutFile '$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe'"`
    Pop $1
    ${If} $1 == 0
      nsExec::ExecToLog '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install'
      Pop $2
    ${EndIf}
  ${EndIf}
SectionEnd

Section "AgentFlow Studio" SecMain
  SetShellVarContext current
  SetOutPath "$INSTDIR"

  ; Close any running instance before replacing the binary.
  nsExec::Exec 'taskkill /IM "${EXE_NAME}" /F'
  Sleep 300

  File "/oname=${EXE_NAME}" "${BINARY}"
  WriteUninstaller "$INSTDIR\Uninstall.exe"

  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
  CreateShortCut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" "$INSTDIR\${EXE_NAME}"
  CreateShortCut "$SMPROGRAMS\${APP_NAME}\Uninstall ${APP_NAME}.lnk" "$INSTDIR\Uninstall.exe"
  CreateShortCut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${EXE_NAME}"

  WriteRegStr HKCU "Software\AgentFlowStudio" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "AgentFlow"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${EXE_NAME}"
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1

  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" $0
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  nsExec::Exec 'taskkill /IM "${EXE_NAME}" /F'
  Sleep 300

  Delete "$DESKTOP\${APP_NAME}.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\Uninstall ${APP_NAME}.lnk"
  RMDir "$SMPROGRAMS\${APP_NAME}"

  Delete "$INSTDIR\${EXE_NAME}"
  Delete "$INSTDIR\${EXE_NAME}.old"
  Delete "$INSTDIR\${EXE_NAME}.new"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  DeleteRegKey HKCU "${UNINST_KEY}"
  DeleteRegKey HKCU "Software\AgentFlowStudio"
SectionEnd
