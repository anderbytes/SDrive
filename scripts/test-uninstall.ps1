param([string]$Compiler = "$PSScriptRoot/../.tools/nsis/nsis-3.12/makensis.exe")
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$Compiler = (Resolve-Path -LiteralPath $Compiler).Path
$workspace = Join-Path ([IO.Path]::GetTempPath()) ('sdrive-uninstall-test-' + [Guid]::NewGuid().ToString('N'))
$registryPath = 'Software\SDrive-Uninstall-Test-' + [Guid]::NewGuid().ToString('N')
$mutexName = 'Local\SDrive-Uninstall-Test-' + [Guid]::NewGuid().ToString('N')
$process = $null
$lock = $null
$mutex = $null
$languageIDs = [ordered]@{ 'en' = 'ENGLISH'; 'pt-BR' = 'PORTUGUESEBR'; 'es' = 'SPANISH'; 'fr' = 'FRENCH'; 'de' = 'GERMAN'; 'it' = 'ITALIAN'; 'zh-CN' = 'SIMPCHINESE' }

# Target only dialogs belonging to the fixture process, never the user's application.
if (-not ('UninstallTestDialog' -as [type])) {
    Add-Type @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class UninstallTestDialog {
    delegate bool WindowCallback(IntPtr window, IntPtr state);
    [DllImport("user32.dll")] static extern bool EnumWindows(WindowCallback callback, IntPtr state);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr window, out uint process);
    [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr window, int id);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr window, StringBuilder text, int count);
    [DllImport("user32.dll")] static extern bool PostMessage(IntPtr window, uint message, IntPtr wparam, IntPtr lparam);
    [DllImport("user32.dll")] static extern bool IsWindowEnabled(IntPtr window);
    public static IntPtr Find(int process, int button, bool messageOnly = false) {
        IntPtr result = IntPtr.Zero;
        EnumWindows((window, state) => {
            uint owner;
            GetWindowThreadProcessId(window, out owner);
            if (owner == process && GetDlgItem(window, button) != IntPtr.Zero && (!messageOnly || Text(window).Length > 0)) { result = window; return false; }
            return true;
        }, IntPtr.Zero);
        return result;
    }
    public static string Text(IntPtr window, int id = 65535) {
        var text = new StringBuilder(4096);
        GetWindowText(GetDlgItem(window, id), text, text.Capacity);
        return text.ToString();
    }
    public static void Click(IntPtr window, int button) {
        var control = GetDlgItem(window, button);
        if (IsWindowEnabled(control)) PostMessage(control, 0xF5, IntPtr.Zero, IntPtr.Zero);
    }
}
'@
}

function Assert($Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}

function Compile([string]$Name, [string]$Source) {
    $sourcePath = Join-Path $workspace "$Name.nsi"
    [IO.File]::WriteAllText($sourcePath, $Source, [Text.UTF8Encoding]::new($false))
    & $Compiler /INPUTCHARSET UTF8 /V2 $sourcePath
    if ($LASTEXITCODE) { throw "Compilation failed: $Name" }
}

function Wait-Dialog([int]$Button = 4, [IntPtr]$Previous = [IntPtr]::Zero) {
    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    do {
        $dialog = [UninstallTestDialog]::Find($process.Id, $Button, $true)
        if ($dialog -ne [IntPtr]::Zero -and $dialog -ne $Previous -and [UninstallTestDialog]::Text($dialog)) { return $dialog }
        if ($process.HasExited) { throw "Fixture exited before its dialog (code $($process.ExitCode))" }
        Start-Sleep -Milliseconds 50
    } while ([DateTime]::UtcNow -lt $deadline)
    throw 'Timed out waiting for the fixture dialog'
}

