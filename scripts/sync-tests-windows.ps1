# file: scripts/sync-tests-windows.ps1
# version: 1.0.0
# guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d
#
# Driver for the iTunes / Apple Devices sync diagnostic suite on Windows.
#
# - Backs up the user's real iTunes Library.itl ONCE (refuses to overwrite an
#   existing backup) before swapping in any test variant.
# - For each NN-name/ folder under -SuiteRoot:
#     1) Closes iTunes and Apple Devices.
#     2) Copies the variant ITL into %USERPROFILE%\Music\iTunes\.
#     3) Launches iTunes via COM (iTunes.Application).
#        - Waits for the LibraryPlaylist to populate.
#        - Records track count, dialog text (best-effort).
#     4) Launches Apple Devices (Start Menu shortcut) and prompts the user.
#     5) Pops a Windows Forms dialog with [Worked] [Failed-AppleDevices]
#        [Won't-open-iTunes] [Skip] and writes result.json for that test.
#
# Apple Devices does NOT expose a programmatic API, so step 4/5 require
# manual confirmation. The PS script just orchestrates the swap-and-prompt
# loop so the user doesn't have to do it by hand.
#
# Usage (from PowerShell, run as your normal user — NOT admin):
#
#   .\sync-tests-windows.ps1 -SuiteRoot "C:\Users\you\Downloads\sync-tests"
#
# To resume after an interruption:
#
#   .\sync-tests-windows.ps1 -SuiteRoot "..." -Resume
#
# CAUTION: This script touches your real iTunes Library.itl. It will refuse
# to start until -ConfirmBackup is supplied to acknowledge that you have a
# manual backup of your iPhone and library OUTSIDE of the script's auto-
# backup. Don't skip this.

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)] [string] $SuiteRoot,
    [string] $ITunesLibraryDir = "$env:USERPROFILE\Music\iTunes",
    [string] $BackupRoot       = "$env:USERPROFILE\sync-tests-backup",
    [switch] $ConfirmBackup,
    [switch] $Resume,
    [switch] $SkipITunesProbe   # Skip the COM launch — useful if iTunes isn't installed
)

$ErrorActionPreference = "Stop"

if (-not $ConfirmBackup) {
    Write-Host "REFUSING TO START." -ForegroundColor Red
    Write-Host ""
    Write-Host "This script will overwrite '$ITunesLibraryDir\iTunes Library.itl' many times."
    Write-Host "Before running, you MUST:"
    Write-Host "  1) Have a manual backup of your iPhone."
    Write-Host "  2) Have a manual backup of '$ITunesLibraryDir' somewhere safe."
    Write-Host "  3) Re-run with -ConfirmBackup."
    Write-Host ""
    exit 1
}

# --- One-time auto-backup ---
if (-not (Test-Path $BackupRoot)) {
    New-Item -ItemType Directory -Force -Path $BackupRoot | Out-Null
}
$autoBackup = Join-Path $BackupRoot "iTunes Library.itl.original"
if (-not (Test-Path $autoBackup)) {
    $src = Join-Path $ITunesLibraryDir "iTunes Library.itl"
    if (Test-Path $src) {
        Copy-Item $src $autoBackup
        Write-Host "Backed up real library to $autoBackup" -ForegroundColor Green
    } else {
        Write-Warning "No existing iTunes Library.itl found at $src — skipping auto-backup."
    }
}

# --- Per-test loop ---
$tests = Get-ChildItem -Directory $SuiteRoot |
         Where-Object { $_.Name -match '^\d{2}-' } |
         Sort-Object Name

Add-Type -AssemblyName PresentationFramework

