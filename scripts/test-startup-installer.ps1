param([string]$Compiler = "$PSScriptRoot/../.tools/nsis/nsis-3.12/makensis.exe")
$ErrorActionPreference = 'Stop'
$workspace = Join-Path ([IO.Path]::GetTempPath()) ('sdrive-startup-test-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $workspace | Out-Null
function Compile([string]$Name, [string]$Source) {
    $path = Join-Path $workspace "$Name.nsi"
    Set-Content -LiteralPath $path -Value $Source -Encoding utf8
    & $Compiler /INPUTCHARSET UTF8 /V2 $path
    if ($LASTEXITCODE) { throw "Could not compile $Name" }
}
function Run([string]$Path, [int]$Expected) {
    $process = Start-Process -FilePath $Path -ArgumentList '/S' -PassThru -WindowStyle Hidden
    try {
        if (-not $process.WaitForExit(15000)) { $process.Kill(); throw 'Fixture timed out' }
        if ($process.ExitCode -ne $Expected) { throw "Expected $Expected, received $($process.ExitCode)" }
    } finally { $process.Dispose() }
}
try {
    $source = Get-Content -Raw -LiteralPath "$PSScriptRoot/../installer/sdrive.nsi"
    $match = [regex]::Match($source, '(?s)  retry_restore_startup:.*?(?=SectionEnd)')
    if (-not $match.Success) { throw 'Production startup restoration block not found' }
    $strings = Get-Content -Raw -LiteralPath "$PSScriptRoot/../installer/languages.nsh"
    $message = [regex]::Match($strings, '(?m)^LangString RestoreStartupFailed \$\{LANG_ENGLISH\} .*').Value
    if (-not $message) { throw 'Production startup error message not found' }
    Compile 'installer' @'
Unicode true
!include "LogicLib.nsh"
!include "MUI2.nsh"
Name "SDrive startup fixture"
OutFile "@WORKSPACE@\installer.exe"
InstallDir "@WORKSPACE@"
RequestExecutionLevel user
SilentInstall silent
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
@MESSAGE@
Section
@RESTORE@
SectionEnd
'@.Replace('@WORKSPACE@', $workspace).Replace('@MESSAGE@', $message).Replace('@RESTORE@', $match.Value)
    Compile 'helper' @'
Unicode true
!include "FileFunc.nsh"
Name "SDrive startup helper fixture"
OutFile "@WORKSPACE@\SDrive.exe"
RequestExecutionLevel user
SilentInstall silent
Section
  ${GetParameters} $0
  StrCmp $0 "--restore-startup" 0 wrong_arguments
  FileOpen $0 "$EXEDIR\called.txt" w
  FileWrite $0 "restored"
  FileClose $0
  IfFileExists "$EXEDIR\fail" failure
  SetErrorLevel 0
  Quit
  failure:
  SetErrorLevel 1
  Quit
  wrong_arguments:
  SetErrorLevel 3
SectionEnd
'@.Replace('@WORKSPACE@', $workspace)
    Set-Content -LiteralPath "$workspace/settings.json" -Value 'preserved profile'
    Run "$workspace/installer.exe" 0
    if ((Get-Content -Raw -LiteralPath "$workspace/called.txt") -ne 'restored') { throw 'Installed executable was not called' }
    Set-Content -LiteralPath "$workspace/fail" -Value 'registry denied'
    Run "$workspace/installer.exe" 1
    Remove-Item -LiteralPath "$workspace/SDrive.exe"
    Run "$workspace/installer.exe" 1
    if ((Get-Content -LiteralPath "$workspace/settings.json") -ne 'preserved profile') { throw 'Failure changed profile' }
    Write-Output 'PASS startup installer: installed helper, exact arguments, success, helper failure, launch failure, silent cancellation and preserved profile'
} finally {
    $resolved = [IO.Path]::GetFullPath($workspace)
    if ((Split-Path $resolved -Parent) -ne [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') -or (Split-Path $resolved -Leaf) -notlike 'sdrive-startup-test-*') { throw 'Unsafe fixture cleanup path' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
