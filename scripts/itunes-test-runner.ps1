# file: scripts/itunes-test-runner.ps1
# version: 1.0.0
# guid: c8d9e0f1-2a3b-4c5d-6e7f-8a9b0c1d2e3f
#
# iTunes Test Runner — Validates .itl files by loading them into iTunes via COM.
#
# For each subfolder in the test directory, this script:
#   1. Copies the iTunes Library.itl to a working location
#   2. Updates the iTunes registry to point to that library
#   3. Launches iTunes via COM and queries the library
#   4. Verifies tracks and file locations
#   5. Writes results.json with detailed findings
#
# Usage:
#   .\itunes-test-runner.ps1 [-BasePath "W:\audiobook-organizer\.itunes-writeback\tests\"]
#   .\itunes-test-runner.ps1 -BasePath "C:\temp\itl-tests"
#
# Requirements:
#   - iTunes for Windows installed
#   - PowerShell 5.1+ (Windows PowerShell) or PowerShell 7+
#   - Administrator access may be needed for registry modifications

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
# Helper: Set the iTunes library path via registry
# ---------------------------------------------------------------------------
function Set-ITunesLibraryPath {
    param([string]$LibraryFolder)

    $regPath = "HKCU:\Software\Apple Computer, Inc.\iTunes"

    # Ensure the registry key exists
    if (-not (Test-Path $regPath)) {
        New-Item -Path $regPath -Force | Out-Null
    }

    # ITunesRecentDatabasePaths is a multi-string value listing library folders.
    # iTunes picks the first entry. We prepend our test folder.
    try {
        $existing = (Get-ItemProperty -Path $regPath -Name "ITunesRecentDatabasePaths" -ErrorAction SilentlyContinue).ITunesRecentDatabasePaths
    }
    catch {
        $existing = @()
    }

    if (-not $existing) { $existing = @() }

    # Put our path first; keep others as fallback
    $newPaths = @($LibraryFolder) + ($existing | Where-Object { $_ -ne $LibraryFolder })
    Set-ItemProperty -Path $regPath -Name "ITunesRecentDatabasePaths" -Value $newPaths -Type MultiString
    Write-Host "  Registry updated: ITunesRecentDatabasePaths[0] = $LibraryFolder"
}

# ---------------------------------------------------------------------------
# Helper: Restore original iTunes library path
# ---------------------------------------------------------------------------
function Restore-ITunesLibraryPath {
    param([string[]]$OriginalPaths)

    $regPath = "HKCU:\Software\Apple Computer, Inc.\iTunes"
    if ($OriginalPaths -and $OriginalPaths.Count -gt 0) {
        Set-ItemProperty -Path $regPath -Name "ITunesRecentDatabasePaths" -Value $OriginalPaths -Type MultiString
        Write-Host "  Restored original ITunesRecentDatabasePaths"
    }
}

# ---------------------------------------------------------------------------
# Helper: Get the original registry paths (for restore later)
# ---------------------------------------------------------------------------
function Get-OriginalITunesPaths {
    $regPath = "HKCU:\Software\Apple Computer, Inc.\iTunes"
    try {
        $val = (Get-ItemProperty -Path $regPath -Name "ITunesRecentDatabasePaths" -ErrorAction SilentlyContinue).ITunesRecentDatabasePaths
        return $val
    }
    catch {
        return @()
    }
}

# ---------------------------------------------------------------------------
# Main: Run a single test case
# ---------------------------------------------------------------------------
function Invoke-TestCase {
    param(
        [string]$TestFolder,
        [string]$TestName
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

    # Check that the .itl file exists
    $itlFile = Join-Path $TestFolder "iTunes Library.itl"
    if (-not (Test-Path $itlFile)) {
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

    # Point iTunes at this test folder
    try {
        Set-ITunesLibraryPath -LibraryFolder $TestFolder
    }
    catch {
        $errors += "Failed to set registry: $_"
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
                    # IITFileOrCDTrack has a Location property
                    $location = $null
                    try {
                        # Only file tracks have Location; URL tracks do not
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
                # This test intentionally has missing files; that's OK
                Write-Host "  (Test expects missing files — $invalidFiles missing is acceptable)"
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

# Save original registry paths for restore
$originalPaths = Get-OriginalITunesPaths
Write-Host "Saved original iTunes library paths for restore"

# Find test subfolders (sorted by name for deterministic order)
$testFolders = Get-ChildItem -Path $BasePath -Directory | Sort-Object Name
if ($testFolders.Count -eq 0) {
    Write-Warning "No test subfolders found in $BasePath"
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

    $result = Invoke-TestCase -TestFolder $tf.FullName -TestName $testName

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

# Restore original iTunes library paths
if (-not $SkipCleanup) {
    Write-Host "Restoring original iTunes configuration..."
    Restore-ITunesLibraryPath -OriginalPaths $originalPaths
}

# Write summary
$summaryPath = Join-Path $BasePath "test-summary.json"
$summary = @{
    timestamp    = Get-ISO8601
    total_tests  = $allResults.Count
    passed       = $passCount
    failed       = $failCount
    results      = $allResults
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
