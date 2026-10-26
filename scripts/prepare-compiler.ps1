$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$archive = Join-Path $projectRoot '.tools/w64devkit-x64-2.10.1.7z.exe'
$compiler = Join-Path $projectRoot '.tools/w64devkit/bin/gcc.exe'
New-Item -ItemType Directory -Force -Path (Join-Path $projectRoot '.tools') | Out-Null
if (!(Test-Path -LiteralPath $archive)) {
    Invoke-WebRequest 'https://github.com/skeeto/w64devkit/releases/download/v2.10.1/w64devkit-x64-2.10.1.7z.exe' -OutFile $archive
}
if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne 'add1edf984ce4d473cfeb7c52e0ab38db73a855bba6c7f564d5e16648eb2896a') {
    throw 'Invalid Windows compiler checksum'
}
if (!(Test-Path -LiteralPath $compiler)) {
    $destination = Join-Path $projectRoot '.tools'
    $process = Start-Process -FilePath $archive -ArgumentList '-y', ('-o"' + $destination + '"') -WindowStyle Hidden -Wait -PassThru
    if ($process.ExitCode) { throw 'Compiler extraction failed' }
}
Write-Output $compiler
