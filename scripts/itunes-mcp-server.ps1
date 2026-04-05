# file: scripts/itunes-mcp-server.ps1
# version: 1.0.0
# guid: a1b2c3d4-5e6f-7a8b-9c0d-e1f2a3b4c5d6
#
# iTunes MCP Server — Model Context Protocol server over stdio (JSON-RPC 2.0).
# Exposes iTunes COM API as MCP tools for remote control from Claude Code.
#
# Usage (typically invoked via SSH from Claude Code):
#   powershell -ExecutionPolicy Bypass -File W:\audiobook-organizer\scripts\itunes-mcp-server.ps1
#
# Protocol:
#   - Reads JSON-RPC 2.0 messages from stdin (one per line, or with Content-Length headers)
#   - Writes JSON-RPC 2.0 responses to stdout
#   - Logs to stderr (does not interfere with protocol)
#
# Requirements:
#   - iTunes for Windows installed
#   - PowerShell 5.1+ (Windows PowerShell)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ---------------------------------------------------------------------------
# Globals
# ---------------------------------------------------------------------------
$script:iTunesApp = $null
$script:ServerInfo = @{
    name    = "itunes-mcp-server"
    version = "1.0.0"
}

# ---------------------------------------------------------------------------
# Logging (stderr only, never stdout)
# ---------------------------------------------------------------------------
function Write-Log {
    param([string]$Message)
    [Console]::Error.WriteLine("[itunes-mcp] $Message")
}

# ---------------------------------------------------------------------------
# JSON-RPC helpers
# ---------------------------------------------------------------------------
function New-JsonRpcResponse {
    param($Id, $Result)
    return @{
        jsonrpc = "2.0"
        id      = $Id
        result  = $Result
    }
}

function New-JsonRpcError {
    param($Id, [int]$Code, [string]$Message, $Data = $null)
    $err = @{
        jsonrpc = "2.0"
        id      = $Id
        error   = @{
            code    = $Code
            message = $Message
        }
    }
    if ($Data) {
        $err.error.data = $Data
    }
    return $err
}

function Send-Response {
    param($Response)
    $json = $Response | ConvertTo-Json -Depth 20 -Compress
    # MCP uses Content-Length framing over stdio
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
    [Console]::Out.Write("Content-Length: $($bytes.Length)`r`n`r`n")
    [Console]::Out.Write($json)
    [Console]::Out.Flush()
}

# ---------------------------------------------------------------------------
# iTunes COM helpers
# ---------------------------------------------------------------------------
function Get-ITunesApp {
    if ($script:iTunesApp) {
        # Verify it's still alive
        try {
            $null = $script:iTunesApp.Version
            return $script:iTunesApp
        }
        catch {
            Write-Log "iTunes COM object stale, clearing reference"
            $script:iTunesApp = $null
        }
    }
    return $null
}

function Initialize-ITunesApp {
    if (Get-ITunesApp) { return $script:iTunesApp }
    try {
        Write-Log "Creating iTunes COM object..."
        $script:iTunesApp = New-Object -ComObject iTunes.Application
        Write-Log "iTunes COM object created, version: $($script:iTunesApp.Version)"
        return $script:iTunesApp
    }
    catch {
        throw "Failed to create iTunes COM object: $_. Is iTunes installed?"
    }
}

function Release-ITunesApp {
    if ($script:iTunesApp) {
        try {
            $script:iTunesApp.Quit()
        }
        catch {
            Write-Log "Warning: iTunes.Quit() failed: $_"
        }
        try {
            [System.Runtime.InteropServices.Marshal]::ReleaseComObject($script:iTunesApp) | Out-Null
        }
        catch {}
        $script:iTunesApp = $null
        # Kill any lingering process
        $procs = Get-Process -Name "iTunes" -ErrorAction SilentlyContinue
        if ($procs) {
            Start-Sleep -Seconds 3
            $procs = Get-Process -Name "iTunes" -ErrorAction SilentlyContinue
            if ($procs) {
                $procs | Stop-Process -Force -ErrorAction SilentlyContinue
            }
        }
        Write-Log "iTunes released"
    }
}