function Wait-Exit([int]$Code) {
    # Interactive NSIS reports completion/abort in its progress page.
    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    while (-not $process.HasExited -and [DateTime]::UtcNow -lt $deadline) {
        foreach ($button in @(1, 2)) {
            $window = [UninstallTestDialog]::Find($process.Id, $button)
            $pattern = if ($Code -eq 1) { 'Close|Fechar|Cerrar|Fermer|Beenden|Fine|关闭|Cancel|Annuler|Abbrechen|Annulla|取消' } else { 'Close|Fechar|Cerrar|Fermer|Beenden|Fine|关闭' }
            if ($window -ne [IntPtr]::Zero -and [UninstallTestDialog]::Text($window, $button) -match $pattern) {
                [UninstallTestDialog]::Click($window, $button)
            }
        }
        Start-Sleep -Milliseconds 50
    }
    Assert $process.HasExited "Fixture did not exit (buttons: $([UninstallTestDialog]::Text($window, 1)), $([UninstallTestDialog]::Text($window, 2)))"
    Assert ($process.ExitCode -eq $Code) "Expected exit $Code, got $($process.ExitCode)"
}

try {
    New-Item -ItemType Directory -Path $workspace | Out-Null
    $source = Get-Content -LiteralPath "$projectRoot/installer/sdrive.nsi" -Raw -Encoding utf8
    $source = $source.Replace('!include "languages.nsh"', (Get-Content -LiteralPath "$projectRoot/installer/languages.nsh" -Raw -Encoding utf8))
    $languages = [regex]::Match($source, '(?s)!insertmacro MUI_LANGUAGE "English".*?(?=Function \.onInit)').Value
    $probe = [regex]::Match($source, '(?s)!macro CheckDataLock .*?!macroend').Value
    $section = [regex]::Match($source, '(?s)Section "Uninstall".*?SectionEnd').Value
    Assert ($languages -and $probe -and $section) 'Could not find production uninstall code'
    $section = $section.Replace('Software\Microsoft\Windows\CurrentVersion\Uninstall\SDrive', $registryPath)
    $section = $section.Replace('Software\Microsoft\Windows\CurrentVersion\Run', "$registryPath\Run")
    $section = $section.Replace('Local\SDrive-$1', $mutexName).Replace('$SMPROGRAMS\SDrive.lnk', '$INSTDIR\SDrive.lnk')
    $section = $section.Replace('..\bin\SDrive-cleanup.exe', "$workspace\helper.exe")
    $probe = $probe.Replace('$LOCALAPPDATA\SDrive\instance.lock', '$INSTDIR\instance.lock')
    $include = (Join-Path $projectRoot 'installer/uninstall-retry.nsh')
    Compile 'helper' @'
Unicode true
!include "FileFunc.nsh"
Name "SDrive cleanup test helper"
OutFile "@WORKSPACE@\helper.exe"
RequestExecutionLevel user
SilentInstall silent
Section
  ${GetParameters} $0
  StrCpy $0 $0 -1 1
  ${GetParent} $0 $1
  FileOpen $0 "$1\cleanup-count.txt" a
  FileSeek $0 0 END
  FileWrite $0 "cleanup$\r$\n"
  FileClose $0
  IfFileExists "$1\fail-cleanup" 0 +3
    SetErrorLevel 1
    Quit
  SetErrorLevel 0
SectionEnd
'@.Replace('@WORKSPACE@', $workspace)
    $fixture = @'
Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "@INCLUDE@"
Name "SDrive uninstall test"
OutFile "@WORKSPACE@\creator.exe"
RequestExecutionLevel user
SetCompress off
!insertmacro MUI_UNPAGE_INSTFILES
@LANGUAGES@
@PROBE@
Function un.onInit
  StrCpy $LANGUAGE ${LANG_ENGLISH}
  ReadEnvStr $R0 "SDRIVE_UNINSTALL_TEST_LANGUAGE"
@LANGUAGE_SELECTION@
FunctionEnd
Section
  WriteUninstaller "@WORKSPACE@\uninstaller.exe"
SectionEnd
@SECTION@
'@
    $selection = foreach ($language in $languageIDs.Keys) { '  StrCmp $R0 "' + $language + '" 0 +2' + "`n" + '    StrCpy $LANGUAGE ${LANG_' + $languageIDs[$language] + '}' }
    $fixture = $fixture.Replace('@LANGUAGE_SELECTION@', ($selection -join "`n")).Replace('@INCLUDE@', $include).Replace('@WORKSPACE@', $workspace).Replace('@LANGUAGES@', $languages).Replace('@PROBE@', $probe).Replace('@SECTION@', $section)
    Compile 'fixture' $fixture
    $creator = Start-Process -FilePath "$workspace/creator.exe" -ArgumentList '/S' -PassThru -WindowStyle Hidden
    Assert ($creator.WaitForExit(15000) -and $creator.ExitCode -eq 0) 'Could not create fixture uninstaller'
    foreach ($scenario in @('lock-retry', 'lock-cancel', 'mutex-retry', 'mutex-cancel', 'cleanup-retry', 'cleanup-cancel', 'helper-retry', 'file-retry', 'directory-retry', 'file-cancel', 'silent-lock', 'silent-mutex', 'silent-cleanup', 'silent-file', 'directory-mismatch', 'silent-directory-mismatch', 'missing-application', 'damaged-application', 'missing-owned-files')) {
        foreach ($language in $languageIDs.Keys) {
            $install = Join-Path $workspace "$scenario-$language"
            New-Item -ItemType Directory -Path "$install/docs" -Force | Out-Null
            Copy-Item -LiteralPath "$workspace/helper.exe" -Destination "$install/SDrive.exe"
            Set-Content -LiteralPath "$install/rclone.exe" -Value 'fixture'
            Set-Content -LiteralPath "$install/docs/locked.txt" -Value 'fixture'
            Set-Content -LiteralPath "$install/Uninstall.exe" -Value 'fixture'
            Set-Content -LiteralPath "$install/preserved.txt" -Value 'unowned file'
            Set-Content -LiteralPath "$install/settings.json" -Value 'preserved settings'
            Set-Content -LiteralPath "$install/key.dpapi" -Value 'preserved credentials'
            New-Item -ItemType Directory -Path "$install/cache" -Force | Out-Null
            Set-Content -LiteralPath "$install/cache/pending.txt" -Value 'pending upload'
            if ($scenario -in @('missing-application', 'missing-owned-files')) { Remove-Item -LiteralPath "$install/SDrive.exe" }
            if ($scenario -eq 'damaged-application') { Set-Content -LiteralPath "$install/SDrive.exe" -Value 'damaged executable' }
            if ($scenario -eq 'missing-owned-files') { Remove-Item -LiteralPath "$install/rclone.exe", "$install/Uninstall.exe", "$install/docs/locked.txt" }
            $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($registryPath)
            $mismatch = $scenario -like '*directory-mismatch'
            $key.SetValue('InstallLocation', $(if ($mismatch) { "$install-other" } else { $install }))
            $runKey = $key.CreateSubKey('Run')
            $runKey.SetValue('SDrive', 'fixture')
            $runKey.Dispose()
            $key.Dispose()
            if ($scenario -like '*lock*') { $lock = [IO.File]::Open("$install/instance.lock", 'OpenOrCreate', 'ReadWrite', 'None') }
            if ($scenario -like '*mutex*') { $mutex = [Threading.Mutex]::new($false, $mutexName) }
            if ($scenario -like '*cleanup*') { Set-Content -LiteralPath "$install/fail-cleanup" -Value 'fail' }
            if ($scenario -in @('file-retry', 'file-cancel', 'silent-file')) { $lock = [IO.File]::Open("$install/rclone.exe", 'Open', 'ReadWrite', 'None') }
            if ($scenario -eq 'helper-retry') { $lock = [IO.File]::Open("$install/SDrive.exe", 'Open', 'ReadWrite', 'None') }
            if ($scenario -eq 'directory-retry') { $lock = [IO.File]::Open("$install/docs/locked.txt", 'Open', 'ReadWrite', 'None') }

            $start = [Diagnostics.ProcessStartInfo]::new("$workspace/uninstaller.exe")
            $start.UseShellExecute = $false
            $start.EnvironmentVariables['SDRIVE_UNINSTALL_TEST_LANGUAGE'] = $language
            $silent = $scenario -like 'silent-*'
            $start.Arguments = $(if ($silent) { '/S ' } else { '' }) + '_?=' + $install
            $process = [Diagnostics.Process]::Start($start)
            $cancel = $scenario -like '*-cancel' -or $silent -or $mismatch
            if (-not $silent -and $scenario -notin @('missing-application', 'damaged-application', 'missing-owned-files')) {
                $dialog = Wait-Dialog $(if ($mismatch) { 2 } else { 4 })
                $text = [UninstallTestDialog]::Text($dialog)
                $pattern = '(?m)^LangString Uninstall\w+ \$\{LANG_' + $languageIDs[$language] + '\} "(.*)"\r?$'
                $expected = @([regex]::Matches($source, $pattern) | ForEach-Object { '^' + [regex]::Escape($_.Groups[1].Value.Replace('$\"', '"')).Replace('\$3', '.+?') + '$' })
                Assert (@($expected | Where-Object { $text -cmatch $_ }).Count -gt 0) "Wrong dialog language: $text"
                Assert (Test-Path -LiteralPath "$install/SDrive.exe") 'Executable removed before blocker resolved'
                $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($registryPath)
                Assert ($null -ne $key) 'Uninstall entry removed before blocker resolved'
                $key.Dispose()
                if (-not $cancel) {
                    # Retrying a persistent blocker must return to the prompt.
                    [UninstallTestDialog]::Click($dialog, 4)
                    $dialog = Wait-Dialog -Previous $dialog
                    if ($lock) { $lock.Dispose(); $lock = $null }
                    if ($mutex) { $mutex.Dispose(); $mutex = $null }
                    if (Test-Path -LiteralPath "$install/fail-cleanup") { Remove-Item -LiteralPath "$install/fail-cleanup" }
                    [UninstallTestDialog]::Click($dialog, 4)
                } else {
                    [UninstallTestDialog]::Click($dialog, 2)
                }
            }
            Wait-Exit $(if ($cancel) { 1 } else { 0 })
            if ($lock) { $lock.Dispose(); $lock = $null }
            if ($mutex) { $mutex.Dispose(); $mutex = $null }
            $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($registryPath)
            if ($cancel) {
                Assert ($null -ne $key) 'Cancellation removed the uninstall entry'
                Assert (Test-Path -LiteralPath "$install/SDrive.exe") 'Cancellation removed the helper'
            } else {
                Assert ($null -eq $key) 'Successful uninstall left its registry entry'
                Assert (-not (Test-Path -LiteralPath "$install/SDrive.exe")) 'Successful uninstall left its executable'
                Assert (-not (Test-Path -LiteralPath "$install/rclone.exe")) 'Successful uninstall left its worker'
                Assert (-not (Test-Path -LiteralPath "$install/docs")) 'Successful uninstall left owned documents'
                $count = @(Get-Content -LiteralPath "$install/cleanup-count.txt").Count
                Assert ($count -eq $(if ($scenario -eq 'cleanup-retry') { 3 } else { 1 })) "Retry repeated the wrong phase ($scenario, count $count)"
            }
            if ($key) { $key.Dispose() }
            Assert ((Get-Content -LiteralPath "$install/preserved.txt") -eq 'unowned file') 'Removed an unowned file'
            Assert ((Get-Content -LiteralPath "$install/settings.json") -eq 'preserved settings') 'Removed settings'
            Assert ((Get-Content -LiteralPath "$install/key.dpapi") -eq 'preserved credentials') 'Removed credentials'
            Assert ((Get-Content -LiteralPath "$install/cache/pending.txt") -eq 'pending upload') 'Removed pending uploads'
            [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($registryPath, $false)
            Write-Output "PASS $scenario ($language)"
            $process.Dispose()
            $process = $null
        }
    }
} finally {
    if ($lock) { $lock.Dispose() }
    if ($mutex) { $mutex.Dispose() }
    if ($process -and -not $process.HasExited) { $process.Kill(); $process.WaitForExit() }
    [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($registryPath, $false)
    $resolved = [IO.Path]::GetFullPath($workspace)
    Assert ((Split-Path $resolved -Parent) -eq [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')) 'Unsafe test cleanup path'
    Assert ((Split-Path $resolved -Leaf) -like 'sdrive-uninstall-test-*') 'Unsafe test cleanup name'
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
