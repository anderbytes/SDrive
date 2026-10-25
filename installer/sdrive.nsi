Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
!include "uninstall-retry.nsh"
Name "SDrive"
!ifdef OFFLINE
  OutFile "..\bin\SDrive-0.1.0-offline-setup.exe"
  !define MUI_WELCOMEPAGE_TEXT "This installer includes every component required by SDrive and works without internet access.$\r$\n$\r$\nYou do not need to download tools or run commands. Windows may request permission to prepare virtual drives.$\r$\n$\r$\nWinFsp and Microsoft WebView2 retain their own licenses; see THIRD_PARTY_NOTICES.md in the application folder."
!else
  OutFile "..\bin\SDrive-0.1.0-setup.exe"
  !define MUI_WELCOMEPAGE_TEXT "This installer automatically downloads missing Windows components. Stay connected to the internet during installation.$\r$\n$\r$\nYou do not need to download tools manually or run commands. Windows may request permission to prepare virtual drives.$\r$\n$\r$\nWinFsp and Microsoft WebView2 retain their own licenses; see THIRD_PARTY_NOTICES.md in the application folder."
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
!define MUI_FINISHPAGE_RUN_TEXT "Open SDrive"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\LICENSE"
!insertmacro MUI_PAGE_INSTFILES
!define MUI_PAGE_CUSTOMFUNCTION_SHOW FinishShow
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "PortugueseBR"

LangString UninstallRunning ${LANG_ENGLISH} "Close SDrive in all Windows sessions using Exit safely, then choose Retry to continue uninstalling."
LangString UninstallRunning ${LANG_PORTUGUESEBR} "Feche o SDrive em todas as sessões do Windows usando Sair com segurança e clique em Tentar novamente para continuar a desinstalação."
LangString UninstallIconsFailed ${LANG_ENGLISH} "Could not remove SDrive's Explorer icons. If needed, open SDrive, turn off service icons and exit safely. Choose Retry to try again."
LangString UninstallIconsFailed ${LANG_PORTUGUESEBR} "Não foi possível remover os ícones do SDrive no Explorador. Se necessário, abra o SDrive, desative os ícones dos serviços e saia com segurança. Clique em Tentar novamente."
LangString UninstallRemoveFailed ${LANG_ENGLISH} "Could not remove $3. Close applications using it or resolve the access problem, then choose Retry."
LangString UninstallRemoveFailed ${LANG_PORTUGUESEBR} "Não foi possível remover $3. Feche os aplicativos que usam esse caminho ou resolva o problema de acesso e clique em Tentar novamente."
LangString UninstallDirectoryMismatch ${LANG_ENGLISH} "The installation directory does not match SDrive's registry entry."
LangString UninstallDirectoryMismatch ${LANG_PORTUGUESEBR} "A pasta de instalação não corresponde ao registro do SDrive."
LangString UninstallRegistryFailed ${LANG_ENGLISH} "Could not remove SDrive's Windows registration. Resolve the registry access problem, then choose Retry."
LangString UninstallRegistryFailed ${LANG_PORTUGUESEBR} "Não foi possível remover o registro do SDrive no Windows. Resolva o problema de acesso ao registro e clique em Tentar novamente."