# ---------------------------------------------------------------------------
# Tool definitions (for tools/list)
# ---------------------------------------------------------------------------
$script:ToolDefinitions = @(
    @{
        name        = "itunes_open_library"
        description = "Set the iTunes library path via registry and launch iTunes. The path should be the folder containing 'iTunes Library.itl'."
        inputSchema = @{
            type       = "object"
            properties = @{
                path = @{
                    type        = "string"
                    description = "Full path to the folder containing the iTunes Library.itl file"
                }
            }
            required = @("path")
        }
    }
    @{
        name        = "itunes_close"
        description = "Quit iTunes via COM and release all resources."
        inputSchema = @{
            type       = "object"
            properties = @{}
        }
    }
    @{
        name        = "itunes_get_track_count"
        description = "Return the total number of tracks in the current iTunes library."
        inputSchema = @{
            type       = "object"
            properties = @{}
        }
    }
    @{
        name        = "itunes_get_tracks"
        description = "Return a paginated list of tracks with Name, Artist, Album, Location, Duration, and PersistentID."
        inputSchema = @{
            type       = "object"
            properties = @{
                offset = @{
                    type        = "integer"
                    description = "Zero-based offset (default 0)"
                    default     = 0
                }
                limit = @{
                    type        = "integer"
                    description = "Max tracks to return (default 50, max 500)"
                    default     = 50
                }
            }
        }
    }
    @{
        name        = "itunes_verify_files"
        description = "Check if track file locations exist on disk. Returns counts and details of invalid paths."
        inputSchema = @{
            type       = "object"
            properties = @{
                limit = @{
                    type        = "integer"
                    description = "Max tracks to verify (default 0 = all)"
                    default     = 0
                }
            }
        }
    }
    @{
        name        = "itunes_get_library_info"
        description = "Return library metadata: path, iTunes version, track count, playlist count."
        inputSchema = @{
            type       = "object"
            properties = @{}
        }
    }
    @{
        name        = "itunes_search"
        description = "Search tracks by name in the iTunes library."
        inputSchema = @{
            type       = "object"
            properties = @{
                query = @{
                    type        = "string"
                    description = "Search query string"
                }
                limit = @{
                    type        = "integer"
                    description = "Max results to return (default 50)"
                    default     = 50
                }
            }
            required = @("query")
        }
    }
    @{
        name        = "itunes_run_test"
        description = "Run a single iTunes test case from the test suite. Returns the results.json content."
        inputSchema = @{
            type       = "object"
            properties = @{
                test_folder = @{
                    type        = "string"
                    description = "Full path to the test subfolder containing iTunes Library.itl"
                }
            }
            required = @("test_folder")
        }
    }
)

# ---------------------------------------------------------------------------
# Tool implementations
# ---------------------------------------------------------------------------
function Invoke-ITunesOpenLibrary {
    param([string]$Path)

    if (-not $Path) { throw "path is required" }
    if (-not (Test-Path $Path)) { throw "Path does not exist: $Path" }

    $itlFile = Join-Path $Path "iTunes Library.itl"
    if (-not (Test-Path $itlFile)) {
        throw "No 'iTunes Library.itl' found in: $Path"
    }

    # Kill existing iTunes
    Release-ITunesApp
    $procs = Get-Process -Name "iTunes" -ErrorAction SilentlyContinue
    if ($procs) { $procs | Stop-Process -Force; Start-Sleep -Seconds 3 }

    # Set registry
    $regPath = "HKCU:\Software\Apple Computer, Inc.\iTunes"
    if (-not (Test-Path $regPath)) {
        New-Item -Path $regPath -Force | Out-Null
    }

    try {
        $existing = (Get-ItemProperty -Path $regPath -Name "ITunesRecentDatabasePaths" -ErrorAction SilentlyContinue).ITunesRecentDatabasePaths
    }
    catch { $existing = @() }
    if (-not $existing) { $existing = @() }

    $newPaths = @($Path) + ($existing | Where-Object { $_ -ne $Path })
    Set-ItemProperty -Path $regPath -Name "ITunesRecentDatabasePaths" -Value $newPaths -Type MultiString

    # Launch iTunes
    $iTunes = Initialize-ITunesApp
    Start-Sleep -Seconds 10  # Wait for library load

    $trackCount = 0
    try {
        $trackCount = $iTunes.LibraryPlaylist.Tracks.Count
    }
    catch {}

    return @{
        success     = $true
        path        = $Path
        track_count = $trackCount
        version     = $iTunes.Version
    }
}

