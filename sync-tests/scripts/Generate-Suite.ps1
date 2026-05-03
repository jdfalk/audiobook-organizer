# file: sync-tests/scripts/Generate-Suite.ps1
# version: 1.0.0
# guid: 5b8d2e9f-1c3a-4d6e-8f02-7a4b5c6d8e9f
#
# Build cmd/itunes-sync-tests for Windows and run it against a baseline
# iTunes Library.itl. Emits a directory of variant ITLs, each with its own
# info.json + README.md, plus a top-level index.json.

[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)]
  [string]$Baseline,

  [Parameter(Mandatory=$true)]
  [string]$OutputDir,

  [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path,

  [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $Baseline)) {
  throw "Baseline file not found: $Baseline"
}

$baselineSize = (Get-Item $Baseline).Length
if ($baselineSize -lt 1MB) {
  Write-Warning "Baseline is only $baselineSize bytes — really a full iTunes library?"
}

if (-not $SkipBuild) {
  Push-Location $RepoRoot
  try {
    Write-Host "Building cmd/itunes-sync-tests..."
    $env:CGO_ENABLED = "0"
    & go build -o "$RepoRoot\sync-tests\bin\itunes-sync-tests.exe" "./cmd/itunes-sync-tests"
    if ($LASTEXITCODE -ne 0) {
      throw "go build failed (exit $LASTEXITCODE)"
    }
  } finally {
    Pop-Location
  }
}

$exe = Join-Path $RepoRoot "sync-tests\bin\itunes-sync-tests.exe"
if (-not (Test-Path $exe)) {
  throw "Binary not found at $exe (run without -SkipBuild)"
}

if (-not (Test-Path $OutputDir)) {
  New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

Write-Host "Generating sync-diagnostic suite into $OutputDir..."
& $exe -baseline $Baseline -out $OutputDir
if ($LASTEXITCODE -ne 0) {
  throw "itunes-sync-tests exited $LASTEXITCODE"
}

$indexPath = Join-Path $OutputDir "index.json"
if (Test-Path $indexPath) {
  $count = (Get-Content $indexPath -Raw | ConvertFrom-Json).Count
  Write-Host "OK. $count variants written. Next: run scripts/Run-Tests.ps1"
} else {
  Write-Warning "index.json not found at $indexPath — generator may have changed schema."
}
