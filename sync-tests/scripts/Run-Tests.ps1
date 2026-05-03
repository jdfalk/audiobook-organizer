# file: sync-tests/scripts/Run-Tests.ps1
# version: 1.0.0
# guid: 9c4e1a78-2b6d-4f3e-8c92-1d5a7b8c9e0f
#
# Interactive Windows test runner for the iTunes / Apple Devices sync
# diagnostic suite. For each variant ITL produced by Generate-Suite.ps1:
#   1. Restore the baseline (so each test starts clean).
#   2. Show hypothesis + description.
#   3. Copy the variant ITL into the iTunes library location.
#   4. Open iTunes; capture user verdict.
#   5. Open Apple Devices; capture user verdict.
#   6. Append a result row to results.json (immediately fsynced).
#
# Resumes on re-run by default — variants already in results.json are skipped.
# Pass -Resume:$false to force re-test.

[CmdletBinding()]
param(
  # Directory produced by Generate-Suite.ps1 — must contain index.json
  # and one subdir per variant.
  [Parameter(Mandatory=$true)]
  [string]$SuiteDir,

  # Path to the live iTunes Library.itl that iTunes/Apple Devices will read.
  # The runner overwrites this file with the variant under test. Make a
  # backup first (-BackupPath defaults to "<ITunesLibPath>.preflight-bak").
  [Parameter(Mandatory=$true)]
  [string]$ITunesLibPath,

  # Path to the JSON file the runner appends results to. Created if missing.
  [Parameter(Mandatory=$true)]
  [string]$ResultsPath,

  # Path to iTunes.exe.
  [string]$ITunesExe = "C:\Program Files\iTunes\iTunes.exe",

  # Path to AppleDevices.exe (UWP install path varies; pass explicitly if needed).
  [string]$AppleDevicesExe = "",

  # Where to save the "preflight" backup. Default: alongside the live library.
  [string]$BackupPath = "$ITunesLibPath.preflight-bak",

  # If true (default), restore the baseline backup over $ITunesLibPath at the
  # END of each variant so the next test starts from a known state.
  [bool]$RestoreAfterEach = $true,

  # If true (default), skip variants already in $ResultsPath.
  [bool]$Resume = $true,

  # Skip the variants whose IDs match this regex.
  [string]$Skip = "",

  # Only run variants whose IDs match this regex (applied AFTER -Skip).
  [string]$Only = ""
)

$ErrorActionPreference = "Stop"

# --- Pre-flight ---------------------------------------------------------

if (-not (Test-Path $SuiteDir)) { throw "SuiteDir not found: $SuiteDir" }
$indexPath = Join-Path $SuiteDir "index.json"
if (-not (Test-Path $indexPath)) { throw "index.json not found at $indexPath — did Generate-Suite.ps1 run?" }

if (-not (Test-Path $ITunesLibPath)) {
  throw "ITunesLibPath does not exist: $ITunesLibPath"
}

if (-not (Test-Path $BackupPath)) {
  Write-Host "No backup found at $BackupPath — creating one from current library."
  Copy-Item -LiteralPath $ITunesLibPath -Destination $BackupPath -Force
  Write-Host "Backup created. THIS BACKUP IS THE SAFETY NET."
} else {
  $bSize = (Get-Item $BackupPath).Length
  $lSize = (Get-Item $ITunesLibPath).Length
  Write-Host "Using existing backup at $BackupPath ($bSize bytes; current library is $lSize bytes)."
}

if (-not (Test-Path $ITunesExe)) {
  Write-Warning "iTunes.exe not found at $ITunesExe — pass -ITunesExe explicitly if iTunes is installed."
}

# --- Load index + prior results -----------------------------------------

$index = Get-Content $indexPath -Raw | ConvertFrom-Json

$results = @()
if (Test-Path $ResultsPath) {
  try {
    $results = @(Get-Content $ResultsPath -Raw | ConvertFrom-Json)
  } catch {
    Write-Warning "Failed to parse $ResultsPath; treating as empty. ($_)"
    $results = @()
  }
}
$completed = @{}
foreach ($r in $results) { $completed[$r.variant_id] = $true }

function Save-Results {
  param($Rows, $Path)
  $tmp = "$Path.tmp"
  $json = $Rows | ConvertTo-Json -Depth 6
  [System.IO.File]::WriteAllText($tmp, $json, (New-Object System.Text.UTF8Encoding($false)))
  Move-Item -LiteralPath $tmp -Destination $Path -Force
}

