#Requires -Version 5.1
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$Repo    = "GyeongHoKim/onvif-simulator"
$Binary  = "onvif-simulator"
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\onvif-simulator" }

# Detect architecture
$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default {
        Write-Error "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE"
        exit 1
    }
}

# Fetch latest version
$Version = if ($env:VERSION) { $env:VERSION } else {
    $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
    $release.tag_name
}

if (-not $Version) {
    Write-Error "Failed to fetch latest version."
    exit 1
}

$VersionNum = $Version.TrimStart("v")
$Archive    = "${Binary}_${VersionNum}_windows_${Arch}.zip"
$Url        = "https://github.com/$Repo/releases/download/$Version/$Archive"

Write-Host "Installing $Binary $Version (windows/$Arch)..."

$Tmp = Join-Path $env:TEMP ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $Tmp | Out-Null

try {
    $ArchivePath = Join-Path $Tmp $Archive
    Invoke-WebRequest -Uri $Url -OutFile $ArchivePath -UseBasicParsing
    Expand-Archive -Path $ArchivePath -DestinationPath $Tmp

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }

    Copy-Item -Path (Join-Path $Tmp "${Binary}.exe") -Destination (Join-Path $InstallDir "${Binary}.exe") -Force

    # Add to PATH for current user if not already present
    $UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($UserPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
        Write-Host "Added $InstallDir to PATH. Restart your terminal to use $Binary."
    }

    Write-Host "$Binary $Version installed to $InstallDir\${Binary}.exe"
} finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}
