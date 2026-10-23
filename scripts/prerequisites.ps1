$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$destination = Join-Path $projectRoot '.tools/prerequisites'
New-Item -ItemType Directory -Force -Path $destination | Out-Null
$manifest = Get-Content -LiteralPath (Join-Path $projectRoot 'installer/prerequisites.json') -Raw | ConvertFrom-Json
foreach ($item in $manifest) {
    $file = Join-Path $destination $item.file
    $download = "$file.download"
    if (!(Test-Path -LiteralPath $file)) {
        Invoke-WebRequest -Uri $item.url -OutFile $download
        if ((Get-FileHash -LiteralPath $download -Algorithm SHA256).Hash -ne $item.sha256) { throw "Invalid checksum: $($item.file)" }
        Move-Item -LiteralPath $download -Destination $file -Force
    }
    if ((Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash -ne $item.sha256) { throw "Invalid checksum: $($item.file)" }
    $signature = Get-AuthenticodeSignature -LiteralPath $file
    if ($signature.Status -ne 'Valid' -or !$signature.SignerCertificate.Subject.StartsWith($item.publisher, [StringComparison]::Ordinal)) {
        throw "Invalid signature or publisher: $($item.file)"
    }
}
$license = Join-Path $destination 'WinFsp-LICENSE.txt'
if (!(Test-Path -LiteralPath $license)) { Invoke-WebRequest 'https://raw.githubusercontent.com/winfsp/winfsp/v2.1/License.txt' -OutFile $license }
if ((Get-FileHash -LiteralPath $license -Algorithm SHA256).Hash -ne '68ae2fa9d8e6110ebdc9a236beecf118e279f8a26530daf09c9e682c231f7879') { throw 'Invalid WinFsp license' }
Write-Output 'Componentes offline conferidos: SHA-256 e assinatura Authenticode.'
