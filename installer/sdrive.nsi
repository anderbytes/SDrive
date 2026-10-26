Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
!include "uninstall-retry.nsh"
Name "SDrive"
!ifndef APP_VERSION
  !error "APP_VERSION must be supplied by scripts/package.ps1"
!endif
VIProductVersion "${APP_VERSION}.0"
VIAddVersionKey /LANG=1033 "ProductName" "SDrive"
VIAddVersionKey /LANG=1033 "FileDescription" "SDrive installer"
VIAddVersionKey /LANG=1033 "FileVersion" "${APP_VERSION}"
VIAddVersionKey /LANG=1033 "ProductVersion" "${APP_VERSION}"
!define MUI_ICON "..\assets\sdrive.ico"
!define MUI_UNICON "..\assets\sdrive.ico"
!ifdef OFFLINE
  OutFile "..\bin\SDrive-${APP_VERSION}-offline-setup.exe"
  !define MUI_WELCOMEPAGE_TEXT "$(WelcomeOffline)"
!else
  OutFile "..\bin\SDrive-${APP_VERSION}-setup.exe"
  !define MUI_WELCOMEPAGE_TEXT "$(WelcomeOnline)"
!endif
InstallDir "$LOCALAPPDATA\Programs\SDrive"
RequestExecutionLevel user
SetCompressor /SOLID lzma
ShowInstDetails show
Var WinFspReboot
Var WebViewReboot
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_NOREBOOTSUPPORT
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchSDrive
!define MUI_FINISHPAGE_RUN_TEXT "$(OpenApplication)"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\LICENSE"
!insertmacro MUI_PAGE_INSTFILES
!define MUI_PAGE_CUSTOMFUNCTION_SHOW FinishShow
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "PortugueseBR"
!insertmacro MUI_LANGUAGE "Spanish"
!insertmacro MUI_LANGUAGE "French"
!insertmacro MUI_LANGUAGE "German"
!insertmacro MUI_LANGUAGE "Italian"
!insertmacro MUI_LANGUAGE "SimpChinese"
!include "languages.nsh"


Function .onInit
  ${IfNot} ${IsNativeAMD64}
    MessageBox MB_OK|MB_ICONSTOP "$(RequireX64)"
    Abort
  ${EndIf}
  SetRegView 64
  ReadRegStr $0 HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion" "CurrentBuildNumber"
  SetRegView 32
  ${If} $0 < 22000
    MessageBox MB_OK|MB_ICONSTOP "$(RequireWindows)"
    Abort
  ${EndIf}
FunctionEnd

Function LaunchSDrive
  ${IfNot} ${RebootFlag}
    Exec '"$INSTDIR\SDrive.exe"'
  ${EndIf}
FunctionEnd

Function FinishShow
  ${If} ${RebootFlag}
    SendMessage $mui.FinishPage.Text ${WM_SETTEXT} 0 "STR:$(RestartRequired)"
    SendMessage $mui.FinishPage.Run ${BM_SETCHECK} ${BST_UNCHECKED} 0
    EnableWindow $mui.FinishPage.Run 0
  ${EndIf}
FunctionEnd

; Probe the same file handle lock used across Windows sessions by the application.
!macro CheckDataLock MESSAGE RETRY
  System::Call 'kernel32::CreateFileW(w "$LOCALAPPDATA\SDrive\instance.lock", i 0xC0000000, i 0, p 0, i 3, i 0x80, p 0) p.r0 ?e'
  Pop $1
  ${If} $0 == -1
    ${If} $1 != 2
    ${AndIf} $1 != 3
      !if "${RETRY}" != ""
        !insertmacro UninstallRetry "${MESSAGE}"
        Goto ${RETRY}
      !else
        MessageBox MB_OK|MB_ICONEXCLAMATION "${MESSAGE}"
        Abort
      !endif
    ${EndIf}
  ${Else}
    System::Call 'kernel32::CloseHandle(p r0)'
  ${EndIf}
!macroend

