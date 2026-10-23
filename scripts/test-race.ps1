$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$archive = Join-Path $projectRoot '.tools/w64devkit-x64-2.10.1.7z.exe'
$compiler = Join-Path $projectRoot '.tools/w64devkit/bin/gcc.exe'
if (!(Test-Path -LiteralPath $archive)) {
    Invoke-WebRequest 'https://github.com/skeeto/w64devkit/releases/download/v2.10.1/w64devkit-x64-2.10.1.7z.exe' -OutFile $archive
}
if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne 'add1edf984ce4d473cfeb7c52e0ab38db73a855bba6c7f564d5e16648eb2896a') {
    throw 'Invalid race-detector compiler checksum'
}
if (!(Test-Path -LiteralPath $compiler)) {
    $destination = Join-Path $projectRoot '.tools'
    $process = Start-Process -FilePath $archive -ArgumentList '-y', ('-o"' + $destination + '"') -WindowStyle Hidden -Wait -PassThru
    if ($process.ExitCode) { throw 'Compiler extraction failed' }
}
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
