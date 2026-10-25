; A retry resumes the caller; cancellation also fails unattended uninstalls.
!macro UninstallRetry MESSAGE
  !define retryAccepted "uninstall_retry_${__COUNTER__}"
  MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "${MESSAGE}" /SD IDCANCEL IDRETRY ${retryAccepted}
  SetErrorLevel 1
  Abort
  ${retryAccepted}:
  !undef retryAccepted
!macroend

!macro RemoveInstalledPath COMMAND PATH
  ${Do}
    ClearErrors
    ${COMMAND} "${PATH}"
    ${IfNot} ${Errors}
      ${Break}
    ${EndIf}
    StrCpy $3 "${PATH}"
    !insertmacro UninstallRetry "$(UninstallRemoveFailed)"
  ${Loop}
!macroend
