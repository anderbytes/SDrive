$ErrorActionPreference = 'Stop'
$workspace = Join-Path ([IO.Path]::GetTempPath()) ('sdrive-antivirus-test-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path "$workspace/scripts", "$workspace/frontend", "$workspace/bin" -Force | Out-Null
Copy-Item -LiteralPath "$PSScriptRoot/test-artifact-antivirus.ps1" -Destination "$workspace/scripts"
Set-Content -LiteralPath "$workspace/frontend/package.json" -Value '{"version":"1.2.3"}'
$scanner = Join-Path $workspace 'scanner.ps1'
Set-Content -LiteralPath $scanner -Value @'
$mode = Get-Content -LiteralPath "$PSScriptRoot/mode.txt"
if ($args -contains '-SignatureUpdate') {
    if ($mode -eq 'update-error') { exit 2 }
    exit 0
}
if ($args -notcontains '-DisableRemediation' -or $args -notcontains '-ScanType') { throw 'Unsafe scan arguments' }
$path = $args[[Array]::IndexOf($args, '-File') + 1]
if ($path.Contains('/')) { throw 'Scanner requires a normalized native path' }
switch ($mode) {
    'detection' { Write-Output 'Threat : Trojan:Win32/Fixture'; exit 2 }
    'zero-detection' { Write-Output 'Scanning fixture found no threats.'; Write-Output 'Threat : Trojan:Win32/Fixture'; exit 0 }
    'skipped' { Write-Output 'Scanning fixture was skipped.'; exit 0 }
    'error' { Write-Output 'Scanning fixture found no threats.'; exit 2 }
    'unknown' { Write-Output 'Scan finished.'; exit 0 }
    'mutation' { Add-Content -LiteralPath $path -Value 'changed' }
}
Write-Output 'Scanning fixture found no threats.'
exit 0
'@
function Get-MpComputerStatus {
    [pscustomobject]@{ AntivirusEnabled = $fixtureMode -ne 'inactive'; RealTimeProtectionEnabled = $fixtureMode -ne 'inactive'; AntivirusSignatureVersion = '1.2.3.4' }
}
try {
    foreach ($fixtureMode in @('clean', 'detection', 'zero-detection', 'skipped', 'error', 'unknown', 'mutation', 'update-error', 'inactive', 'missing-scanner', 'missing-artifact')) {
        foreach ($name in @('SDrive.exe', 'SDrive-cleanup.exe', 'rclone.exe', 'SDrive-1.2.3-setup.exe', 'SDrive-1.2.3-offline-setup.exe')) {
            Set-Content -LiteralPath (Join-Path $workspace "bin/$name") -Value 'synthetic artifact'
        }
        Set-Content -LiteralPath "$workspace/mode.txt" -Value $fixtureMode
        $scannerPath = if ($fixtureMode -eq 'missing-scanner') { Join-Path $workspace 'missing.exe' } else { $scanner }
        if ($fixtureMode -eq 'missing-artifact') { Remove-Item -LiteralPath "$workspace/bin/SDrive-1.2.3-offline-setup.exe" }
        $failed = $false
        try { & "$workspace/scripts/test-artifact-antivirus.ps1" -ScannerPath $scannerPath *> $null }
        catch { $failed = $true }
        if ($failed -ne ($fixtureMode -ne 'clean')) { throw "Unexpected gate outcome: $fixtureMode" }
        $report = Get-Content -LiteralPath "$workspace/.tools/check-results/artifact-antivirus.json" -Raw | ConvertFrom-Json
        $expected = if ($failed) { 'failed' } else { 'passed' }
        if ($report.status -cne $expected) { throw "Incorrect evidence status: $fixtureMode" }
        if (!$failed -and ($report.artifacts.Count -ne 5 -or $report.signatureVersion -cne '1.2.3.4' -or @($report.artifacts | Where-Object { !$_.clean -or $_.sha256 -notmatch '^[0-9a-f]{64}$' }).Count)) { throw 'Incomplete clean-scan evidence' }
        if ($failed -and !$report.error) { throw "Missing failure evidence: $fixtureMode" }
        Write-Output "PASS antivirus gate: $fixtureMode"
    }
} finally {
    $resolved = [IO.Path]::GetFullPath($workspace)
    if ((Split-Path $resolved -Parent) -ne [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') -or (Split-Path $resolved -Leaf) -notlike 'sdrive-antivirus-test-*') { throw 'Unsafe fixture cleanup path' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
