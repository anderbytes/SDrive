param([string]$ScannerPath)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$reportPath = Join-Path $projectRoot '.tools/check-results/artifact-antivirus.json'
New-Item -ItemType Directory -Force -Path (Split-Path $reportPath -Parent) | Out-Null
$report = [ordered]@{ status = 'failed'; time = [DateTime]::UtcNow.ToString('o'); signatureVersion = ''; artifacts = @(); error = '' }
try {
    if (!$ScannerPath) {
        $ScannerPath = Join-Path $env:ProgramData 'Microsoft/Windows Defender/Platform/CurrentVersion/MpCmdRun.exe'
        if (!(Test-Path -LiteralPath $ScannerPath -PathType Leaf)) {
            $ScannerPath = Join-Path $env:ProgramFiles 'Windows Defender/MpCmdRun.exe'
        }
    }
    if (!(Test-Path -LiteralPath $ScannerPath -PathType Leaf)) { throw 'Microsoft Defender scanner is unavailable; artifact verification is required.' }
    $status = Get-MpComputerStatus
    if (!$status.AntivirusEnabled -or !$status.RealTimeProtectionEnabled) { throw 'Microsoft Defender protection must be active for artifact verification.' }
    & $ScannerPath -SignatureUpdate | Out-Host
    if ($LASTEXITCODE) { throw 'Microsoft Defender signature update failed.' }
    $report.signatureVersion = (Get-MpComputerStatus).AntivirusSignatureVersion
    if (!$report.signatureVersion) { throw 'Could not verify Microsoft Defender signature version.' }
    $version = (Get-Content -LiteralPath "$projectRoot/frontend/package.json" -Raw | ConvertFrom-Json).version
    if ($version -cnotmatch '^\d+\.\d+\.\d+$') { throw 'Artifact verification requires a stable three-part package version.' }
    # Scan unpacked payloads too: NSIS archive scans can miss a detected executable.
    foreach ($name in @('SDrive.exe', 'SDrive-cleanup.exe', 'rclone.exe', "SDrive-$version-setup.exe", "SDrive-$version-offline-setup.exe")) {
        $file = Get-Item -LiteralPath (Join-Path $projectRoot "bin/$name")
        $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $output = @(& $ScannerPath -Scan -ScanType 3 -File $file.FullName -DisableRemediation 2>&1)
        $scanExit = $LASTEXITCODE
        $text = $output -join "`n"
        $clean = $scanExit -eq 0 -and $text -match '(?im)^Scanning .+ found no threats\.\s*$' -and $text -notmatch '(?i)was skipped|Threat\s+:'
        $report.artifacts += [ordered]@{ file = $name; sha256 = $hash; exitCode = $scanExit; clean = $clean; output = $text }
        $output | Out-Host
        if (!$clean) { throw "Antivirus verification did not confirm a clean scan: $name. Do not publish these artifacts." }
        if ((Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -cne $hash) { throw "Artifact changed during antivirus verification: $name" }
    }
    $report.status = 'passed'
    Write-Output "PASS: application, uninstall helper, rclone and both installers scanned with Defender signatures $($report.signatureVersion)"
} catch {
    $report.error = $_.Exception.Message
    throw
} finally {
    $report | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $reportPath -Encoding utf8
}