foreach ($t in $tests) {
    $resultPath = Join-Path $t.FullName "result.json"
    if ($Resume -and (Test-Path $resultPath)) {
        $existing = Get-Content $resultPath | ConvertFrom-Json
        if ($existing.apple_devices_sync -and $existing.apple_devices_sync -ne "did-not-test") {
            Write-Host "[skip] $($t.Name) (already has result: $($existing.apple_devices_sync))"
            continue
        }
    }

    Write-Host ""
    Write-Host "================================================================"
    Write-Host " TEST: $($t.Name)" -ForegroundColor Cyan
    Write-Host "================================================================"

    # Stop iTunes / Apple Devices if running
    Get-Process -Name iTunes              -ErrorAction SilentlyContinue | Stop-Process -Force
    Get-Process -Name "Apple Devices"     -ErrorAction SilentlyContinue | Stop-Process -Force
    Get-Process -Name "AppleMobileDevice*" -ErrorAction SilentlyContinue | Stop-Process -Force
    Start-Sleep -Seconds 2

    # Swap in the test ITL
    $variant = Join-Path $t.FullName "iTunes Library.itl"
    if (-not (Test-Path $variant)) {
        Write-Warning "  variant ITL not found at $variant — skipping"
        continue
    }
    Copy-Item -Force $variant (Join-Path $ITunesLibraryDir "iTunes Library.itl")
    Write-Host "  swapped in $variant"

    # --- Probe iTunes via COM ---
    $opensInITunes = "unknown"
    $iTunesTrackCount = -1
    if (-not $SkipITunesProbe) {
        try {
            $iTunes = New-Object -ComObject iTunes.Application
            # Block briefly to let it open the library
            Start-Sleep -Seconds 5
            $lib = $iTunes.LibraryPlaylist
            $iTunesTrackCount = $lib.Tracks.Count
            $opensInITunes = "yes"
            Write-Host "  iTunes loaded library; LibraryPlaylist.Tracks.Count = $iTunesTrackCount" -ForegroundColor Green
            $iTunes.Quit()
            [System.Runtime.InteropServices.Marshal]::ReleaseComObject($iTunes) | Out-Null
            $iTunes = $null
            [GC]::Collect(); [GC]::WaitForPendingFinalizers()
        } catch {
            $opensInITunes = "no"
            Write-Warning "  iTunes COM probe failed: $($_.Exception.Message)"
        }
        Get-Process -Name iTunes -ErrorAction SilentlyContinue | Stop-Process -Force
        Start-Sleep -Seconds 2
    }

    # --- Launch Apple Devices ---
    Write-Host "  Launching Apple Devices — manually attempt to sync your iPhone."
    try {
        Start-Process "shell:AppsFolder\AppleInc.AppleDevices_nzyj5cx40ttqa!App"
    } catch {
        Write-Warning "  Couldn't launch Apple Devices via AppsFolder URI. Open it manually."
    }

    # --- Manual prompt ---
    $choice = [System.Windows.MessageBox]::Show(
        "Test: $($t.Name)`n`nDid the Apple Devices sync of Audiobooks succeed?`n`nYes  = sync worked`nNo   = sync failed (the 'restart Apple Devices' error)`nCancel = skip this test",
        "Sync diagnostic",
        [System.Windows.MessageBoxButton]::YesNoCancel,
        [System.Windows.MessageBoxImage]::Question)

    $appleDevicesSync = switch ($choice) {
        "Yes"    { "worked" }
        "No"     { "failed" }
        "Cancel" { "skipped" }
        default  { "skipped" }
    }

    # Optional notes
    $notes = Read-Host "  Optional notes (press Enter for none)"

    $result = @{
        id                  = $t.Name
        opens_in_itunes     = $opensInITunes
        itunes_track_count  = $iTunesTrackCount
        apple_devices_sync  = $appleDevicesSync
        notes               = $notes
        timestamp           = (Get-Date).ToUniversalTime().ToString("o")
    }
    $result | ConvertTo-Json -Depth 5 | Set-Content -Path $resultPath -Encoding UTF8
    Write-Host "  → $resultPath  ($appleDevicesSync)" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "All tests done. Restoring original iTunes Library.itl..." -ForegroundColor Cyan
Get-Process -Name iTunes              -ErrorAction SilentlyContinue | Stop-Process -Force
Get-Process -Name "Apple Devices"     -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Seconds 2
Copy-Item -Force $autoBackup (Join-Path $ITunesLibraryDir "iTunes Library.itl")
Write-Host "Restored." -ForegroundColor Green
