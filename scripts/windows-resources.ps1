$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$compiler = & "$PSScriptRoot/prepare-compiler.ps1"
$windres = Join-Path (Split-Path $compiler -Parent) 'windres.exe'
$version = (Get-Content -LiteralPath "$projectRoot/frontend/package.json" -Raw | ConvertFrom-Json).version
if ($version -cnotmatch '^\d+\.\d+\.\d+$') { throw 'Windows resources require a stable three-part version' }
$template = Get-Content -LiteralPath "$projectRoot/assets/sdrive.rc" -Raw
$resource = $template.Replace('@VERSION@', $version).Replace('@VERSION_NUMBER@', ($version.Replace('.', ',') + ',0'))
$resourcePath = Join-Path $projectRoot '.tools/sdrive.rc'
$resource | Set-Content -LiteralPath $resourcePath -Encoding ascii
& $windres --input $resourcePath --include-dir "$projectRoot/assets" --output "$projectRoot/resource_windows_amd64.syso" --output-format coff --target pe-x86-64
if ($LASTEXITCODE) { throw 'Windows resource compilation failed' }
