param([switch]$Test)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$go = Join-Path $projectRoot '.tools/go/bin/go.exe'
$overlayRoot = Join-Path $PSScriptRoot 'engine-overlay'
$output = Join-Path $projectRoot '.tools/engine-overlay'
New-Item -ItemType Directory -Force -Path $output | Out-Null
$rclone = & $go -C $projectRoot mod download -json 'github.com/rclone/rclone@v1.75.2' | ConvertFrom-Json
if ($LASTEXITCODE -or $rclone.Sum -cne 'h1:0cqa6P3WmhGnu2NeOdZSetJnacpwCRtpsNbHOICqPk8=') { throw 'Invalid rclone source checksum' }
$smb = & $go -C $projectRoot mod download -json 'github.com/cloudsoda/go-smb2@v0.0.0-20260701064823-d8c5600d73b8' | ConvertFrom-Json
if ($LASTEXITCODE) { throw 'Could not retrieve pinned SMB sources' }
$expectedSum = Select-String -LiteralPath (Join-Path $rclone.Dir 'go.sum') -Pattern '^github.com/cloudsoda/go-smb2 v0.0.0-20260701064823-d8c5600d73b8 '
if (!$expectedSum -or $smb.Sum -cne ($expectedSum.Line.Split(' ')[2])) { throw 'Invalid SMB source checksum' }
# Go forbids overlays inside GOMODCACHE, so build from private copies of the verified sources.
$rcloneDir = Join-Path $output 'rclone'
$smbDir = Join-Path $output 'go-smb2'
foreach ($source in @(@{ Module = $rclone; Destination = $rcloneDir }, @{ Module = $smb; Destination = $smbDir })) {
    New-Item -ItemType Directory -Force -Path $source.Destination | Out-Null
    Get-ChildItem -LiteralPath $source.Module.Dir -Force | Copy-Item -Destination $source.Destination -Recurse -Force
}
$modFile = Join-Path $output 'build.mod'
Copy-Item -LiteralPath (Join-Path $rclone.Dir 'go.mod') -Destination $modFile -Force
Copy-Item -LiteralPath (Join-Path $rclone.Dir 'go.sum') -Destination (Join-Path $output 'build.sum') -Force
(Get-Item -LiteralPath $modFile).IsReadOnly = $false
& $go mod edit -modfile $modFile "-replace=github.com/cloudsoda/go-smb2=$smbDir"
if ($LASTEXITCODE) { throw 'Could not prepare isolated engine module' }
$replace = @{}
foreach ($name in @('smb_transport', 'smb_transport_test')) {
    $replace[(Join-Path $smbDir ($name + '.go'))] = Join-Path $overlayRoot ($name + '.go.in')
}
foreach ($name in @('transport_command', 'transport_command_test')) {
    $replace[(Join-Path $rcloneDir ('backend/smb/' + $name + '.go'))] = Join-Path $overlayRoot ($name + '.go.in')
}
$overlay = Join-Path $output 'overlay.json'
$networkReplace = & node (Join-Path $overlayRoot 'prepare-network.mjs') $rcloneDir (Join-Path $output 'network') | ConvertFrom-Json -AsHashtable
if ($LASTEXITCODE) { throw 'Could not prepare network counters' }
foreach ($key in $networkReplace.Keys) { $replace[$key] = $networkReplace[$key] }
[IO.File]::WriteAllText($overlay, (@{ Replace = $replace } | ConvertTo-Json -Depth 4), [Text.UTF8Encoding]::new($false))
$inputs = @(Get-ChildItem -LiteralPath $overlayRoot -File | Sort-Object Name) + @(Get-Item -LiteralPath $PSCommandPath, (Join-Path $PSScriptRoot 'bootstrap.ps1'))
$hashes = @($inputs | ForEach-Object { (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash }) -join ':'
$hasher = [Security.Cryptography.SHA256]::Create()
try { $digest = [BitConverter]::ToString($hasher.ComputeHash([Text.Encoding]::UTF8.GetBytes($hashes))).Replace('-', '') }
finally { $hasher.Dispose() }
if ($Test) {
    & $go -C $rcloneDir test -mod=readonly -modfile $modFile -overlay $overlay ./fs/accounting ./fs/fshttp -run '^TestNetwork' -count=1 | Out-Host
    if ($LASTEXITCODE) { throw 'Network counter tests failed' }
    & $go -C $rcloneDir test -mod=readonly -modfile $modFile -overlay $overlay github.com/cloudsoda/go-smb2 -run '^TestShareTransportEncryption$' -count=1 | Out-Host
    if ($LASTEXITCODE) { throw 'SMB transport tests failed' }
    & $go -C $rcloneDir test -mod=readonly -modfile $modFile -overlay $overlay ./backend/smb -run '^TestTransportCommand' -count=1 | Out-Host
    if ($LASTEXITCODE) { throw 'SMB backend command tests failed' }
}
[PSCustomObject]@{ Path = $overlay; Digest = $digest; RcloneDir = $rcloneDir; SMBDir = $smbDir; ModFile = $modFile }
