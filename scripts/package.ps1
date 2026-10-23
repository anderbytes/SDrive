$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
Push-Location $projectRoot
try {
    & "$PSScriptRoot/prerequisites.ps1"
    & "$projectRoot/.tools/go/bin/go.exe" test ./internal/platform -run TestOfficialPrerequisiteSignatures -count=1
    if ($LASTEXITCODE) { throw 'Native component signature verification failed' }
    $toolsDir = Join-Path $projectRoot '.tools'
    $compiler = Join-Path $toolsDir 'nsis/nsis-3.12/makensis.exe'
    if (!(Test-Path -LiteralPath $compiler)) {
        # SourceForge is sometimes blocked; MacPorts mirrors the unchanged release archive.
        $archive = Join-Path $toolsDir 'nsis-3.12.zip'
        if (!(Test-Path -LiteralPath $archive)) { Invoke-WebRequest 'https://distfiles.macports.org/nsis/nsis-3.12.zip' -OutFile $archive }
        if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne '56581f90db321581c5381193d796fffcf2d24b2f8fed2160a6c6a3baa67f2c4f') { throw 'Invalid NSIS checksum' }
        Expand-Archive -LiteralPath $archive -DestinationPath "$toolsDir/nsis" -Force
    }
    node scripts/licenses.mjs
    if ($LASTEXITCODE) { throw 'License collection failed' }
    Copy-Item -LiteralPath '.tools/prerequisites/WinFsp-LICENSE.txt' -Destination 'bin/licenses/WinFsp-LICENSE.txt' -Force
    Copy-Item -LiteralPath 'installer/prerequisites.json' -Destination 'bin/licenses/prerequisites.json' -Force
    $sourceFiles = @('main.go','main_test.go','mount_integration_test.go','go.mod','go.sum','internal','frontend/src','frontend/index.html','frontend/package.json','frontend/package-lock.json','frontend/tsconfig.json','frontend/vitest.config.ts','scripts','docs','installer','LICENSE','README.md','THIRD_PARTY_NOTICES.md','.github','.gitignore')
    $sourceStage = Join-Path $toolsDir ('sdrive-source-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $sourceStage | Out-Null
    foreach ($sourceFile in $sourceFiles) {
        $targetPath = Join-Path $sourceStage $sourceFile
        $targetParent = Split-Path $targetPath -Parent
        New-Item -ItemType Directory -Force -Path $targetParent | Out-Null
        Copy-Item -LiteralPath $sourceFile -Destination $targetPath -Recurse
    }
    Compress-Archive -Path "$sourceStage/*" -DestinationPath 'bin/SDrive-source.zip' -Force
    & $compiler /INPUTCHARSET UTF8 /V2 installer/sdrive.nsi
    if ($LASTEXITCODE) { throw 'Online installer compilation failed' }
    & $compiler /DOFFLINE /INPUTCHARSET UTF8 /V2 installer/sdrive.nsi
    if ($LASTEXITCODE) { throw 'Offline installer compilation failed' }
    $artifacts = @('bin/SDrive.exe','bin/rclone.exe','bin/SDrive-0.1.0-setup.exe','bin/SDrive-0.1.0-offline-setup.exe','bin/SDrive-source.zip','bin/rclone-source.zip')
    $hashLines = foreach ($artifact in $artifacts) { $hash = Get-FileHash -LiteralPath $artifact -Algorithm SHA256; "$($hash.Hash.ToLower())  $([IO.Path]::GetFileName($artifact))" }
    $hashLines | Set-Content -LiteralPath 'bin/SHA256SUMS' -Encoding utf8
    Get-Item -LiteralPath $artifacts[2],$artifacts[3] | ForEach-Object { Write-Output "$($_.Name): $([Math]::Round($_.Length / 1MB, 1)) MiB" }
} finally { Pop-Location }
