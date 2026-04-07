# Fonzygrok client installer for Windows
# Usage: irm https://fonzygrok.com/install.ps1 | iex
#
# Installs the fonzygrok client to %LOCALAPPDATA%\Fonzygrok and adds it to PATH.
#
# Environment variables:
#   FONZYGROK_VERSION  - version to install (default: latest)

$ErrorActionPreference = "Stop"

$Repo = "johncrowleydev/fonzygrok"
$Binary = "fonzygrok.exe"
$InstallDir = "$env:LOCALAPPDATA\Fonzygrok"

function Write-Step($msg) { Write-Host "  → " -ForegroundColor Cyan -NoNewline; Write-Host $msg }
function Write-Ok($msg)   { Write-Host "  ✔ " -ForegroundColor Green -NoNewline; Write-Host $msg }
function Write-Err($msg)  { Write-Host "  ✘ " -ForegroundColor Red -NoNewline; Write-Host $msg; exit 1 }

# --- Detect architecture ---
function Get-Arch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { Write-Err "Unsupported architecture: $arch" }
    }
}

# --- Resolve version ---
function Get-Version {
    if ($env:FONZYGROK_VERSION) { return $env:FONZYGROK_VERSION }

    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "fonzygrok-installer" }
        return $release.tag_name
    } catch {
        Write-Err "Could not determine latest version. Set FONZYGROK_VERSION manually."
    }
}

# --- Main ---
Write-Host ""
Write-Host "  Fonzygrok Installer" -ForegroundColor Cyan
Write-Host ""

$arch = Get-Arch
Write-Step "Detected: windows/$arch"

$version = Get-Version
Write-Step "Version: $version"

$asset = "fonzygrok-windows-$arch.exe"
$url = "https://github.com/$Repo/releases/download/$version/$asset"

Write-Step "Downloading $url ..."

# Create install directory.
if (!(Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$dest = Join-Path $InstallDir $Binary
try {
    Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing
} catch {
    Write-Err "Download failed. Check that $version has a release asset for windows/$arch."
}

Write-Ok "Downloaded to $dest"

# --- Add to PATH ---
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
    Write-Ok "Added $InstallDir to user PATH"
    Write-Host ""
    Write-Host "  ⚠ " -ForegroundColor Yellow -NoNewline
    Write-Host "Restart your terminal for PATH changes to take effect."
} else {
    Write-Ok "$InstallDir already in PATH"
}

Write-Host ""
Write-Ok "Installed fonzygrok $version"
Write-Host ""
Write-Host "  Run: fonzygrok --name my-app --port 3000" -ForegroundColor White
Write-Host ""
