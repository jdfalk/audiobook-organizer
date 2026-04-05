# file: scripts/itunes-test-runner.ps1
# version: 1.1.0
# guid: c8d9e0f1-2a3b-4c5d-6e7f-8a9b0c1d2e3f
#
# iTunes Test Runner -- Validates .itl files by loading them into iTunes via COM.
#
# Strategy: iTunes always opens whatever "iTunes Library.itl" is in its library
# folder. We find that folder, back up the real ITL, swap in each test ITL,
# launch iTunes via COM, verify tracks, then restore the original.
#
# Usage:
#   .\itunes-test-runner.ps1
#   .\itunes-test-runner.ps1 -BasePath "C:\temp\itl-tests"
#
# Requirements:
#   - iTunes for Windows installed
#   - PowerShell 5.1+ (Windows PowerShell) or PowerShell 7+

[CmdletBinding()]
param(
    [Parameter(Mandatory = $false)]
    [string]$BasePath = "W:\audiobook-organizer\.itunes-writeback\tests\",

    [Parameter(Mandatory = $false)]
    [switch]$SkipCleanup,

    [Parameter(Mandatory = $false)]
    [int]$ITunesStartupWaitSeconds = 15,

    [Parameter(Mandatory = $false)]
    [int]$ITunesShutdownWaitSeconds = 10
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ---------------------------------------------------------------------------
# Helper: Get current timestamp in ISO 8601 format
# ---------------------------------------------------------------------------
function Get-ISO8601 {
    return (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
}

# ---------------------------------------------------------------------------
# Helper: Kill any running iTunes process
# ---------------------------------------------------------------------------
function Stop-ITunesProcess {
    $procs = Get-Process -Name "iTunes" -ErrorAction SilentlyContinue
    if ($procs) {
        Write-Host "  Killing existing iTunes processes..."
        $procs | Stop-Process -Force
        Start-Sleep -Seconds 3
    }
}

# ---------------------------------------------------------------------------
# Helper: Find iTunes library folder from registry or default location
# ---------------------------------------------------------------------------
function Get-ITunesLibraryFolder {
    # Try registry first -- ITunesRecentDatabasePaths[0] is the last-used folder
    $regPath = "HKCU:\Software\Apple Computer, Inc.\iTunes"
    try {
        $paths = (Get-ItemProperty -Path $regPath -Name "ITunesRecentDatabasePaths" -ErrorAction SilentlyContinue).ITunesRecentDatabasePaths
        if ($paths -and $paths.Count -gt 0 -and (Test-Path $paths[0])) {
            return $paths[0]
        }
    }
    catch {}

    # Fallback: default iTunes library location
    $default = Join-Path $env:APPDATA "Apple Computer\iTunes"
    if (Test-Path $default) {
        return $default
    }

    # Second fallback: Music folder
    $music = Join-Path ([Environment]::GetFolderPath("MyMusic")) "iTunes"
    if (Test-Path $music) {
        return $music
    }

    return $null
}

# ---------------------------------------------------------------------------
# Main: Run a single test case
# ---------------------------------------------------------------------------
function Invoke-TestCase {
    param(
        [string]$TestFolder,
        [string]$TestName,
        [string]$ITunesLibraryFolder,
        [string]$BackupITLPath
    )

    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    $errors = @()
    $result = @{
        test_name                = $TestName
        success                  = $false
        track_count              = 0
        tracks_verified          = 0
        tracks_with_valid_files  = 0
        tracks_with_invalid_files = 0
        invalid_file_paths       = @()
        errors                   = @()
        itunes_version           = ""
        test_duration_ms         = 0
        timestamp                = Get-ISO8601
    }

    # Check that the test .itl file exists
    $testITL = Join-Path $TestFolder "iTunes Library.itl"
    if (-not (Test-Path $testITL)) {
        $errors += "iTunes Library.itl not found in $TestFolder"
        $result.errors = $errors
        $result.test_duration_ms = $stopwatch.ElapsedMilliseconds
        return $result
    }

    # Read test-info.json if present
    $testInfoPath = Join-Path $TestFolder "test-info.json"
    $testInfo = $null
    if (Test-Path $testInfoPath) {
        try {
            $testInfo = Get-Content $testInfoPath -Raw | ConvertFrom-Json
            Write-Host "  Test info: $($testInfo.description)"
        }
        catch {
            Write-Host "  Warning: Could not parse test-info.json"
        }
    }

    # Kill any existing iTunes
    Stop-ITunesProcess

    # Swap: copy the test ITL into iTunes' library folder
    $realITL = Join-Path $ITunesLibraryFolder "iTunes Library.itl"
    try {
        Write-Host "  Swapping test ITL into: $realITL"
        Copy-Item -Path $testITL -Destination $realITL -Force
    }
    catch {
        $errors += "Failed to swap ITL file: $_"
        $result.errors = $errors
        $result.test_duration_ms = $stopwatch.ElapsedMilliseconds
        return $result
    }

    # Launch iTunes via COM
    $iTunes = $null
    try {
        Write-Host "  Launching iTunes..."
        $iTunes = New-Object -ComObject iTunes.Application

        # Wait for iTunes to finish loading the library
        Write-Host "  Waiting ${ITunesStartupWaitSeconds}s for library load..."
        Start-Sleep -Seconds $ITunesStartupWaitSeconds

        # Get iTunes version
        try {
            $result.itunes_version = $iTunes.Version
            Write-Host "  iTunes version: $($result.itunes_version)"
        }
        catch {
            $result.itunes_version = "unknown"
            $errors += "Could not get iTunes version: $_"
        }

        # Get the main library playlist (contains all tracks)
        $library = $null
        try {
            $library = $iTunes.LibraryPlaylist
        }
        catch {
            $errors += "Could not access LibraryPlaylist: $_"
        }

        if ($library) {
            $tracks = $library.Tracks
            $trackCount = $tracks.Count
            $result.track_count = $trackCount
            Write-Host "  Track count: $trackCount"

            $verified = 0
            $validFiles = 0
            $invalidFiles = 0
            $invalidPaths = @()

            # Iterate through tracks
            for ($i = 1; $i -le $trackCount; $i++) {
                try {
                    $track = $tracks.Item($i)
                    $verified++

                    # Check if track has a file location
                    $location = $null
                    try {
                        $location = $track.Location
                    }
                    catch {
                        # Track may not be a file track (e.g., streaming)
                    }

                    if ($location) {
                        if (Test-Path $location) {
                            $validFiles++
                        }
                        else {
                            $invalidFiles++
                            $invalidPaths += $location
                        }
                    }

                    # Log first few tracks for debugging
                    if ($i -le 5) {
                        $name = ""
                        try { $name = $track.Name } catch {}
                        $artist = ""
                        try { $artist = $track.Artist } catch {}
                        Write-Host "    Track $i`: $name - $artist $(if ($location) { "[$location]" } else { '[no location]' })"
                    }
                    elseif ($i -eq 6 -and $trackCount -gt 5) {
                        Write-Host "    ... ($($trackCount - 5) more tracks)"
                    }
                }
                catch {
                    $errors += "Error reading track $i`: $_"
                }
            }

            $result.tracks_verified = $verified
            $result.tracks_with_valid_files = $validFiles
            $result.tracks_with_invalid_files = $invalidFiles
            $result.invalid_file_paths = $invalidPaths

            # Determine success based on test expectations
            if ($testInfo -and $testInfo.PSObject.Properties["expected_track_count"]) {
                $expected = $testInfo.expected_track_count
                if ($trackCount -ne $expected) {
                    $errors += "Expected $expected tracks, got $trackCount"
                }
            }

            if ($testInfo -and $testInfo.PSObject.Properties["expect_missing_files"] -and $testInfo.expect_missing_files) {
                Write-Host "  (Test expects missing files -- $invalidFiles missing is acceptable)"
            }
            elseif ($invalidFiles -gt 0 -and (-not $testInfo -or -not $testInfo.PSObject.Properties["allow_missing_files"])) {
                $errors += "$invalidFiles tracks have invalid file paths"
            }
        }
        else {
            $errors += "LibraryPlaylist was null"
        }
    }
    catch {
        $errors += "iTunes COM error: $_"
    }
    finally {
        # Close iTunes
        if ($iTunes) {
            try {
                Write-Host "  Closing iTunes..."
                $iTunes.Quit()
                [System.Runtime.Interopservices.Marshal]::ReleaseComObject($iTunes) | Out-Null
            }
            catch {
                Write-Host "  Warning: Could not quit iTunes gracefully: $_"
            }
        }

        # Wait for iTunes to shut down
        Start-Sleep -Seconds $ITunesShutdownWaitSeconds
        Stop-ITunesProcess
    }

    $result.errors = $errors
    $result.success = ($errors.Count -eq 0)
    $result.test_duration_ms = $stopwatch.ElapsedMilliseconds
    return $result
}

# ===========================================================================
# Main script entry point
# ===========================================================================

Write-Host "=========================================="
Write-Host "iTunes ITL Test Runner"
Write-Host "=========================================="
Write-Host "Base path: $BasePath"
Write-Host "Time: $(Get-ISO8601)"
Write-Host ""

# Validate base path
if (-not (Test-Path $BasePath)) {
    Write-Error "Base path does not exist: $BasePath"
    exit 1
}

# Find iTunes library folder
$iTunesFolder = Get-ITunesLibraryFolder
if (-not $iTunesFolder) {
    Write-Error "Could not find iTunes library folder. Is iTunes installed?"
    exit 1
}
Write-Host "iTunes library folder: $iTunesFolder"

$realITL = Join-Path $iTunesFolder "iTunes Library.itl"
if (-not (Test-Path $realITL)) {
    Write-Error "No iTunes Library.itl found at: $realITL"
    exit 1
}

# Kill iTunes before we touch anything
Stop-ITunesProcess

# Back up the real ITL
$backupITL = Join-Path $iTunesFolder "iTunes Library.itl.test-backup"
Write-Host "Backing up real ITL to: $backupITL"
Copy-Item -Path $realITL -Destination $backupITL -Force
Write-Host ""

# Find test subfolders (sorted by name for deterministic order)
$testFolders = Get-ChildItem -Path $BasePath -Directory | Sort-Object Name
if ($testFolders.Count -eq 0) {
    Write-Warning "No test subfolders found in $BasePath"
    # Restore backup before exiting
    Copy-Item -Path $backupITL -Destination $realITL -Force
    Remove-Item $backupITL -Force
    exit 0
}

Write-Host "Found $($testFolders.Count) test case(s):"
foreach ($tf in $testFolders) {
    Write-Host "  - $($tf.Name)"
}
Write-Host ""

# Run each test case
$allResults = @()
$passCount = 0
$failCount = 0

foreach ($tf in $testFolders) {
    $testName = $tf.Name
    Write-Host "------------------------------------------"
    Write-Host "Test: $testName"
    Write-Host "------------------------------------------"

    $result = Invoke-TestCase -TestFolder $tf.FullName -TestName $testName `
        -ITunesLibraryFolder $iTunesFolder -BackupITLPath $backupITL

    # Write results.json to the test subfolder
    $resultsPath = Join-Path $tf.FullName "results.json"
    $result | ConvertTo-Json -Depth 10 | Set-Content -Path $resultsPath -Encoding UTF8
    Write-Host "  Results written to: $resultsPath"

    if ($result.success) {
        Write-Host "  PASS" -ForegroundColor Green
        $passCount++
    }
    else {
        Write-Host "  FAIL" -ForegroundColor Red
        foreach ($err in $result.errors) {
            Write-Host "    Error: $err" -ForegroundColor Yellow
        }
        $failCount++
    }

    $allResults += $result
    Write-Host ""
}

# Restore original ITL
if (-not $SkipCleanup) {
    Write-Host "Restoring original iTunes library..."
    Stop-ITunesProcess
    Copy-Item -Path $backupITL -Destination $realITL -Force
    Remove-Item $backupITL -Force
    Write-Host "  Original ITL restored."
}
else {
    Write-Host "Skipping cleanup -- backup remains at: $backupITL"
}

# Write summary
$summaryPath = Join-Path $BasePath "test-summary.json"
$summary = @{
    timestamp          = Get-ISO8601
    total_tests        = $allResults.Count
    passed             = $passCount
    failed             = $failCount
    itunes_library_dir = $iTunesFolder
    results            = $allResults
}
$summary | ConvertTo-Json -Depth 10 | Set-Content -Path $summaryPath -Encoding UTF8

Write-Host "=========================================="
Write-Host "Summary: $passCount passed, $failCount failed out of $($allResults.Count) tests"
Write-Host "Full results: $summaryPath"
Write-Host "=========================================="

if ($failCount -gt 0) {
    exit 1
}
exit 0
