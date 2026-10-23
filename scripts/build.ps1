param([switch]$Installer, [switch]$Check)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
Push-Location $projectRoot
try {
    & "$PSScriptRoot/bootstrap.ps1"
    $go = Join-Path $projectRoot '.tools/go/bin/go.exe'
    Push-Location frontend
    try {
        npm ci
        if ($LASTEXITCODE) { throw 'npm ci failed' }
        if ($Check) { npm test; if ($LASTEXITCODE) { throw 'Interface tests failed' } }
        npm run build
        if ($LASTEXITCODE) { throw 'Interface build failed' }
    } finally { Pop-Location }
    if ($Check) {
        node --test scripts/ui-smoke-runtime.test.mjs
        if ($LASTEXITCODE) { throw 'Native smoke harness tests failed' }
        New-Item -ItemType Directory -Force -Path '.tools/check-results' | Out-Null
        & $go test -json ./... | Tee-Object -FilePath '.tools/check-results/go-tests.jsonl' | ForEach-Object {
            $event = $_ | ConvertFrom-Json
            if ($event.Action -eq 'output') { Write-Host $event.Output -NoNewline }
        }
        if ($LASTEXITCODE) { throw 'Go tests failed' }
    }
    New-Item -ItemType Directory -Force -Path bin | Out-Null
    & $go build -trimpath -tags production -ldflags '-s -w -H windowsgui' -o bin/SDrive.exe .
    if ($LASTEXITCODE) { throw 'Go build failed' }
    Copy-Item -LiteralPath '.tools/rclone/rclone.exe' -Destination 'bin/rclone.exe' -Force
    & $go build -trimpath -tags production -o bin/SDrive-check.exe .
    if ($LASTEXITCODE) { throw 'Diagnostic build failed' }
    & './bin/SDrive-check.exe' --self-check
    if ($LASTEXITCODE) { throw 'Integrated check failed' }
    if ($Installer) {
        & "$PSScriptRoot/package.ps1"
    }
    Write-Output "Build ready at $projectRoot/bin"
} finally { Pop-Location }