function Prompt-Choice {
  param([string]$Question, [string[]]$Choices)
  while ($true) {
    Write-Host ""
    Write-Host $Question -ForegroundColor Cyan
    for ($i = 0; $i -lt $Choices.Length; $i++) {
      Write-Host ("  [{0}] {1}" -f ($i + 1), $Choices[$i])
    }
    $r = Read-Host "Enter 1-$($Choices.Length)"
    $n = 0
    if ([int]::TryParse($r, [ref]$n) -and $n -ge 1 -and $n -le $Choices.Length) {
      return $Choices[$n - 1]
    }
    Write-Host "Invalid input." -ForegroundColor Yellow
  }
}

function Stop-AppByName {
  param([string]$Name)
  Get-Process -Name $Name -ErrorAction SilentlyContinue | ForEach-Object {
    try { $_ | Stop-Process -Force -ErrorAction SilentlyContinue } catch {}
  }
  Start-Sleep -Seconds 1
}

# --- Main loop ----------------------------------------------------------

$ran = 0
foreach ($entry in $index) {
  $vid = $entry.id
  if ($Skip -ne "" -and $vid -match $Skip) { continue }
  if ($Only -ne "" -and $vid -notmatch $Only) { continue }
  if ($Resume -and $completed.ContainsKey($vid)) {
    Write-Host "[skip] $vid (already in results)" -ForegroundColor DarkGray
    continue
  }

  $variantDir = Join-Path $SuiteDir $vid
  $variantITL = Join-Path $variantDir "iTunes Library.itl"
  $infoPath   = Join-Path $variantDir "info.json"

  if (-not (Test-Path $variantITL)) {
    Write-Warning "Variant ITL missing: $variantITL — skipping."
    continue
  }

  $info = $null
  if (Test-Path $infoPath) {
    try { $info = Get-Content $infoPath -Raw | ConvertFrom-Json } catch {}
  }

  Write-Host ""
  Write-Host ("=" * 72)
  Write-Host "VARIANT: $vid" -ForegroundColor Green
  Write-Host ("=" * 72)
  if ($info) {
    Write-Host "Hypothesis : $($info.hypothesis)"
    Write-Host "Description: $($info.description)"
    if ($info.mutations) {
      Write-Host "Mutations  : $($info.mutations -join ', ')"
    }
  }

  # Make sure neither app is holding the file.
  Stop-AppByName "iTunes"
  Stop-AppByName "AppleDevices"

  # Restore baseline before applying variant — guarantees clean slate.
  Copy-Item -LiteralPath $BackupPath -Destination $ITunesLibPath -Force
  # Apply variant.
  Copy-Item -LiteralPath $variantITL -Destination $ITunesLibPath -Force

  $startedAt = (Get-Date).ToString("o")

  # --- iTunes step ---
  if (Test-Path $ITunesExe) {
    Write-Host "Launching iTunes..."
    Start-Process -FilePath $ITunesExe | Out-Null
  } else {
    Write-Host "Skipping iTunes launch (exe not found). Open it manually if you want."
  }
  $itunesVerdict = Prompt-Choice "Did iTunes open the library cleanly?" @("ok","wont-open","crash","skip")

  # --- Apple Devices step ---
  if ($itunesVerdict -ne "wont-open" -and $itunesVerdict -ne "crash") {
    if ($AppleDevicesExe -and (Test-Path $AppleDevicesExe)) {
      Write-Host "Launching Apple Devices..."
      Start-Process -FilePath $AppleDevicesExe | Out-Null
    } else {
      Write-Host "Open Apple Devices manually, plug in your iPhone, and start a sync."
    }
    $adVerdict = Prompt-Choice "What happened in Apple Devices during sync?" @(
      "sync-ok",
      "sync-fail-step-3",
      "sync-fail-other",
      "wont-open",
      "crash",
      "skip"
    )
  } else {
    $adVerdict = "skipped-due-to-itunes"
  }

  $notes = Read-Host "Free-text notes (optional, hit Enter to skip)"

  $row = [ordered]@{
    variant_id    = $vid
    hypothesis    = if ($info) { $info.hypothesis } else { $entry.hypothesis }
    description   = if ($info) { $info.description } else { "" }
    mutations     = if ($info) { $info.mutations } else { @() }
    started_at    = $startedAt
    finished_at   = (Get-Date).ToString("o")
    itunes        = $itunesVerdict
    apple_devices = $adVerdict
    notes         = $notes
  }
  $results = @($results) + ,$row
  Save-Results -Rows $results -Path $ResultsPath
  Write-Host "[saved] $vid -> $($row.itunes) / $($row.apple_devices)" -ForegroundColor Green

  Stop-AppByName "iTunes"
  Stop-AppByName "AppleDevices"

  if ($RestoreAfterEach) {
    Copy-Item -LiteralPath $BackupPath -Destination $ITunesLibPath -Force
  }

  $ran++
}

Write-Host ""
Write-Host "Done. Ran $ran variant(s). Total results in $($ResultsPath): $($results.Count)."
