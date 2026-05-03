# file: sync-tests/scripts/Restore-Baseline.ps1
# version: 1.0.0
# guid: 4d7a8b1e-9f0c-4e5d-8a13-2b6c7d8e9f01
#
# One-click safety net: copy the preflight backup back over the live
# iTunes library. Use this if Run-Tests.ps1 was interrupted partway and
# left a variant ITL in place.

[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)]
  [string]$ITunesLibPath,

  [string]$BackupPath = "$ITunesLibPath.preflight-bak"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $BackupPath)) {
  throw "Backup not found at $BackupPath — cannot restore."
}

Get-Process -Name iTunes,AppleDevices -ErrorAction SilentlyContinue |
  ForEach-Object { try { $_ | Stop-Process -Force -ErrorAction SilentlyContinue } catch {} }
Start-Sleep -Seconds 1

Copy-Item -LiteralPath $BackupPath -Destination $ITunesLibPath -Force
$size = (Get-Item $ITunesLibPath).Length
Write-Host "Restored $ITunesLibPath from backup ($size bytes)."
