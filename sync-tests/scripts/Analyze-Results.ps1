<#
.SYNOPSIS
  Group Apple Devices sync test results by mutation tag and surface
  patterns that correlate with sync-fail-step-3.

.DESCRIPTION
  Reads results.json (produced by Run-Tests.ps1) plus index.json
  (produced by Generate-Suite.ps1) and prints:

    1. A per-variant table: id | iTunes | AppleDevices | mutations
    2. A per-mutation-tag breakdown: counts of each AppleDevices outcome
    3. A "smoking gun" section: mutation tags whose presence flips the
       outcome from sync-ok -> sync-fail-step-3 across pairs.

  No external deps. Pure PowerShell.

.PARAMETER ResultsPath
  Path to results.json (verdicts + timestamps written by Run-Tests.ps1).

.PARAMETER SuiteDir
  Path to the generated suite directory (must contain index.json).

.PARAMETER OutputJson
  Optional. If set, writes the structured analysis as JSON to this path
  in addition to the console report.

.EXAMPLE
  .\Analyze-Results.ps1 `
    -ResultsPath C:\sync-tests-out\results.json `
    -SuiteDir    C:\sync-tests-out
#>

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string] $ResultsPath,

    [Parameter(Mandatory = $true)]
    [string] $SuiteDir,

    [string] $OutputJson
)

$ErrorActionPreference = 'Stop'

if (-not (Test-Path -LiteralPath $ResultsPath)) {
    throw "Results file not found: $ResultsPath"
}
$indexPath = Join-Path $SuiteDir 'index.json'
if (-not (Test-Path -LiteralPath $indexPath)) {
    throw "Suite index not found: $indexPath"
}

$results = Get-Content -LiteralPath $ResultsPath -Raw | ConvertFrom-Json
$index   = Get-Content -LiteralPath $indexPath   -Raw | ConvertFrom-Json

$indexById = @{}
foreach ($entry in $index) {
    $id = if ($entry.id) { $entry.id } elseif ($entry.variant_id) { $entry.variant_id } else { $null }
    if ($id) { $indexById[$id] = $entry }
}

function Get-Mutations($row) {
    if ($row.mutations -and $row.mutations.Count -gt 0) {
        return @($row.mutations)
    }
    $idx = $indexById[$row.variant_id]
    if ($idx -and $idx.mutations) { return @($idx.mutations) }
    if ($row.variant_id -match '^\d+-(.+)$') { return @($Matches[1]) }
    return @()
}

# 1. Per-variant table -----------------------------------------------------
Write-Host ''
Write-Host '=== Per-variant results ===' -ForegroundColor Cyan
$rows = foreach ($r in $results) {
    [pscustomobject]@{
        Variant       = $r.variant_id
        iTunes        = $r.itunes
        AppleDevices  = $r.apple_devices
        Mutations     = (Get-Mutations $r) -join ','
        Notes         = $r.notes
    }
}
$rows | Format-Table -AutoSize | Out-String | Write-Host

# 2. Per-mutation breakdown ------------------------------------------------
Write-Host '=== Per-mutation Apple Devices outcome counts ===' -ForegroundColor Cyan
$tagStats = @{}
foreach ($r in $results) {
    $verdict = if ($r.apple_devices) { $r.apple_devices } else { 'missing' }
    foreach ($tag in (Get-Mutations $r)) {
        if (-not $tagStats.ContainsKey($tag)) { $tagStats[$tag] = @{} }
        if (-not $tagStats[$tag].ContainsKey($verdict)) { $tagStats[$tag][$verdict] = 0 }
        $tagStats[$tag][$verdict]++
    }
}

$verdictCols = @($results | ForEach-Object { $_.apple_devices } | Where-Object { $_ } | Sort-Object -Unique)
if (-not $verdictCols -or $verdictCols.Count -eq 0) { $verdictCols = @('sync-ok','sync-fail-step-3') }

$tagRows = foreach ($tag in ($tagStats.Keys | Sort-Object)) {
    $obj = [ordered]@{ Tag = $tag }
    foreach ($v in $verdictCols) {
        $obj[$v] = if ($tagStats[$tag].ContainsKey($v)) { $tagStats[$tag][$v] } else { 0 }
    }
    [pscustomobject]$obj
}
$tagRows | Format-Table -AutoSize | Out-String | Write-Host

# 3. Smoking-gun section ---------------------------------------------------
Write-Host '=== Mutation tags suspicious for step-3 failure ===' -ForegroundColor Cyan
$suspects = foreach ($tag in ($tagStats.Keys | Sort-Object)) {
    $fail = if ($tagStats[$tag].ContainsKey('sync-fail-step-3')) { $tagStats[$tag]['sync-fail-step-3'] } else { 0 }
    $ok   = if ($tagStats[$tag].ContainsKey('sync-ok'))           { $tagStats[$tag]['sync-ok']           } else { 0 }
    $total = $fail + $ok
    if ($total -gt 0 -and $fail -ge 1) {
        [pscustomobject]@{
            Tag        = $tag
            FailStep3  = $fail
            SyncOk     = $ok
            FailRate   = if ($total -gt 0) { [math]::Round(100.0 * $fail / $total, 1) } else { 0 }
        }
    }
}
if ($suspects) {
    $suspects | Sort-Object FailRate -Descending | Format-Table -AutoSize | Out-String | Write-Host
} else {
    Write-Host '(no step-3 failures recorded yet)' -ForegroundColor DarkGray
}

# 4. iTunes-ok / Apple-fail flips ------------------------------------------
Write-Host '=== Variants where iTunes opened OK but Apple Devices failed ===' -ForegroundColor Cyan
$flips = $results | Where-Object {
    $_.itunes -eq 'ok' -and $_.apple_devices -like 'sync-fail*'
}
if ($flips) {
    $flips | ForEach-Object {
        [pscustomobject]@{
            Variant   = $_.variant_id
            Verdict   = $_.apple_devices
            Mutations = (Get-Mutations $_) -join ','
            Notes     = $_.notes
        }
    } | Format-Table -AutoSize | Out-String | Write-Host
} else {
    Write-Host '(none yet)' -ForegroundColor DarkGray
}

# 5. Optional JSON dump ----------------------------------------------------
if ($OutputJson) {
    $report = [pscustomobject]@{
        generated_at = (Get-Date).ToString('o')
        variants     = $rows
        per_mutation = $tagRows
        suspects     = $suspects
        itunes_ok_apple_fail = $flips | ForEach-Object {
            [pscustomobject]@{
                variant_id    = $_.variant_id
                apple_devices = $_.apple_devices
                mutations     = (Get-Mutations $_)
                notes         = $_.notes
            }
        }
    }
    $report | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $OutputJson -Encoding UTF8
    Write-Host "Wrote analysis JSON to $OutputJson" -ForegroundColor Green
}
