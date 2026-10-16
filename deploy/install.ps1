<#
.SYNOPSIS
    Zev installer for Windows — downloads a precompiled binary and installs it.

.DESCRIPTION
    Downloads the zev archive for this machine, verifies its SHA-256 checksum,
    extracts it and installs zev into the install directory.

    Run it from PowerShell:
        irm https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.ps1 | iex

    Or download the file first and run it:
        .\install.ps1

.PARAMETER Version
    Install a specific version. By default the version is read from the VERSION
    file in the repository.

.PARAMETER Prefix
    Installation directory. Defaults to $env:LOCALAPPDATA\Programs\zev, which
    needs no administrator rights.

.EXAMPLE
    irm https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.ps1 | iex

.EXAMPLE
    .\install.ps1 -Version 1.0.0
#>

# Write-Host is intentional: this script's purpose is showing installer progress
# to a human, not emitting objects to a pipeline.
[CmdletBinding()]
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '')]
param(
    [string]$Version = $env:ZEV_VERSION,
    [string]$Prefix = $env:ZEV_PREFIX
)

$ErrorActionPreference = 'Stop'

$Repo = 'ra1phdd/zev'
$Branch = 'main'
$GitHubUrl = "https://github.com/$Repo"
$RawUrl = "https://raw.githubusercontent.com/$Repo/$Branch"

function Write-Info { param([string]$Message) Write-Host "==> $Message" -ForegroundColor Cyan }
function Write-Warn { param([string]$Message) Write-Host "warning: $Message" -ForegroundColor Yellow }
function Stop-WithError { param([string]$Message) throw $Message }

if (-not $Prefix) {
    $Prefix = Join-Path $env:LOCALAPPDATA 'Programs\zev'
}

# Map the OS architecture onto the Go naming scheme used by the release archives.
# PROCESSOR_ARCHITECTURE reflects the OS, PROCESSOR_ARCHITEW6432 the real process
# when it runs under WOW64 — prefer the latter when present.
$architecture = $env:PROCESSOR_ARCHITEW6432
if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
if (-not $architecture) { $architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() }

switch ($architecture.ToUpperInvariant()) {
    'AMD64' { $arch = 'amd64' }
    'ARM64' { $arch = 'arm64' }
    'X64'   { $arch = 'amd64' }
    default { Stop-WithError "unsupported architecture: $architecture (this installer builds Windows binaries only)" }
}

# Resolve the version from the repository VERSION file, so the installer and the
# release workflow can never disagree.
if (-not $Version) {
    Write-Info "Resolving latest version from $RawUrl/VERSION"
    try {
        $response = Invoke-WebRequest -UseBasicParsing "$RawUrl/VERSION"
        # Content is a byte array on some PowerShell versions, a string on others.
        if ($response.Content -is [byte[]]) {
            $Version = [System.Text.Encoding]::UTF8.GetString($response.Content).Trim()
        }
        else {
            $Version = ([string]$response.Content).Trim()
        }
    }
    catch {
        Stop-WithError "could not read VERSION from $RawUrl/VERSION - pass -Version <version> explicitly"
    }
}

if (-not $Version) {
    Stop-WithError 'version resolved to an empty string'
}

Write-Info "Installing zev $Version ($arch)"

$archive = "zev_${Version}_windows_${arch}.zip"
$baseUrl = "$GitHubUrl/releases/download/v$Version"

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) "zev-install-$([System.Guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

try {
    $archivePath = Join-Path $tempDir $archive

    Write-Info "Downloading $archive"
    try {
        Invoke-WebRequest -UseBasicParsing "$baseUrl/$archive" -OutFile $archivePath
    }
    catch {
        Stop-WithError "download failed: $baseUrl/$archive"
    }

    # Verify the archive against the published checksums before unpacking.
    $checksumsPath = Join-Path $tempDir 'checksums.txt'
    try {
        Invoke-WebRequest -UseBasicParsing "$baseUrl/checksums.txt" -OutFile $checksumsPath

        $expected = (Get-Content $checksumsPath |
            Where-Object { $_ -match "\s$([regex]::Escape($archive))$" } |
            ForEach-Object { ($_ -split '\s+')[0] } |
            Select-Object -First 1)

        if (-not $expected) {
            Stop-WithError "no checksum published for $archive"
        }

        $actual = (Get-FileHash -Algorithm SHA256 -Path $archivePath).Hash.ToLower()
        if ($actual -ne $expected.ToLower()) {
            Stop-WithError "checksum mismatch for $archive - refusing to install"
        }

        Write-Info 'Checksum verified'
    }
    catch [System.Management.Automation.ErrorRecord] {
        # Only a checksum mismatch is fatal; an unavailable checksums.txt is not.
        if ($_.Exception.Message -match 'checksum') { throw }
        Write-Warn 'checksums.txt unavailable - skipping integrity check'
    }

    Write-Info 'Extracting'
    Expand-Archive -Path $archivePath -DestinationPath $tempDir -Force

    $binary = Join-Path $tempDir 'zev.exe'
    if (-not (Test-Path $binary)) {
        Stop-WithError "zev.exe not found inside $archive"
    }

    New-Item -ItemType Directory -Path $Prefix -Force | Out-Null
    Copy-Item -Path $binary -Destination (Join-Path $Prefix 'zev.exe') -Force

    Write-Info "Installed: $(Join-Path $Prefix 'zev.exe')"

    # Offer to put the install directory on PATH for the current user.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath -notlike "*$Prefix*") {
        Write-Warn "$Prefix is not in your PATH"
        Write-Warn "add it with: [Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path','User') + ';$Prefix', 'User')"
        Write-Warn 'then open a new terminal'
    }

    Write-Host ''
    Write-Host 'Next steps:'
    Write-Host '  zev setup                          configure your LLM provider'
    Write-Host '  zev "find large files here"        get a command suggestion'
    Write-Host ''
    Write-Warn 'On Windows, command injection into the terminal is not supported.'
    Write-Warn 'Use zev --execute, or copy the suggested command manually.'
}
finally {
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
}