Function CheckRunning
  !insertmacro CheckDataLock "$(InstallRunningSessions)" ""
  ReadEnvStr $1 "USERNAME"
  System::Call 'kernel32::OpenMutexW(i 0x100000, i 0, w "Local\SDrive-$1") p.r0'
  ${If} $0 <> 0
    System::Call 'kernel32::CloseHandle(p r0)'
    MessageBox MB_OK|MB_ICONEXCLAMATION "$(InstallRunning)"
    Abort
  ${EndIf}
FunctionEnd

!macro PrepareComponent ID FILE
  !ifdef OFFLINE
    File /oname=${FILE} "..\.tools\prerequisites\${FILE}"
  !else
    retry_${ID}:
    nsExec::ExecToLog /TIMEOUT=120000 '"$PLUGINSDIR\prerequisites.exe" --download-prerequisite "${ID}" "$PLUGINSDIR"'
    Pop $0
    ${If} $0 != 0
      StrCpy $3 "${FILE}"
      MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "$(DownloadFailed)" IDRETRY retry_${ID}
      Abort
    ${EndIf}
  !endif
!macroend

Section "SDrive"
  Call CheckRunning
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=prerequisites.exe "..\bin\SDrive.exe"
  ClearErrors
  ExecWait '"$PLUGINSDIR\prerequisites.exe" --check-prerequisites' $0
  ${If} ${Errors}
    MessageBox MB_OK|MB_ICONSTOP "$(CheckComponentsFailed)"
    Abort
  ${EndIf}
  StrCpy $1 $0
  ${If} $1 == 10
  ${OrIf} $1 == 30
    DetailPrint "$(PrepareDrives)"
    !insertmacro PrepareComponent winfsp winfsp.msi
    ClearErrors
    ExecWait '"$PLUGINSDIR\prerequisites.exe" --install-prerequisite winfsp "$PLUGINSDIR"' $0
    ${If} ${Errors}
      MessageBox MB_OK|MB_ICONSTOP "$(StartDriveInstallFailed)"
      Abort
    ${EndIf}
    ${If} $0 == 3010
      StrCpy $WinFspReboot 1
      SetRebootFlag true
      DetailPrint "$(DrivesRestartRequired)"
    ${ElseIf} $0 != 0
      MessageBox MB_OK|MB_ICONSTOP "$(PrepareDrivesFailed)"
      Abort
    ${EndIf}
  ${EndIf}
  ${If} $1 == 20
  ${OrIf} $1 == 30
    DetailPrint "$(PrepareInterface)"
    !insertmacro PrepareComponent webview2 webview2.exe
    ClearErrors
    ExecWait '"$PLUGINSDIR\prerequisites.exe" --install-prerequisite webview2 "$PLUGINSDIR"' $0
    ${If} ${Errors}
      MessageBox MB_OK|MB_ICONSTOP "$(StartInterfaceInstallFailed)"
      Abort
    ${EndIf}
    ${If} $0 == 3010
      StrCpy $WebViewReboot 1
      SetRebootFlag true
    ${ElseIf} $0 != 0
      MessageBox MB_OK|MB_ICONSTOP "$(PrepareInterfaceFailed)"
      Abort
    ${EndIf}
  ${EndIf}
  ClearErrors
  ExecWait '"$PLUGINSDIR\prerequisites.exe" --check-prerequisites' $0
  ${If} ${Errors}
    MessageBox MB_OK|MB_ICONSTOP "$(VerifyInstallFailed)"
    Abort
  ${EndIf}
  ${If} $0 != 0
  ${AndIf} $0 != 10
  ${AndIf} $0 != 20
  ${AndIf} $0 != 30
    MessageBox MB_OK|MB_ICONSTOP "$(VerifyComponentsFailed)"
    Abort
  ${EndIf}
  ${If} $0 == 10
  ${OrIf} $0 == 30
    ${If} $WinFspReboot != 1
      MessageBox MB_OK|MB_ICONSTOP "$(DrivesUnavailable)"
      Abort
    ${EndIf}
  ${EndIf}
  ${If} $0 == 20
  ${OrIf} $0 == 30
    ${If} $WebViewReboot != 1
      MessageBox MB_OK|MB_ICONSTOP "$(InterfaceUnavailable)"
      Abort
    ${EndIf}
  ${EndIf}
  DetailPrint "$(InstallApplication)"
  SetOutPath "$INSTDIR"
  File "..\bin\SDrive.exe"
  File "..\bin\rclone.exe"
  File "..\LICENSE"
  File "..\README.md"
  File "..\THIRD_PARTY_NOTICES.md"
  File "..\bin\SDrive-source.zip"
  File "..\bin\rclone-source.zip"
  File /r "..\bin\licenses"
  File /r "..\docs"
  CreateShortcut "$SMPROGRAMS\SDrive.lnk" "$INSTDIR\SDrive.exe"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive" "DisplayName" "SDrive"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive" "DisplayIcon" '"$INSTDIR\SDrive.exe",0'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive" "NoRepair" 1
