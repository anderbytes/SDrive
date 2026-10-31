param([string]$Compiler = "$PSScriptRoot/../.tools/nsis/nsis-3.12/makensis.exe")
$ErrorActionPreference = 'Stop'
$Compiler = (Resolve-Path -LiteralPath $Compiler).Path
$workspace = Join-Path ([IO.Path]::GetTempPath()) ('sdrive-payload-test-' + [Guid]::NewGuid().ToString('N'))
try {
    $source = Get-Content -LiteralPath "$PSScriptRoot/../installer/sdrive.nsi" -Raw
    $payload = [regex]::Match($source, '(?s)  SetOutPath "\$INSTDIR\\licenses".*?  SetOutPath "\$INSTDIR"').Value
    if (!$payload) { throw 'Production documentation and license payload block not found' }
    $files = @('docs/guide.md', 'docs/nested/reference.md', 'bin/licenses/LICENSE', 'bin/licenses/nested/NOTICE', 'bin/linux-amd64/licenses/unexpected.txt', '.tools/engine/docs/unexpected.txt', 'frontend/node_modules/dependency/docs/unexpected.txt')
    foreach ($relative in $files) {
        $path = Join-Path $workspace "source/$relative"
        New-Item -ItemType Directory -Path (Split-Path $path -Parent) -Force | Out-Null
        Set-Content -LiteralPath $path -Value $relative
    }
    New-Item -ItemType Directory -Path "$workspace/source/installer" -Force | Out-Null
    $fixture = @'
Unicode true
Name "SDrive payload fixture"
OutFile "@WORKSPACE@\fixture.exe"
InstallDir "@WORKSPACE@\installed"
RequestExecutionLevel user
SilentInstall silent
Section
@PAYLOAD@
SectionEnd
'@.Replace('@WORKSPACE@', $workspace).Replace('@PAYLOAD@', $payload)
    Set-Content -LiteralPath "$workspace/source/installer/fixture.nsi" -Value $fixture -Encoding utf8
    & $Compiler /INPUTCHARSET UTF8 /V2 "$workspace/source/installer/fixture.nsi"
    if ($LASTEXITCODE) { throw 'Payload fixture compilation failed' }
    $process = Start-Process -FilePath "$workspace/fixture.exe" -ArgumentList '/S' -PassThru -WindowStyle Hidden
    try {
        if (!$process.WaitForExit(15000) -or $process.ExitCode -ne 0) { throw 'Payload fixture installation failed' }
    } finally { $process.Dispose() }
    $expected = @('docs/guide.md', 'docs/nested/reference.md', 'licenses/LICENSE', 'licenses/nested/NOTICE')
    $actual = @(Get-ChildItem -LiteralPath "$workspace/installed" -Recurse -File | ForEach-Object { [IO.Path]::GetRelativePath("$workspace/installed", $_.FullName).Replace('\', '/') })
    if (@(Compare-Object $expected $actual).Count) { throw ('Unexpected installed payload: ' + ($actual -join ', ')) }
    foreach ($relative in $expected) {
        $sourceRelative = if ($relative.StartsWith('licenses/')) { 'bin/' + $relative } else { $relative }
        if ((Get-FileHash -LiteralPath "$workspace/installed/$relative").Hash -cne (Get-FileHash -LiteralPath "$workspace/source/$sourceRelative").Hash) { throw "Payload content mismatch: $relative" }
    }
    Write-Output 'PASS: nested product documentation and licenses included; tools, dependencies and Linux payloads excluded'
} finally {
    $resolved = [IO.Path]::GetFullPath($workspace)
    if ((Split-Path $resolved -Parent) -ne [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') -or (Split-Path $resolved -Leaf) -notlike 'sdrive-payload-test-*') { throw 'Unsafe fixture cleanup path' }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
