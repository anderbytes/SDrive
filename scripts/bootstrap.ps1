$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$toolsDir = Join-Path $projectRoot '.tools'
New-Item -ItemType Directory -Force -Path $toolsDir | Out-Null

function Get-VerifiedArchive($Uri, $Destination, $Sha256) {
    if (!(Test-Path -LiteralPath $Destination)) {
        Invoke-WebRequest -Uri $Uri -OutFile $Destination
    }
    if ((Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash -ne $Sha256) {
        throw "Invalid checksum: $Destination"
    }
}

if (!(Test-Path -LiteralPath "$toolsDir/go/bin/go.exe")) {
    $archive = Join-Path $toolsDir 'go.zip'
    Get-VerifiedArchive 'https://go.dev/dl/go1.26.10.windows-amd64.zip' $archive '7ecd2d7260e82c59bcbd7f1077a07cee2dea607295b858df67195a1e4e09629b'
    Expand-Archive -LiteralPath $archive -DestinationPath $toolsDir -Force
}

$go = Join-Path $toolsDir 'go/bin/go.exe'
$binary = Join-Path $toolsDir 'rclone/rclone.exe'
$stampPath = Join-Path $toolsDir 'rclone/build.json'
$sourceSum = 'h1:0cqa6P3WmhGnu2NeOdZSetJnacpwCRtpsNbHOICqPk8='
$overlay = & "$PSScriptRoot/engine-overlay.ps1"
$needsBuild = $true
if ((Test-Path -LiteralPath $stampPath) -and (Test-Path -LiteralPath $binary)) {
    $stamp = Get-Content -LiteralPath $stampPath -Raw | ConvertFrom-Json
    $needsBuild = $stamp.sourceSum -cne $sourceSum -or $stamp.overlayDigest -cne $overlay.Digest -or $stamp.compiler -ne 'go1.26.10' -or $stamp.sha256 -ne (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash
}
if ($needsBuild) {
    # Keep pinned upstream modules unchanged; overlay SMB status and network accounting.
    $module = & $go -C $projectRoot mod download -json 'github.com/rclone/rclone@v1.75.2' | ConvertFrom-Json
    if ($LASTEXITCODE -or $module.Sum -cne $sourceSum) { throw 'Invalid rclone source checksum' }
    New-Item -ItemType Directory -Force -Path (Split-Path $binary -Parent) | Out-Null
    $previousCgo = $env:CGO_ENABLED
    try {
        $env:CGO_ENABLED = '0'
        Push-Location $overlay.RcloneDir
        try { & $go build -mod=readonly -modfile $overlay.ModFile -overlay $overlay.Path -trimpath -tags cmount -ldflags '-s -w -X github.com/rclone/rclone/fs.Version=v1.75.2' -o $binary . }
        finally { Pop-Location }
        if ($LASTEXITCODE) { throw 'rclone build failed' }
    } finally {
        if ($null -eq $previousCgo) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue }
        else { $env:CGO_ENABLED = $previousCgo }
    }
    @{ sourceSum=$sourceSum; overlayDigest=$overlay.Digest; compiler='go1.26.10'; sha256=(Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash } | ConvertTo-Json | Set-Content -LiteralPath $stampPath -Encoding utf8
}
Write-Output 'Local tools ready. Global PATH unchanged.'
