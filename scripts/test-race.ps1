$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$compiler = & "$PSScriptRoot/prepare-compiler.ps1"
$previousCgo = $env:CGO_ENABLED
$previousCC = $env:CC
Push-Location $projectRoot
try {
    $env:CGO_ENABLED = '1'
    $env:CC = $compiler
    & './.tools/go/bin/go.exe' test -race ./internal/...
    if ($LASTEXITCODE) { throw 'Race-detector tests failed' }
} finally {
    $env:CGO_ENABLED = $previousCgo
    $env:CC = $previousCC
    Pop-Location
}
