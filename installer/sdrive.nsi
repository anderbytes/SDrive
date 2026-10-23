Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
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

Function CheckRunning
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
  ReadRegStr $2 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive" "InstallLocation"
  ${If} $2 != $INSTDIR
    MessageBox MB_OK|MB_ICONEXCLAMATION "The installation directory does not match SDrive's registry entry."
    Abort
  ${EndIf}
  ReadEnvStr $1 "USERNAME"
  System::Call 'kernel32::OpenMutexW(i 0x100000, i 0, w "Local\SDrive-$1") p.r0'
  ${If} $0 <> 0
    System::Call 'kernel32::CloseHandle(p r0)'
    MessageBox MB_OK|MB_ICONEXCLAMATION "Close SDrive using Exit safely before uninstalling."
    Abort
  ${EndIf}
  ; Never delete caches or credentials: pending uploads may still need recovery.
  Delete "$SMPROGRAMS\SDrive.lnk"
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "SDrive"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive"
  Delete "$INSTDIR\SDrive.exe"
  Delete "$INSTDIR\rclone.exe"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\THIRD_PARTY_NOTICES.md"
  Delete "$INSTDIR\SDrive-source.zip"
  Delete "$INSTDIR\rclone-source.zip"
  RMDir /r "$INSTDIR\licenses"
  RMDir /r "$INSTDIR\docs"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
