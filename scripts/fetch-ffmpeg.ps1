# fetch-ffmpeg.ps1 — PowerShell port of fetch-ffmpeg.sh for Windows (no bash).
#
# Usage:
#   pwsh -NoProfile -File scripts/fetch-ffmpeg.ps1 -Goos linux -Goarch amd64

param(
    [Parameter(Mandatory)][string]$Goos,
    [Parameter(Mandatory)][string]$Goarch
)

$ErrorActionPreference = 'Stop'

$tuple = "${Goos}_${Goarch}"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$shaFile = Join-Path $repoRoot 'scripts/ffmpeg.sha256'
$destDir = Join-Path $repoRoot "internal/ffmpeg/binaries/${Goos}_${Goarch}"
$destBinName = if ($Goos -eq 'windows') { 'ffmpeg.exe' } else { 'ffmpeg' }
$destBin = Join-Path $destDir $destBinName

if (Test-Path -LiteralPath $destBin) {
    $size = (Get-Item -LiteralPath $destBin).Length
    if ($size -gt 1048576) {
        Write-Host "fetch-ffmpeg: $destBin already present (${size} bytes), trusting prior install"
        exit 0
    }
}

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
    if ($parts.Count -ge 2 -and $parts[1] -eq $tuple) {
        $expectedSha = $parts[0].ToLowerInvariant()
        break
    }
}
if (-not $expectedSha) {
    Write-Error "no SHA-256 entry for $tuple in $shaFile"
}

$url = $null
$archive = $null
switch ($tuple) {
    'linux_amd64' {
        $url = 'https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz'
        $archive = 'ffmpeg-release-amd64-static.tar.xz'
    }
    'linux_arm64' {
        $url = 'https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-arm64-static.tar.xz'
        $archive = 'ffmpeg-release-arm64-static.tar.xz'
    }
    'linux_arm' {
        $url = 'https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-armhf-static.tar.xz'
        $archive = 'ffmpeg-release-armhf-static.tar.xz'
    }
    'darwin_amd64' {
        $url = 'https://evermeet.cx/ffmpeg/ffmpeg-7.1.zip'
        $archive = 'ffmpeg-darwin-amd64.zip'
    }
    'darwin_arm64' {
        $url = 'https://www.osxexperts.net/ffmpeg71arm.zip'
        $archive = 'ffmpeg-darwin-arm64.zip'
    }
    'windows_amd64' {
        $url = 'https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-lgpl.zip'
        $archive = 'ffmpeg-windows-amd64.zip'
    }
    'windows_arm64' {
        $url = 'https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-winarm64-lgpl.zip'
        $archive = 'ffmpeg-windows-arm64.zip'
    }
    default {
        Write-Error "unsupported (goos, goarch): ${tuple}"
    }
}

New-Item -ItemType Directory -Path $destDir -Force | Out-Null
$tmpDir = New-Item -ItemType Directory -Path ([System.IO.Path]::Combine(
        [System.IO.Path]::GetTempPath(),
        [System.IO.Path]::GetRandomFileName()))
$archivePath = Join-Path $tmpDir $archive

try {
    Write-Host "fetch-ffmpeg: downloading $url"
    if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
        & curl.exe -fsSL $url -o $archivePath
    } elseif (Get-Command wget.exe -ErrorAction SilentlyContinue) {
        & wget.exe -q $url -O $archivePath
    } else {
        Invoke-WebRequest -Uri $url -OutFile $archivePath -UseBasicParsing
    }

    $actual = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expectedSha) {
        Write-Error "checksum mismatch for ${tuple}`n  expected: $expectedSha`n  got:      $actual"
    }

    switch -Wildcard ($archive) {
        '*.tar.xz' {
            & tar -C $tmpDir -xJf $archivePath
        }
        '*.zip' {
            Expand-Archive -LiteralPath $archivePath -DestinationPath $tmpDir -Force
        }
        default {
            Write-Error "unhandled archive format: $archive"
        }
    }

    $src = $null
    switch ($tuple) {
        'linux_amd64' {
            $src = Get-ChildItem -Path $tmpDir -Recurse -File -Filter 'ffmpeg' |
                Where-Object { $_.FullName -match 'amd64-static' } |
                Select-Object -First 1
        }
        'linux_arm64' {
            $src = Get-ChildItem -Path $tmpDir -Recurse -File -Filter 'ffmpeg' |
                Where-Object { $_.FullName -match 'arm64-static' } |
                Select-Object -First 1
        }
        'linux_arm' {
            $src = Get-ChildItem -Path $tmpDir -Recurse -File -Filter 'ffmpeg' |
                Where-Object { $_.FullName -match 'armhf-static' } |
                Select-Object -First 1
        }
        'darwin_amd64' {
            $src = Get-ChildItem -Path $tmpDir -Recurse -File -Filter 'ffmpeg' |
                Select-Object -First 1
        }
        'darwin_arm64' {
            $src = Get-ChildItem -Path $tmpDir -Recurse -File -Filter 'ffmpeg' |
                Select-Object -First 1
        }
        'windows_amd64' {
            $src = Get-ChildItem -Path $tmpDir -Recurse -File -Filter 'ffmpeg.exe' |
                Where-Object { $_.Directory.Name -eq 'bin' } |
                Select-Object -First 1
        }
        'windows_arm64' {
            $src = Get-ChildItem -Path $tmpDir -Recurse -File -Filter 'ffmpeg.exe' |
                Where-Object { $_.Directory.Name -eq 'bin' } |
                Select-Object -First 1
        }
    }

    if ($null -eq $src -or -not (Test-Path -LiteralPath $src.FullName)) {
        $src = Get-ChildItem -Path $tmpDir -Recurse -File -Filter $destBinName |
            Select-Object -First 1
    }

    if ($null -eq $src -or -not (Test-Path -LiteralPath $src.FullName)) {
        Write-Error "could not locate $destBinName inside $archive"
    }

    Move-Item -LiteralPath $src.FullName -Destination $destBin -Force
    Write-Host "fetch-ffmpeg: $destBin ready (archive $actual)"
} finally {
    Remove-Item -LiteralPath $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
}
