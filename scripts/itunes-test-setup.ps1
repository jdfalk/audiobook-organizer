# file: scripts/itunes-test-setup.ps1
# version: 1.0.0
# guid: d9e0f1a2-3b4c-5d6e-7f8a-9b0c1d2e3f4a
#
# iTunes Test Setup — Prepares the test directory structure for ITL testing.
#
# This script:
#   1. Creates the test directory at the specified base path
#   2. For each test case subfolder, ensures the proper directory structure
#   3. Sets up the iTunes Media folder path in each subfolder
#   4. Creates placeholder files if needed (for tests that need real files)
#
# Usage:
#   .\itunes-test-setup.ps1 [-BasePath "W:\audiobook-organizer\.itunes-writeback\tests\"]
#   .\itunes-test-setup.ps1 -BasePath "C:\temp\itl-tests" -CreatePlaceholderFiles
#
# The .itl files themselves are generated on the Linux server by the Go
# GenerateTestITLSuite function. This script only sets up the directory
# structure and auxiliary files that iTunes expects.

[CmdletBinding()]
param(
    [Parameter(Mandatory = $false)]
    [string]$BasePath = "W:\audiobook-organizer\.itunes-writeback\tests\",

    [Parameter(Mandatory = $false)]
    [switch]$CreatePlaceholderFiles,

    [Parameter(Mandatory = $false)]
    [string]$AudiobookRoot = "W:\audiobook-organizer"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ---------------------------------------------------------------------------
# Helper: Create a minimal M4B placeholder file (just enough for iTunes)
# ---------------------------------------------------------------------------
function New-PlaceholderM4B {
    param([string]$FilePath)

    $dir = Split-Path $FilePath -Parent
    if (-not (Test-Path $dir)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }

    if (-not (Test-Path $FilePath)) {
        # Create a minimal valid ftyp + moov container.
        # This is not a real M4B but enough that iTunes won't crash on it.
        # A real test should use actual audio files.
        $ftyp = [byte[]]@(
            0x00, 0x00, 0x00, 0x18,  # size = 24
            0x66, 0x74, 0x79, 0x70,  # 'ftyp'
            0x4D, 0x34, 0x42, 0x20,  # 'M4B '
            0x00, 0x00, 0x00, 0x00,  # minor version
            0x4D, 0x34, 0x42, 0x20,  # compatible brand 'M4B '
            0x69, 0x73, 0x6F, 0x6D   # compatible brand 'isom'
        )
        $moov = [byte[]]@(
            0x00, 0x00, 0x00, 0x08,  # size = 8 (empty moov)
            0x6D, 0x6F, 0x6F, 0x76   # 'moov'
        )
        $content = $ftyp + $moov
        [System.IO.File]::WriteAllBytes($FilePath, $content)
        Write-Host "    Created placeholder: $FilePath"
    }
}

# ---------------------------------------------------------------------------
# Helper: Set up a test case subfolder
# ---------------------------------------------------------------------------
function Initialize-TestFolder {
    param(
        [string]$FolderPath,
        [string]$TestName
    )

    # Create the folder if it doesn't exist
    if (-not (Test-Path $FolderPath)) {
        New-Item -ItemType Directory -Path $FolderPath -Force | Out-Null
    }

    # Create the "iTunes Media" subfolder (iTunes expects this)
    $mediaFolder = Join-Path $FolderPath "iTunes Media"
    if (-not (Test-Path $mediaFolder)) {
        New-Item -ItemType Directory -Path $mediaFolder -Force | Out-Null
    }

    # Create "iTunes Media\Music" subfolder
    $musicFolder = Join-Path $mediaFolder "Music"
    if (-not (Test-Path $musicFolder)) {
        New-Item -ItemType Directory -Path $musicFolder -Force | Out-Null
    }

    # Create "iTunes Media\Audiobooks" subfolder
    $audiobooksFolder = Join-Path $mediaFolder "Audiobooks"
    if (-not (Test-Path $audiobooksFolder)) {
        New-Item -ItemType Directory -Path $audiobooksFolder -Force | Out-Null
    }

    # Create an empty iTunes Library Genius.itdb (iTunes looks for this)
    $geniusPath = Join-Path $FolderPath "iTunes Library Genius.itdb"
    if (-not (Test-Path $geniusPath)) {
        [System.IO.File]::WriteAllBytes($geniusPath, [byte[]]@())
    }

    # Create an empty iTunes Library Extras.itdb
    $extrasPath = Join-Path $FolderPath "iTunes Library Extras.itdb"
    if (-not (Test-Path $extrasPath)) {
        [System.IO.File]::WriteAllBytes($extrasPath, [byte[]]@())
    }

    Write-Host "  Initialized: $TestName"
}

# ---------------------------------------------------------------------------
# Helper: Create placeholder audio files for tests that reference them
# ---------------------------------------------------------------------------
function New-TestAudioFiles {
    param(
        [string]$TestFolder,
        [string]$AudiobookRoot
    )

    # Read test-info.json to find expected file paths
    $testInfoPath = Join-Path $TestFolder "test-info.json"
    if (-not (Test-Path $testInfoPath)) {
        Write-Host "    No test-info.json — skipping placeholder creation"
        return
    }

    try {
        $testInfo = Get-Content $testInfoPath -Raw | ConvertFrom-Json
    }
    catch {
        Write-Host "    Warning: Could not parse test-info.json"
        return
    }

    # If test-info.json has a "tracks" array with "location" fields, create placeholders
    if ($testInfo.PSObject.Properties["tracks"]) {
        foreach ($track in $testInfo.tracks) {
            if ($track.PSObject.Properties["location"] -and $track.location) {
                $loc = $track.location
                # Only create placeholders for paths under our audiobook root
                if ($loc.StartsWith($AudiobookRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
                    New-PlaceholderM4B -FilePath $loc
                }
            }
        }
    }
}

# ===========================================================================
# Main script entry point
# ===========================================================================

Write-Host "=========================================="
Write-Host "iTunes Test Setup"
Write-Host "=========================================="
Write-Host "Base path: $BasePath"
Write-Host "Audiobook root: $AudiobookRoot"
Write-Host "Create placeholders: $CreatePlaceholderFiles"
Write-Host ""

# Create the base directory
if (-not (Test-Path $BasePath)) {
    New-Item -ItemType Directory -Path $BasePath -Force | Out-Null
    Write-Host "Created base directory: $BasePath"
}

# Define the test cases — these correspond to what GenerateTestITLSuite creates
$testCases = @(
    "01-blank",
    "02-single-track",
    "03-ten-tracks",
    "04-hundred-tracks",
    "05-full-library",
    "06-updated-locations",
    "07-mixed-sources",
    "08-unicode-paths",
    "09-missing-files",
    "10-duplicate-pids"
)

Write-Host "Setting up $($testCases.Count) test case folders..."
Write-Host ""

foreach ($tc in $testCases) {
    $folderPath = Join-Path $BasePath $tc
    Initialize-TestFolder -FolderPath $folderPath -TestName $tc

    # Create placeholder audio files if requested
    if ($CreatePlaceholderFiles) {
        New-TestAudioFiles -TestFolder $folderPath -AudiobookRoot $AudiobookRoot
    }
}

Write-Host ""
Write-Host "=========================================="
Write-Host "Setup complete."
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. On the Linux server, generate ITL files:"
Write-Host "     curl -sk -X POST 'https://server:8484/api/v1/maintenance/generate-itl-tests'"
Write-Host ""
Write-Host "  2. Verify .itl files were created:"
Write-Host "     dir '$BasePath\*\iTunes Library.itl'"
Write-Host ""
Write-Host "  3. Run the test suite:"
Write-Host "     .\itunes-test-runner.ps1 -BasePath '$BasePath'"
Write-Host "=========================================="