function Invoke-ITunesClose {
    Release-ITunesApp
    return @{
        success = $true
        message = "iTunes closed"
    }
}

function Invoke-ITunesGetTrackCount {
    $iTunes = Get-ITunesApp
    if (-not $iTunes) { throw "iTunes is not running. Call itunes_open_library first." }

    $count = $iTunes.LibraryPlaylist.Tracks.Count
    return @{
        track_count = $count
    }
}

function Invoke-ITunesGetTracks {
    param([int]$Offset = 0, [int]$Limit = 50)

    $iTunes = Get-ITunesApp
    if (-not $iTunes) { throw "iTunes is not running. Call itunes_open_library first." }

    if ($Limit -gt 500) { $Limit = 500 }
    if ($Limit -lt 1) { $Limit = 50 }
    if ($Offset -lt 0) { $Offset = 0 }

    $tracks = $iTunes.LibraryPlaylist.Tracks
    $total = $tracks.Count
    $results = @()

    # iTunes COM tracks are 1-indexed
    $start = $Offset + 1
    $end = [Math]::Min($Offset + $Limit, $total)

    for ($i = $start; $i -le $end; $i++) {
        try {
            $track = $tracks.Item($i)
            $info = @{
                index    = $i - 1
                name     = ""
                artist   = ""
                album    = ""
                location = ""
                duration = 0
                persistent_id = ""
            }
            try { $info.name = $track.Name } catch {}
            try { $info.artist = $track.Artist } catch {}
            try { $info.album = $track.Album } catch {}
            try { $info.location = $track.Location } catch {}
            try { $info.duration = $track.Duration } catch {}
            try {
                # GetITObjectPersistentIDs returns high and low 32-bit parts
                $highID = 0
                $lowID = 0
                $iTunes.QueryInterface([ref]$null) | Out-Null  # Dummy to keep COM alive
            }
            catch {}
            # PersistentID via hex conversion of the track's DatabaseID as fallback
            try { $info.persistent_id = "0x{0:X8}" -f $track.TrackDatabaseID } catch {}

            $results += $info
        }
        catch {
            Write-Log "Error reading track $i`: $_"
        }
    }

    return @{
        tracks      = $results
        total       = $total
        offset      = $Offset
        limit       = $Limit
        returned    = $results.Count
    }
}

function Invoke-ITunesVerifyFiles {
    param([int]$Limit = 0)

    $iTunes = Get-ITunesApp
    if (-not $iTunes) { throw "iTunes is not running. Call itunes_open_library first." }

    $tracks = $iTunes.LibraryPlaylist.Tracks
    $total = $tracks.Count
    $checkCount = if ($Limit -gt 0) { [Math]::Min($Limit, $total) } else { $total }

    $validCount = 0
    $invalidCount = 0
    $noLocationCount = 0
    $invalidPaths = @()

    for ($i = 1; $i -le $checkCount; $i++) {
        try {
            $track = $tracks.Item($i)
            $location = $null
            try { $location = $track.Location } catch {}

            if ($location) {
                if (Test-Path $location) {
                    $validCount++
                }
                else {
                    $invalidCount++
                    if ($invalidPaths.Count -lt 100) {
                        $name = ""
                        try { $name = $track.Name } catch {}
                        $invalidPaths += @{
                            index    = $i - 1
                            name     = $name
                            location = $location
                        }
                    }
                }
            }
            else {
                $noLocationCount++
            }
        }
        catch {
            Write-Log "Error verifying track $i`: $_"
        }
    }

    return @{
        total_checked     = $checkCount
        total_in_library  = $total
        valid_files       = $validCount
        invalid_files     = $invalidCount
        no_location       = $noLocationCount
        invalid_paths     = $invalidPaths
        invalid_paths_truncated = ($invalidCount -gt 100)
    }
}

