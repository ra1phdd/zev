<#
.SYNOPSIS
    Zev uninstaller for Windows — removes the zev.exe binary.

.DESCRIPTION
    Removes the zev binary from the installation directory.

    Run it from PowerShell:
        irm https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/uninstall.ps1 | iex

    Or download the file first and run it:
        .\uninstall.ps1

.PARAMETER Prefix
    Installation directory. Defaults to $env:LOCALAPPDATA\Programs\zev,
    matching the install.ps1 default.

.EXAMPLE
    irm https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/uninstall.ps1 | iex
#>

[CmdletBinding()]
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '')]
param(
    [string]$Prefix = $env:ZEV_PREFIX
)

$ErrorActionPreference = 'Stop'

function Write-Info { param([string]$Message) Write-Host "==> $Message" -ForegroundColor Cyan }
function Write-Warn { param([string]$Message) Write-Host "warning: $Message" -ForegroundColor Yellow }
function Stop-WithError { param([string]$Message) throw $Message }

if (-not $Prefix) {
    $Prefix = Join-Path $env:LOCALAPPDATA 'Programs\zev'
}

$binary = Join-Path $Prefix 'zev.exe'

if (-not (Test-Path -LiteralPath $binary)) {
    Write-Warn "$binary not found — nothing to remove"
    exit 0
}

Write-Info "Removing $binary"
Remove-Item -LiteralPath $binary -Force

Write-Info "Uninstalled"

# Optionally remove the installation directory if it's now empty.
if ((Test-Path -LiteralPath $Prefix -PathType Container) -and
    @(Get-ChildItem -LiteralPath $Prefix -ErrorAction SilentlyContinue).Count -eq 0) {
    Write-Info "Installation directory is empty, removing $Prefix"
    try {
        Remove-Item -LiteralPath $Prefix -Force
    }
    catch {
        # The directory may still be in use; leaving it behind is harmless.
        Write-Verbose "could not remove ${Prefix}: $($_.Exception.Message)"
    }
}
