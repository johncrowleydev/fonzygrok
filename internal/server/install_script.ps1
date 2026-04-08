# Fonzygrok client installer for Windows
# Usage: irm https://fonzygrok.com/install.ps1 | iex
#
# Installs the fonzygrok client to %LOCALAPPDATA%\Fonzygrok and adds it to PATH.

$ErrorActionPreference = "Stop"

$Binary = "fonzygrok.exe"
$InstallDir = "$env:LOCALAPPDATA\Fonzygrok"
$DownloadURL = "https://fonzygrok.com/download/fonzygrok.exe"

function Write-Step($msg) { Write-Host "  → " -ForegroundColor Cyan -NoNewline; Write-Host $msg }
function Write-Ok($msg)   { Write-Host "  ✔ " -ForegroundColor Green -NoNewline; Write-Host $msg }
function Write-Err($msg)  { Write-Host "  ✘ " -ForegroundColor Red -NoNewline; Write-Host $msg; exit 1 }

# --- Main ---
Write-Host ""
Write-Host "  Fonzygrok Installer" -ForegroundColor Cyan
Write-Host ""

Write-Step "Downloading $DownloadURL ..."

# Create install directory.
if (!(Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$dest = Join-Path $InstallDir $Binary
try {
    Invoke-WebRequest -Uri $DownloadURL -OutFile $dest -UseBasicParsing
} catch {
    Write-Err "Download failed: $_"
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
Write-Ok "Installed fonzygrok"
Write-Host ""
Write-Host "  Usage: fonzygrok --name my-app --port 3000 --token YOUR_TOKEN" -ForegroundColor White
Write-Host ""