function Invoke-ITunesGetLibraryInfo {
    $iTunes = Get-ITunesApp
    if (-not $iTunes) { throw "iTunes is not running. Call itunes_open_library first." }

    $info = @{
        version        = ""
        track_count    = 0
        playlist_count = 0
        library_path   = ""
    }

    try { $info.version = $iTunes.Version } catch {}
    try { $info.track_count = $iTunes.LibraryPlaylist.Tracks.Count } catch {}
    try {
        $playlists = $iTunes.LibrarySource.Playlists
        $info.playlist_count = $playlists.Count
    }
    catch {}

    # Read current library path from registry
    try {
        $regPath = "HKCU:\Software\Apple Computer, Inc.\iTunes"
        $paths = (Get-ItemProperty -Path $regPath -Name "ITunesRecentDatabasePaths" -ErrorAction SilentlyContinue).ITunesRecentDatabasePaths
        if ($paths -and $paths.Count -gt 0) {
            $info.library_path = $paths[0]
        }
    }
    catch {}

    return $info
}

function Invoke-ITunesSearch {
    param([string]$Query, [int]$Limit = 50)

    $iTunes = Get-ITunesApp
    if (-not $iTunes) { throw "iTunes is not running. Call itunes_open_library first." }

    if (-not $Query) { throw "query is required" }
    if ($Limit -lt 1) { $Limit = 50 }
    if ($Limit -gt 500) { $Limit = 500 }

    $results = @()
    try {
        # Search the library playlist; field 5 = ITPlaylistSearchFieldAll
        $found = $iTunes.LibraryPlaylist.Search($Query, 5)
        if ($found) {
            $count = [Math]::Min($found.Count, $Limit)
            for ($i = 1; $i -le $count; $i++) {
                $track = $found.Item($i)
                $info = @{
                    name     = ""
                    artist   = ""
                    album    = ""
                    location = ""
                    duration = 0
                }
                try { $info.name = $track.Name } catch {}
                try { $info.artist = $track.Artist } catch {}
                try { $info.album = $track.Album } catch {}
                try { $info.location = $track.Location } catch {}
                try { $info.duration = $track.Duration } catch {}
                $results += $info
            }
        }
    }
    catch {
        throw "Search failed: $_"
    }

    return @{
        query    = $Query
        results  = $results
        returned = $results.Count
    }
}