Function .onInit
  ${IfNot} ${IsNativeAMD64}
    MessageBox MB_OK|MB_ICONSTOP "This version of SDrive requires Windows 11 x64 (Intel or AMD)."
    Abort
  ${EndIf}
  SetRegView 64
  ReadRegStr $0 HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion" "CurrentBuildNumber"
  SetRegView 32
  ${If} $0 < 22000
    MessageBox MB_OK|MB_ICONSTOP "This version of SDrive requires Windows 11 or later."
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
    SendMessage $mui.FinishPage.Text ${WM_SETTEXT} 0 "STR:Installation is complete. Restart Windows before opening SDrive to activate installed components. Your files and settings have been preserved."
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
  !insertmacro CheckDataLock "Close SDrive in all Windows sessions using Exit safely before installing or updating." ""
  ReadEnvStr $1 "USERNAME"
  System::Call 'kernel32::OpenMutexW(i 0x100000, i 0, w "Local\SDrive-$1") p.r0'
  ${If} $0 <> 0
    System::Call 'kernel32::CloseHandle(p r0)'
    MessageBox MB_OK|MB_ICONEXCLAMATION "Close SDrive using Exit safely before installing or updating."
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
      MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "Could not download or verify ${FILE}. Check your internet connection and try again. See this window's details for the reason. You can also cancel and use the offline installer." IDRETRY retry_${ID}
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
    MessageBox MB_OK|MB_ICONSTOP "Could not check Windows components. Run the installer again."
    Abort
  ${EndIf}
  StrCpy $1 $0
  ${If} $1 == 10
  ${OrIf} $1 == 30
    DetailPrint "Preparing virtual drives (WinFsp)…"
    !insertmacro PrepareComponent winfsp winfsp.msi
    ClearErrors
    ExecWait '"$PLUGINSDIR\prerequisites.exe" --install-prerequisite winfsp "$PLUGINSDIR"' $0
    ${If} ${Errors}
      MessageBox MB_OK|MB_ICONSTOP "Could not start virtual drive installation. Run the installer again."
      Abort
    ${EndIf}
    ${If} $0 == 3010
      StrCpy $WinFspReboot 1
      SetRebootFlag true
      DetailPrint "Virtual drives will be activated after restarting Windows."
    ${ElseIf} $0 != 0
      MessageBox MB_OK|MB_ICONSTOP "Could not prepare virtual drives (code $0). If Windows authorization was cancelled, run the installer again and allow installation."
      Abort
    ${EndIf}
  ${EndIf}
  ${If} $1 == 20
  ${OrIf} $1 == 30
    DetailPrint "Preparing the application interface (Microsoft WebView2)…"
    !insertmacro PrepareComponent webview2 webview2.exe
    ClearErrors
    ExecWait '"$PLUGINSDIR\prerequisites.exe" --install-prerequisite webview2 "$PLUGINSDIR"' $0
    ${If} ${Errors}
      MessageBox MB_OK|MB_ICONSTOP "Could not start interface installation. Run the installer again."
      Abort
    ${EndIf}
    ${If} $0 == 3010
      StrCpy $WebViewReboot 1
      SetRebootFlag true
    ${ElseIf} $0 != 0
      MessageBox MB_OK|MB_ICONSTOP "Could not prepare the application interface (code $0). Run the installer again."
      Abort
    ${EndIf}
  ${EndIf}
  ClearErrors
  ExecWait '"$PLUGINSDIR\prerequisites.exe" --check-prerequisites' $0
  ${If} ${Errors}
    MessageBox MB_OK|MB_ICONSTOP "Could not verify installation. Run the installer again."
    Abort
  ${EndIf}
  ${If} $0 != 0
  ${AndIf} $0 != 10
  ${AndIf} $0 != 20
  ${AndIf} $0 != 30
    MessageBox MB_OK|MB_ICONSTOP "Could not verify installed components (code $0). Run the installer again."
    Abort
  ${EndIf}
  ${If} $0 == 10
  ${OrIf} $0 == 30
    ${If} $WinFspReboot != 1
      MessageBox MB_OK|MB_ICONSTOP "Virtual drives are still unavailable. Run the installer again or contact SDrive support."
      Abort
    ${EndIf}
  ${EndIf}
  ${If} $0 == 20
  ${OrIf} $0 == 30
    ${If} $WebViewReboot != 1
      MessageBox MB_OK|MB_ICONSTOP "The application interface is still unavailable. Run the installer again or contact SDrive support."
      Abort
    ${EndIf}
  ${EndIf}
  DetailPrint "Installing SDrive for your user…"
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
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive" "DisplayVersion" "0.1.0"
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
