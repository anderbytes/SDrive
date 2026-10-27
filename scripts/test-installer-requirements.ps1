param([string]$Compiler = "$PSScriptRoot/../.tools/nsis/nsis-3.12/makensis.exe")
$ErrorActionPreference = 'Stop'
$Compiler = (Resolve-Path -LiteralPath $Compiler).Path
$workspace = Join-Path ([IO.Path]::GetTempPath()) ('sdrive-requirements-test-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $workspace | Out-Null
try {
    $source = Get-Content -Raw -LiteralPath "$PSScriptRoot/../installer/sdrive.nsi"
    $match = [regex]::Match($source, '(?s)Function \.onInit\r?\n(.*?)FunctionEnd')
    if (-not $match.Success) { throw 'Production requirements block not found' }
    $block = $match.Groups[1].Value
    $architecture = '${IfNot} ${IsNativeAMD64}'
    $registry = 'ReadRegStr $0 HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion" "CurrentBuildNumber"'
    if (-not $block.Contains($architecture) -or -not $block.Contains($registry)) { throw 'Production platform probes changed; update fixture injection' }
    $block = $block.Replace($architecture, '${If} $2 != "amd64"').Replace($registry, 'StrCpy $0 $3')
    $strings = Get-Content -Raw -LiteralPath "$PSScriptRoot/../installer/languages.nsh"
    $messages = [regex]::Matches($strings, '(?m)^LangString Require(?:X64|Windows) \$\{LANG_ENGLISH\} .*') | ForEach-Object { $_.Value }
    if ($messages.Count -ne 2) { throw 'Production requirement messages not found' }
    $fixture = @'
Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
Name "SDrive requirements fixture"
OutFile "@WORKSPACE@\requirements.exe"
RequestExecutionLevel user
SilentInstall silent
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
@MESSAGES@
Function .onInit
  ${GetParameters} $4
  ${GetOptions} $4 "/arch=" $2
  ${GetOptions} $4 "/build=" $3
@BLOCK@
FunctionEnd
Section
  FileOpen $0 "$EXEDIR\installed.txt" w
  FileWrite $0 "accepted"
  FileClose $0
SectionEnd
'@.Replace('@WORKSPACE@', $workspace).Replace('@MESSAGES@', ($messages -join "`n")).Replace('@BLOCK@', $block)
    Set-Content -LiteralPath "$workspace/requirements.nsi" -Value $fixture -Encoding utf8
    & $Compiler /INPUTCHARSET UTF8 /V2 "$workspace/requirements.nsi"
    if ($LASTEXITCODE) { throw 'Could not compile requirements fixture' }
    $cases = @(
        @{ Build = '19045'; Architecture = 'amd64'; Accept = $true },
        @{ Build = '22000'; Architecture = 'amd64'; Accept = $true },
        @{ Build = '26300'; Architecture = 'amd64'; Accept = $true },
        @{ Build = '19044'; Architecture = 'amd64'; Accept = $false },
        @{ Build = '10240'; Architecture = 'amd64'; Accept = $false },
        @{ Build = '7601'; Architecture = 'amd64'; Accept = $false },
        @{ Build = ''; Architecture = 'amd64'; Accept = $false },
        @{ Build = 'invalid'; Architecture = 'amd64'; Accept = $false },
        @{ Build = '19045bad'; Architecture = 'amd64'; Accept = $false },
        @{ Build = '-1'; Architecture = 'amd64'; Accept = $false },
        @{ Build = '4294986341'; Architecture = 'amd64'; Accept = $false },
        @{ Build = '19045'; Architecture = 'x86'; Accept = $false },
        @{ Build = '22000'; Architecture = 'arm64'; Accept = $false }
    )
    foreach ($case in $cases) {
        Remove-Item -LiteralPath "$workspace/installed.txt" -ErrorAction SilentlyContinue
        $process = Start-Process -FilePath "$workspace/requirements.exe" -ArgumentList @('/S', "/arch=$($case.Architecture)", "/build=$($case.Build)") -PassThru -WindowStyle Hidden
        try {
            if (-not $process.WaitForExit(15000)) { $process.Kill(); throw 'Requirements fixture timed out' }
            $expected = if ($case.Accept) { 0 } else { 1 }
            if ($process.ExitCode -ne $expected -or (Test-Path -LiteralPath "$workspace/installed.txt") -ne $case.Accept) {
                throw "Unexpected requirements result: build=$($case.Build), architecture=$($case.Architecture), exit=$($process.ExitCode)"
            }
        } finally { $process.Dispose() }
    }
    Write-Output "PASS installer requirements: $($cases.Count) cases; rejected platforms never reach installation"
} finally {
    $resolved = [IO.Path]::GetFullPath($workspace)
    if ((Split-Path $resolved -Parent) -ne [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') -or (Split-Path $resolved -Leaf) -notlike 'sdrive-requirements-test-*') { throw 'Unsafe fixture cleanup path' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
