# fetch-mtxrpicam.ps1 — PowerShell port of fetch-mtxrpicam.sh for Windows
# developers (no bash required). Same checksum + tarball contract as the .sh.
#
# Usage:
#   pwsh -NoProfile -File scripts/fetch-mtxrpicam.ps1 -WordSize 32 -Version v2.5.6 -DestDir internal/rpicamera/mtxrpicam_32

param(
    [Parameter(Mandatory)][ValidateSet('32', '64')][string]$WordSize,
    [Parameter(Mandatory)][string]$Version,
    [Parameter(Mandatory)][string]$DestDir
)

$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$shaFile = Join-Path $repoRoot 'scripts/mtxrpicam.sha256'
$tarName = "mtxrpicam_${WordSize}.tar.gz"
$srcDir = "mtxrpicam_${WordSize}"
$destDirAbs = [System.IO.Path]::GetFullPath((Join-Path (Get-Location).Path $DestDir))
$destBin = Join-Path $destDirAbs 'mtxrpicam'

if (-not (Test-Path -LiteralPath $shaFile)) {
    Write-Error "missing $shaFile — pin a checksum before fetching"
}

$expectedSha = $null
foreach ($line in [System.IO.File]::ReadLines($shaFile)) {
    $t = $line.Trim()
    if ($t.StartsWith('#') -or $t -eq '') {
        continue
    }
    $parts = $t -split '\s+', 2
    if ($parts.Count -ge 2 -and $parts[1] -eq $tarName) {
        $expectedSha = $parts[0].ToLowerInvariant()
        break
    }
}
if (-not $expectedSha) {
    Write-Error "no SHA-256 entry for $tarName in $shaFile"
}

if (Test-Path -LiteralPath $destBin) {
    Write-Host "fetch-mtxrpicam: $destBin already present, trusting prior install"
    exit 0
}

$destBase = Split-Path -Path $destDirAbs -Leaf
if ($destBase -ne $srcDir) {
    Write-Error "expected dest_dir basename $srcDir, got $destBase"
}

New-Item -ItemType Directory -Path $destDirAbs -Force | Out-Null
Get-ChildItem -LiteralPath $destDirAbs -Recurse -File -Filter 'placeholder' -ErrorAction SilentlyContinue |
    Remove-Item -Force -ErrorAction SilentlyContinue

$url = "https://github.com/bluenviron/mediamtx-rpicamera/releases/download/${Version}/${tarName}"
$tmpDir = New-Item -ItemType Directory -Path ([System.IO.Path]::Combine(
        [System.IO.Path]::GetTempPath(),
        [System.IO.Path]::GetRandomFileName()))
$tmpTar = Join-Path $tmpDir $tarName

try {
    Write-Host "fetch-mtxrpicam: downloading $url"
    if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
        & curl.exe -fsSL $url -o $tmpTar
    } elseif (Get-Command wget.exe -ErrorAction SilentlyContinue) {
        & wget.exe -q $url -O $tmpTar
    } else {
        Invoke-WebRequest -Uri $url -OutFile $tmpTar -UseBasicParsing
    }

    $actual = (Get-FileHash -LiteralPath $tmpTar -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expectedSha) {
        Write-Error "checksum mismatch for $tarName`n  expected: $expectedSha`n  got:      $actual"
    }

    $destParent = Split-Path -Path $destDirAbs -Parent
    if (Test-Path -LiteralPath $destDirAbs) {
        Remove-Item -LiteralPath $destDirAbs -Recurse -Force
    }
    & tar -xzf $tmpTar -C $destParent
    Write-Host "fetch-mtxrpicam: $destBin ready (tarball $actual)"
} finally {
    Remove-Item -LiteralPath $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
}
