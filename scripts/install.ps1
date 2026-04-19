# Installs the latest onvif-simulator CLI binary from GitHub Releases.
# Usage: iex (irm https://github.com/GyeongHoKim/onvif-simulator/releases/latest/download/install.ps1)
$ErrorActionPreference = "Stop"

$Owner = "GyeongHoKim"
$Name = "onvif-simulator"
$Api = "https://api.github.com/repos/$Owner/$Name/releases/latest"

$release = Invoke-RestMethod -Uri $Api -Headers @{ "User-Agent" = "onvif-simulator-install" }
$tag = $release.tag_name
$version = $tag.TrimStart("v")

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "Unsupported PROCESSOR_ARCHITECTURE: $($env:PROCESSOR_ARCHITECTURE)" }
}

$file = "onvif-simulator_${version}_windows_${arch}.zip"
$asset = $release.assets | Where-Object { $_.name -eq $file } | Select-Object -First 1
if (-not $asset) {
    throw "Could not find release asset named $file. Available: $($release.assets.name -join ', ')"
}

$temp = Join-Path $env:TEMP ("onvif-simulator-" + [Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $temp | Out-Null
try {
    $zip = Join-Path $temp $file
    Write-Host "Downloading $file ..."
    Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $zip
    Expand-Archive -Path $zip -DestinationPath $temp -Force

    $binObj = Get-ChildItem -Path $temp -Filter onvif-simulator.exe -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $binObj) {
        throw "Could not find onvif-simulator.exe inside archive"
    }
    $bin = $binObj.FullName

    $destDir = Join-Path $env:LOCALAPPDATA "Programs\onvif-simulator"
    New-Item -ItemType Directory -Path $destDir -Force | Out-Null
    $dest = Join-Path $destDir "onvif-simulator.exe"
    Move-Item -Path $bin -Destination $dest -Force

    Write-Host "Installed: $dest"
    Write-Host "Add this folder to your PATH if needed: $destDir"
}
finally {
    Remove-Item -Recurse -Force $temp -ErrorAction SilentlyContinue
}