SectionEnd

Section "Uninstall"
  retry_uninstall_checks:
  ReadRegStr $2 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive" "InstallLocation"
  ${If} $2 != $INSTDIR
    MessageBox MB_OK|MB_ICONEXCLAMATION "$(UninstallDirectoryMismatch)" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
  !insertmacro CheckDataLock "$(UninstallRunning)" retry_uninstall_checks
  ReadEnvStr $1 "USERNAME"
  System::Call 'kernel32::OpenMutexW(i 0x100000, i 0, w "Local\SDrive-$1") p.r0'
  ${If} $0 <> 0
    System::Call 'kernel32::CloseHandle(p r0)'
    !insertmacro UninstallRetry "$(UninstallRunning)"
    Goto retry_uninstall_checks
  ${EndIf}
  ; Never delete caches or credentials: pending uploads may still need recovery.
  ClearErrors
  ExecWait '"$INSTDIR\SDrive.exe" --cleanup-drive-icons' $0
  ${If} ${Errors}
  ${OrIf} $0 != 0
    !insertmacro UninstallRetry "$(UninstallIconsFailed)"
    Goto retry_uninstall_checks
  ${EndIf}
  !insertmacro RemoveInstalledPath Delete "$SMPROGRAMS\SDrive.lnk"
  !insertmacro RemoveInstalledPath Delete "$INSTDIR\rclone.exe"
  !insertmacro RemoveInstalledPath Delete "$INSTDIR\LICENSE"
  !insertmacro RemoveInstalledPath Delete "$INSTDIR\README.md"
  !insertmacro RemoveInstalledPath Delete "$INSTDIR\THIRD_PARTY_NOTICES.md"
  !insertmacro RemoveInstalledPath Delete "$INSTDIR\SDrive-source.zip"
  !insertmacro RemoveInstalledPath Delete "$INSTDIR\rclone-source.zip"
  !insertmacro RemoveInstalledPath "RMDir /r" "$INSTDIR\licenses"
  !insertmacro RemoveInstalledPath "RMDir /r" "$INSTDIR\docs"
  !insertmacro RemoveInstalledPath Delete "$INSTDIR\SDrive.exe"
  !insertmacro RemoveInstalledPath Delete "$INSTDIR\Uninstall.exe"
  ClearErrors
  ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "SDrive"
  ${IfNot} ${Errors}
    retry_remove_startup:
    ClearErrors
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "SDrive"
    ${If} ${Errors}
      !insertmacro UninstallRetry "$(UninstallRegistryFailed)"
      Goto retry_remove_startup
    ${EndIf}
  ${EndIf}
  retry_remove_registration:
  ClearErrors
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive"
  ${If} ${Errors}
    !insertmacro UninstallRetry "$(UninstallRegistryFailed)"
    Goto retry_remove_registration
  ${EndIf}
  ; Leave any files not owned by the installer in place.
  RMDir "$INSTDIR"
  SetErrorLevel 0
SectionEnd
