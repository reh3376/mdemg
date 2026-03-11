#!/usr/bin/env pwsh
#Requires -Version 7.0
<#
.SYNOPSIS
    MDEMG Windows Installer
    Equivalent of: brew tap reh3376/mdemg && brew install mdemg

.DESCRIPTION
    Installs the MDEMG CLI and all prerequisites on Windows.
    Downloads the native mdemg.exe binary from GitHub releases,
    sets up PATH so 'mdemg' is available globally, and configures
    tab-completion.

.PARAMETER Upgrade
    Re-download and replace the current installation.

.PARAMETER Uninstall
    Remove MDEMG CLI from the system.

.PARAMETER InstallDir
    Override installation directory (default: %USERPROFILE%\mdemg)

.EXAMPLE
    # One-liner install (run in PowerShell 7+):
    irm https://raw.githubusercontent.com/reh3376/mdemg-windows/main/Install-MDEMG.ps1 | iex

    # Or with explicit flags:
    .\Install-MDEMG.ps1 -Upgrade
    .\Install-MDEMG.ps1 -Uninstall
#>
[CmdletBinding()]
param(
    [switch]$Upgrade,
    [switch]$Uninstall,
    [string]$InstallDir = (Join-Path $env:USERPROFILE "mdemg")
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ──────────────────────────────────────────────────────────────────────────────
# Config
# ──────────────────────────────────────────────────────────────────────────────
$GITHUB_REPO        = "reh3376/mdemg"
$GITHUB_WINDOWS_REPO= "reh3376/mdemg-windows"
$MIN_PWSH_VERSION   = [Version]"7.0"

# ──────────────────────────────────────────────────────────────────────────────
# Helpers
# ──────────────────────────────────────────────────────────────────────────────
function Write-Step   { param([string]$Msg) Write-Host "`n  ► $Msg" -ForegroundColor Cyan }
function Write-Ok     { param([string]$Msg) Write-Host "  ✓ $Msg" -ForegroundColor Green }
function Write-Warn   { param([string]$Msg) Write-Host "  ⚠ $Msg" -ForegroundColor Yellow }
function Write-Err    { param([string]$Msg) Write-Host "  ✗ $Msg" -ForegroundColor Red }
function Write-Info   { param([string]$Msg) Write-Host "  · $Msg" -ForegroundColor Gray }
function Write-Banner {
    Write-Host ""
    Write-Host "  ╔═══════════════════════════════════════════════╗" -ForegroundColor Cyan
    Write-Host "  ║   MDEMG — Multi-Dimensional Emergent Memory   ║" -ForegroundColor Cyan
    Write-Host "  ║          Graph  ·  Windows Installer           ║" -ForegroundColor Cyan
    Write-Host "  ╚═══════════════════════════════════════════════╝" -ForegroundColor Cyan
    Write-Host ""
}

function Add-ToUserPath {
    param([string]$Dir)
    $currentPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
    if ($currentPath -notlike "*$Dir*") {
        [System.Environment]::SetEnvironmentVariable("PATH", "$currentPath;$Dir", "User")
        $env:PATH += ";$Dir"
        Write-Ok "Added to user PATH: $Dir"
    } else {
        Write-Info "Already in PATH: $Dir"
    }
}

function Remove-FromUserPath {
    param([string]$Dir)
    $currentPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
    $newPath = ($currentPath -split ";" | Where-Object { $_ -ne $Dir }) -join ";"
    [System.Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
    Write-Ok "Removed from PATH: $Dir"
}

function Get-LatestGitHubRelease {
    param([string]$Repo)
    $url = "https://api.github.com/repos/$Repo/releases/latest"
    try {
        $release = Invoke-RestMethod -Uri $url -Headers @{ "User-Agent" = "mdemg-installer" } -TimeoutSec 15
        return $release.tag_name
    } catch {
        return $null
    }
}

function Get-ArchSuffix {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64"   { return "amd64" }
        "ARM64"   { return "arm64" }
        default   { return "amd64" }
    }
}

# ──────────────────────────────────────────────────────────────────────────────
# Prerequisite Checks
# ──────────────────────────────────────────────────────────────────────────────
function Test-Prerequisites {
    Write-Step "Checking prerequisites"
    $allOk = $true

    # PowerShell 7+
    $psVer = $PSVersionTable.PSVersion
    if ($psVer -ge $MIN_PWSH_VERSION) {
        Write-Ok "PowerShell $psVer"
    } else {
        Write-Err "PowerShell $MIN_PWSH_VERSION+ required (found $psVer)"
        Write-Info "Install: winget install Microsoft.PowerShell"
        $allOk = $false
    }

    # Docker Desktop
    $docker = Get-Command docker -ErrorAction SilentlyContinue
    if ($docker) {
        $dockerVer = (docker --version 2>&1) -replace "Docker version ", "" -split "," | Select-Object -First 1
        Write-Ok "Docker $dockerVer"
    } else {
        Write-Warn "Docker Desktop not found (required for Neo4j)"
        Write-Info "Install: winget install Docker.DockerDesktop"
    }

    # Git (optional)
    $git = Get-Command git -ErrorAction SilentlyContinue
    if ($git) {
        Write-Ok "Git $((git --version 2>&1) -replace 'git version ', '')"
    } else {
        Write-Warn "Git not found (needed for git hooks)"
        Write-Info "Install: winget install Git.Git"
    }

    if (!$allOk) {
        Write-Err "One or more required prerequisites are missing."
        exit 1
    }
}

# ──────────────────────────────────────────────────────────────────────────────
# Download & Install MDEMG Binary
# ──────────────────────────────────────────────────────────────────────────────
function Install-MdemgCli {
    param([string]$InstallDir)

    Write-Step "Installing MDEMG CLI to $InstallDir"

    if (!(Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    }

    $latestTag = Get-LatestGitHubRelease -Repo $GITHUB_REPO
    if (!$latestTag) {
        Write-Err "Could not determine latest release from GitHub"
        exit 1
    }

    Write-Info "Latest release: $latestTag"

    $version = $latestTag.TrimStart("v")
    $arch = Get-ArchSuffix
    $zipName = "mdemg_${version}_windows_${arch}.zip"
    $binaryUrl = "https://github.com/$GITHUB_REPO/releases/download/$latestTag/$zipName"
    $zipPath = Join-Path $InstallDir $zipName
    $binaryPath = Join-Path $InstallDir "mdemg.exe"

    # Download zip
    try {
        Write-Info "Downloading $zipName..."
        Invoke-WebRequest -Uri $binaryUrl -OutFile $zipPath -TimeoutSec 120
    } catch {
        Write-Err "Failed to download Windows binary: $_"
        Write-Info "URL: $binaryUrl"
        Write-Info "Windows builds may not be available for this release yet."
        Write-Info "Check: https://github.com/$GITHUB_REPO/releases"
        exit 1
    }

    # Verify checksum (if checksums.txt is available)
    try {
        $checksumsUrl = "https://github.com/$GITHUB_REPO/releases/download/$latestTag/checksums.txt"
        $checksums = (Invoke-WebRequest -Uri $checksumsUrl -TimeoutSec 15).Content
        $localHash = (Get-FileHash $zipPath -Algorithm SHA256).Hash.ToLower()
        $expectedLine = $checksums -split "`n" | Where-Object { $_ -like "*$zipName*" }
        if ($expectedLine) {
            $expectedHash = ($expectedLine -split "\s+")[0].ToLower()
            if ($localHash -eq $expectedHash) {
                Write-Ok "Checksum verified: $($localHash.Substring(0,16))..."
            } else {
                Write-Err "Checksum mismatch! Expected: $expectedHash, Got: $localHash"
                Remove-Item $zipPath -Force
                exit 1
            }
        } else {
            Write-Warn "No checksum found for $zipName — skipping verification"
        }
    } catch {
        Write-Warn "Could not verify checksum (non-fatal): $_"
    }

    # Extract
    if (Test-Path $binaryPath) { Remove-Item $binaryPath -Force }
    Expand-Archive -Path $zipPath -DestinationPath $InstallDir -Force
    Remove-Item $zipPath -Force

    if (Test-Path $binaryPath) {
        Write-Ok "Binary installed: $binaryPath"
    } else {
        Write-Err "mdemg.exe not found after extraction"
        exit 1
    }

    # Install PowerShell wrapper
    $wrapperUrl  = "https://raw.githubusercontent.com/$GITHUB_WINDOWS_REPO/main/scripts/mdemg.ps1"
    $wrapperPath = Join-Path $InstallDir "mdemg-wrapper.ps1"
    try {
        Invoke-WebRequest -Uri $wrapperUrl -OutFile $wrapperPath -TimeoutSec 15
        Write-Ok "PowerShell wrapper installed"
    } catch {
        Write-Info "Could not download PowerShell wrapper (non-fatal)"
    }

    # Create tab-completion profile shim
    $profileShim = Join-Path $InstallDir "mdemg-profile.ps1"
    @"
# MDEMG tab-completion — source this in your `$PROFILE
Register-ArgumentCompleter -CommandName mdemg -Native -ScriptBlock {
    param(`$wordToComplete, `$commandAst, `$cursorPosition)
    @('init','version','start','stop','restart','serve','status',
      'db','ingest','watch','consolidate','decay','prune',
      'config','embeddings','hooks','mcp','space','demo','upgrade','help') |
    Where-Object { `$_ -like "`$wordToComplete*" } |
    ForEach-Object { [System.Management.Automation.CompletionResult]::new(`$_,`$_,'ParameterValue',`$_) }
}
"@ | Set-Content $profileShim
    Write-Ok "Tab-completion shim created"

    return $latestTag
}

# ──────────────────────────────────────────────────────────────────────────────
# PATH & Profile Setup
# ──────────────────────────────────────────────────────────────────────────────
function Setup-Path {
    param([string]$InstallDir)
    Write-Step "Configuring PATH"
    Add-ToUserPath -Dir $InstallDir
}

function Setup-Profile {
    param([string]$InstallDir)
    Write-Step "PowerShell profile integration"

    $profileShim = Join-Path $InstallDir "mdemg-profile.ps1"
    $sourceLine  = ". `"$profileShim`""

    $profilePath = $PROFILE.CurrentUserAllHosts
    $profileDir  = Split-Path $profilePath

    if (!(Test-Path $profileDir)) { New-Item -ItemType Directory -Force $profileDir | Out-Null }
    if (!(Test-Path $profilePath)) { New-Item -ItemType File -Force $profilePath | Out-Null }

    $existing = Get-Content $profilePath -Raw -ErrorAction SilentlyContinue
    if ($existing -and $existing -like "*mdemg-profile*") {
        Write-Info "Profile already sources mdemg-profile.ps1"
    } else {
        Add-Content -Path $profilePath -Value "`n# MDEMG tab-completion`n$sourceLine"
        Write-Ok "Added mdemg tab-completion to $profilePath"
    }
}

# ──────────────────────────────────────────────────────────────────────────────
# Version File
# ──────────────────────────────────────────────────────────────────────────────
function Write-VersionFile {
    param([string]$InstallDir, [string]$Version)
    $versionFile = Join-Path $InstallDir "VERSION"
    @{
        version      = $Version
        install_dir  = $InstallDir
        installed_at = (Get-Date -Format "o")
        platform     = "windows"
        arch         = (Get-ArchSuffix)
        pwsh         = $PSVersionTable.PSVersion.ToString()
    } | ConvertTo-Json | Set-Content $versionFile
}

# ──────────────────────────────────────────────────────────────────────────────
# Uninstall
# ──────────────────────────────────────────────────────────────────────────────
function Invoke-Uninstall {
    param([string]$InstallDir)
    Write-Step "Uninstalling MDEMG"

    # Stop server if running
    $pidFile = Join-Path $env:USERPROFILE ".mdemg\mdemg.pid"
    if (Test-Path $pidFile) {
        $procId = (Get-Content $pidFile).Trim()
        Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
        Write-Ok "Server stopped"
    }

    # Remove install dir
    if (Test-Path $InstallDir) {
        Remove-Item $InstallDir -Recurse -Force
        Write-Ok "Removed: $InstallDir"
    }

    # Remove from PATH
    Remove-FromUserPath -Dir $InstallDir

    # Remove profile line
    if (Test-Path $PROFILE.CurrentUserAllHosts) {
        $content = Get-Content $PROFILE.CurrentUserAllHosts -Raw
        $cleaned = $content -replace "(?m)^# MDEMG tab-completion\r?\n.*mdemg-profile.*\r?\n?", ""
        Set-Content $PROFILE.CurrentUserAllHosts $cleaned
        Write-Ok "Removed from PowerShell profile"
    }

    Write-Host ""
    Write-Host "  MDEMG uninstalled." -ForegroundColor Green
    Write-Host "  Config and data remain at: $($env:USERPROFILE)\.mdemg" -ForegroundColor Gray
    Write-Host "  To remove data: Remove-Item `"$($env:USERPROFILE)\.mdemg`" -Recurse -Force" -ForegroundColor Gray
    Write-Host ""
}

# ──────────────────────────────────────────────────────────────────────────────
# Post-Install
# ──────────────────────────────────────────────────────────────────────────────
function Show-PostInstall {
    param([string]$InstallDir, [string]$Version)

    Write-Host ""
    Write-Host "  ╔═══════════════════════════════════════════════╗" -ForegroundColor Green
    Write-Host "  ║          MDEMG installed successfully!         ║" -ForegroundColor Green
    Write-Host "  ╚═══════════════════════════════════════════════╝" -ForegroundColor Green
    Write-Host ""
    Write-Host "  Version: $Version" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  Quick Start:" -ForegroundColor White
    Write-Host ""
    Write-Host "    # Restart your terminal first, then:" -ForegroundColor DarkGray
    Write-Host "    mdemg version" -ForegroundColor White
    Write-Host ""
    Write-Host "    # One-command setup:" -ForegroundColor DarkGray
    Write-Host '    $env:OPENAI_API_KEY = "sk-..."' -ForegroundColor White
    Write-Host "    mdemg init --quick" -ForegroundColor White
    Write-Host ""
    Write-Host "    # Step-by-step:" -ForegroundColor DarkGray
    Write-Host "    mdemg init                  # Interactive wizard" -ForegroundColor White
    Write-Host "    mdemg db start              # Start Neo4j container" -ForegroundColor White
    Write-Host "    mdemg start --auto-migrate  # Start server" -ForegroundColor White
    Write-Host "    mdemg status                # Verify everything" -ForegroundColor White
    Write-Host "    mdemg ingest --path .       # Ingest your codebase" -ForegroundColor White
    Write-Host ""
    Write-Host "  Installed to: $InstallDir" -ForegroundColor DarkGray
    Write-Host "  Docs: https://github.com/reh3376/homebrew-mdemg" -ForegroundColor DarkGray
    Write-Host ""
}

# ──────────────────────────────────────────────────────────────────────────────
# Main
# ──────────────────────────────────────────────────────────────────────────────
Write-Banner

if ($Uninstall) {
    Invoke-Uninstall -InstallDir $InstallDir
    exit 0
}

if ($Upgrade) {
    Write-Step "Upgrading MDEMG"
    $versionFile = Join-Path $InstallDir "VERSION"
    if (Test-Path $versionFile) {
        $current = (Get-Content $versionFile | ConvertFrom-Json).version
        Write-Info "Current version: $current"
    }
}

Test-Prerequisites
$installedTag = Install-MdemgCli -InstallDir $InstallDir
$version = if ($installedTag) { $installedTag.TrimStart("v") } else { "unknown" }

if (!$Upgrade) {
    Setup-Path -InstallDir $InstallDir
    Setup-Profile -InstallDir $InstallDir
}

Write-VersionFile -InstallDir $InstallDir -Version $version
Show-PostInstall -InstallDir $InstallDir -Version $version