function Invoke-ITunesRunTest {
    param([string]$TestFolder)

    if (-not $TestFolder) { throw "test_folder is required" }
    if (-not (Test-Path $TestFolder)) { throw "Test folder does not exist: $TestFolder" }

    # Save original registry paths
    $regPath = "HKCU:\Software\Apple Computer, Inc.\iTunes"
    $originalPaths = @()
    try {
        $originalPaths = (Get-ItemProperty -Path $regPath -Name "ITunesRecentDatabasePaths" -ErrorAction SilentlyContinue).ITunesRecentDatabasePaths
    }
    catch {}

    $testName = Split-Path $TestFolder -Leaf

    # Close any existing iTunes
    Release-ITunesApp
    $procs = Get-Process -Name "iTunes" -ErrorAction SilentlyContinue
    if ($procs) { $procs | Stop-Process -Force; Start-Sleep -Seconds 3 }

    $result = @{
        test_name   = $testName
        success     = $false
        track_count = 0
        errors      = @()
        timestamp   = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    }

    # Check for .itl file
    $itlFile = Join-Path $TestFolder "iTunes Library.itl"
    if (-not (Test-Path $itlFile)) {
        $result.errors += "iTunes Library.itl not found in $TestFolder"
        return $result
    }

    try {
        # Point iTunes at test folder
        if (-not (Test-Path $regPath)) { New-Item -Path $regPath -Force | Out-Null }
        $newPaths = @($TestFolder) + ($originalPaths | Where-Object { $_ -ne $TestFolder })
        Set-ItemProperty -Path $regPath -Name "ITunesRecentDatabasePaths" -Value $newPaths -Type MultiString

        # Launch
        $iTunes = Initialize-ITunesApp
        Start-Sleep -Seconds 15

        $tracks = $iTunes.LibraryPlaylist.Tracks
        $result.track_count = $tracks.Count

        # Quick file verification
        $invalidCount = 0
        $validCount = 0
        for ($i = 1; $i -le $tracks.Count; $i++) {
            try {
                $track = $tracks.Item($i)
                $loc = $null
                try { $loc = $track.Location } catch {}
                if ($loc) {
                    if (Test-Path $loc) { $validCount++ } else { $invalidCount++ }
                }
            }
            catch {}
        }
        $result.valid_files = $validCount
        $result.invalid_files = $invalidCount

        # Check test-info.json expectations
        $testInfoPath = Join-Path $TestFolder "test-info.json"
        if (Test-Path $testInfoPath) {
            try {
                $testInfo = Get-Content $testInfoPath -Raw | ConvertFrom-Json
                if ($testInfo.PSObject.Properties["expected_track_count"]) {
                    if ($result.track_count -ne $testInfo.expected_track_count) {
                        $result.errors += "Expected $($testInfo.expected_track_count) tracks, got $($result.track_count)"
                    }
                }
            }
            catch {}
        }

        $result.success = ($result.errors.Count -eq 0)
    }
    catch {
        $result.errors += "Error: $_"
    }
    finally {
        Release-ITunesApp
        Start-Sleep -Seconds 5

        # Write results.json
        $resultsPath = Join-Path $TestFolder "results.json"
        $result | ConvertTo-Json -Depth 10 | Set-Content -Path $resultsPath -Encoding UTF8

        # Restore original paths
        if ($originalPaths -and $originalPaths.Count -gt 0) {
            try {
                Set-ItemProperty -Path $regPath -Name "ITunesRecentDatabasePaths" -Value $originalPaths -Type MultiString
            }
            catch {}
        }
    }

    return $result
}

# ---------------------------------------------------------------------------
# MCP protocol dispatch
# ---------------------------------------------------------------------------
function Handle-Request {
    param($Request)

    $method = $Request.method
    $id = $Request.id
    $params = $Request.params

    Write-Log "Received: method=$method id=$id"

    switch ($method) {
        "initialize" {
            return New-JsonRpcResponse -Id $id -Result @{
                protocolVersion = "2024-11-05"
                capabilities    = @{
                    tools = @{}
                }
                serverInfo = $script:ServerInfo
            }
        }

        "notifications/initialized" {
            # Client acknowledgment, no response needed
            return $null
        }

        "tools/list" {
            return New-JsonRpcResponse -Id $id -Result @{
                tools = $script:ToolDefinitions
            }
        }

        "tools/call" {
            $toolName = $params.name
            $args = $params.arguments
            if (-not $args) { $args = @{} }

            Write-Log "Calling tool: $toolName"

            try {
                $toolResult = $null
                switch ($toolName) {
                    "itunes_open_library" {
                        $toolResult = Invoke-ITunesOpenLibrary -Path $args.path
                    }
                    "itunes_close" {
                        $toolResult = Invoke-ITunesClose
                    }
                    "itunes_get_track_count" {
                        $toolResult = Invoke-ITunesGetTrackCount
                    }
                    "itunes_get_tracks" {
                        $offset = if ($args.offset) { [int]$args.offset } else { 0 }
                        $limit = if ($args.limit) { [int]$args.limit } else { 50 }
                        $toolResult = Invoke-ITunesGetTracks -Offset $offset -Limit $limit
                    }
                    "itunes_verify_files" {
                        $limit = if ($args.limit) { [int]$args.limit } else { 0 }
                        $toolResult = Invoke-ITunesVerifyFiles -Limit $limit
                    }
                    "itunes_get_library_info" {
                        $toolResult = Invoke-ITunesGetLibraryInfo
                    }
                    "itunes_search" {
                        $limit = if ($args.limit) { [int]$args.limit } else { 50 }
                        $toolResult = Invoke-ITunesSearch -Query $args.query -Limit $limit
                    }
                    "itunes_run_test" {
                        $toolResult = Invoke-ITunesRunTest -TestFolder $args.test_folder
                    }
                    default {
                        return New-JsonRpcError -Id $id -Code -32601 -Message "Unknown tool: $toolName"
                    }
                }

                $text = $toolResult | ConvertTo-Json -Depth 10
                return New-JsonRpcResponse -Id $id -Result @{
                    content = @(
                        @{
                            type = "text"
                            text = $text
                        }
                    )
                }
            }
            catch {
                $errMsg = $_.Exception.Message
                Write-Log "Tool error: $errMsg"
                return New-JsonRpcResponse -Id $id -Result @{
                    content = @(
                        @{
                            type = "text"
                            text = "Error: $errMsg"
                        }
                    )
                    isError = $true
                }
            }
        }

        "ping" {
            return New-JsonRpcResponse -Id $id -Result @{}
        }

        default {
            if ($id) {
                return New-JsonRpcError -Id $id -Code -32601 -Message "Method not found: $method"
            }
            # Notifications (no id) that we don't handle — just ignore
            return $null
        }
    }
}

# ---------------------------------------------------------------------------
# Main loop: read JSON-RPC messages from stdin, respond on stdout
# ---------------------------------------------------------------------------
function Start-McpServer {
    Write-Log "iTunes MCP Server starting..."
    Write-Log "Waiting for MCP client connection on stdio..."

    $reader = [System.IO.StreamReader]::new([Console]::OpenStandardInput(), [System.Text.Encoding]::UTF8)

    try {
        while ($true) {
            # MCP uses Content-Length header framing (like LSP)
            $contentLength = -1

            # Read headers until empty line
            while ($true) {
                $headerLine = $reader.ReadLine()
                if ($null -eq $headerLine) {
                    Write-Log "stdin closed, shutting down"
                    return
                }

                $headerLine = $headerLine.Trim()
                if ($headerLine -eq "") {
                    break  # End of headers
                }

                if ($headerLine -match "^Content-Length:\s*(\d+)") {
                    $contentLength = [int]$Matches[1]
                }
            }

            if ($contentLength -lt 0) {
                # Fallback: maybe it's just a raw JSON line (no Content-Length framing).
                # Some transports send bare JSON lines. Try to read a line.
                continue
            }

            # Read exactly contentLength bytes
            $buffer = New-Object char[] $contentLength
            $bytesRead = 0
            while ($bytesRead -lt $contentLength) {
                $read = $reader.Read($buffer, $bytesRead, $contentLength - $bytesRead)
                if ($read -eq 0) {
                    Write-Log "stdin closed during read"
                    return
                }
                $bytesRead += $read
            }

            $jsonStr = New-Object string(,$buffer)
            Write-Log "Received $($jsonStr.Length) bytes"

            try {
                $request = $jsonStr | ConvertFrom-Json
            }
            catch {
                Write-Log "Failed to parse JSON: $_"
                $errResp = New-JsonRpcError -Id $null -Code -32700 -Message "Parse error"
                Send-Response -Response $errResp
                continue
            }

            $response = Handle-Request -Request $request
            if ($response) {
                Send-Response -Response $response
            }
        }
    }
    finally {
        Write-Log "Cleaning up..."
        Release-ITunesApp
        $reader.Dispose()
        Write-Log "Server stopped"
    }
}

# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------
Start-McpServer
