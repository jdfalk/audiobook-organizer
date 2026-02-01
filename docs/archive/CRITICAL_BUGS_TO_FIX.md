<!-- file: CRITICAL_BUGS_TO_FIX.md -->
<!-- version: 1.0.0 -->
<!-- guid: bugs-critical-fixes-needed -->
<!-- last-edited: 2026-01-27 -->

# Critical Bugs to Fix - Manual Testing Results

**Date**: 2026-01-27
**Status**: 🔴 **BLOCKING ISSUES** - Server running, but UI has critical problems
**Priority**: P0 - Must fix before MVP release
**Server Status**: ✅ Running at http://localhost:8080

---

## 🐛 Bug #1: Browse Button Broken in Welcome Dialog

### Problem
The "Browse" button in the initial welcome/setup dialog doesn't work correctly.

### Issues
1. **Sets wrong default**: Uses `/home` instead of user's home directory
2. **Browse replaces value**: Clicking "Browse" button replaces whatever text was there with `/home`
3. **No file picker**: Browse button doesn't open a file/folder picker dialog

### Expected Behavior
- Default should be user's home directory (e.g., `/Users/jdfalk` on macOS, `/home/username` on Linux)
- Browse button should open a native folder picker dialog
- Selected folder should populate the text field without replacing existing text unless user selects a folder

### Steps to Reproduce
1. Open app at http://localhost:8080
2. See welcome dialog with library path field
3. Notice default value is `/home` (wrong on macOS)
4. Click "Browse" button
5. Observe: Text field changes to `/home` instead of opening a picker

### Files to Check
**Frontend**:
- `web/src/components/setup/WelcomeDialog.tsx` or similar
- `web/src/components/common/ServerFileBrowser.tsx` (file browser component)
- Look for initial setup/onboarding components

**Backend**:
- `cmd/server/handlers/filesystem.go` - Browse filesystem endpoint
- Check if there's a "get home directory" endpoint

### Suggested Fix

**Frontend** - Update default path:
```typescript
// In WelcomeDialog.tsx or similar
const [libraryPath, setLibraryPath] = useState('');

useEffect(() => {
  // Get user's home directory from backend
  api.getHomeDirectory().then(homePath => {
    setLibraryPath(homePath);
  });
}, []);
```

**Backend** - Add home directory endpoint if missing:
```go
// In cmd/server/handlers/filesystem.go
func (h *FilesystemHandler) GetHomeDirectory(c *gin.Context) {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get home directory"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"path": homeDir})
}

// Register route
api.GET("/filesystem/home", h.GetHomeDirectory)
```

**Frontend** - Fix browse button:
```typescript
const handleBrowse = async () => {
  try {
    // Use ServerFileBrowser component or API call
    const result = await openFolderPicker(libraryPath || homeDirectory);
    if (result) {
      setLibraryPath(result);
    }
  } catch (error) {
    console.error('Browse failed:', error);
  }
};
```

---

## 🐛 Bug #2: App Keeps Disconnecting from Backend

### Problem
The frontend loses connection to the backend repeatedly in both Safari and Chrome.

### Symptoms
- Connection drops after a few seconds/minutes
- API requests fail with network errors
- May show "Server disconnected" or similar messages
- SSE (Server-Sent Events) connections may be dropping

### Expected Behavior
- Persistent connection to backend
- Automatic reconnection if connection is lost
- User notified of connection issues

### Possible Causes
1. **CORS issues** - Misconfigured CORS headers
2. **SSE connection drops** - Server-sent events timing out
3. **Keepalive missing** - No heartbeat/ping to keep connection alive
4. **Timeout too short** - Server or client timeout too aggressive
5. **WebSocket/SSE issue** - If using real-time updates

### Files to Check
**Backend**:
- `cmd/server/main.go` - CORS configuration
- `cmd/server/handlers/events.go` - SSE endpoint if exists
- Server timeout configuration

**Frontend**:
- `web/src/services/api.ts` - API client configuration
- SSE/WebSocket connection code
- Error handling and reconnection logic

### Debug Steps
1. Open browser DevTools → Network tab
2. Watch for failed requests
3. Check Console for errors
4. Monitor SSE connections (look for `/api/events` or similar)

### Suggested Fixes

**Backend** - Check CORS:
```go
// In cmd/server/main.go
func setupCORS(r *gin.Engine) {
    r.Use(cors.New(cors.Config{
        AllowOrigins:     []string{"*"}, // Or specific origins
        AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
        AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
        ExposeHeaders:    []string{"Content-Length"},
        AllowCredentials: true,
        MaxAge:           12 * time.Hour,
    }))
}
```

**Backend** - Add keepalive for SSE:
```go
// In SSE handler
func (h *Handler) HandleSSE(c *gin.Context) {
    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")

    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            // Send keepalive ping
            c.SSEvent("ping", time.Now().Unix())
            c.Writer.Flush()
        case <-c.Request.Context().Done():
            return
        }
    }
}
```

**Frontend** - Add reconnection logic:
```typescript
// In api.ts or similar
class ApiClient {
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;

  async request(url: string, options?: RequestInit) {
    try {
      const response = await fetch(url, {
        ...options,
        keepalive: true, // Keep connection alive
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      this.reconnectAttempts = 0; // Reset on success
      return response;
    } catch (error) {
      if (this.reconnectAttempts < this.maxReconnectAttempts) {
        this.reconnectAttempts++;
        console.log(`Reconnecting... (attempt ${this.reconnectAttempts})`);
        await new Promise(resolve => setTimeout(resolve, 1000 * this.reconnectAttempts));
        return this.request(url, options); // Retry
      }
      throw error;
    }
  }
}
```

---

## 🐛 Bug #3: Welcome Screen Shows Repeatedly

### Problem
The initial welcome/setup screen keeps appearing every time the app loads, even after completing setup.

### Expected Behavior
- Welcome screen should show only on first launch (when no config exists)
- After completing setup, should go straight to dashboard
- Should store "setup completed" flag in config or local storage

### Possible Causes
1. Config not persisting to backend
2. Frontend not checking if setup is complete
3. Local storage not being used to track setup status
4. Backend not returning setup status in config

### Files to Check
**Frontend**:
- Welcome/setup dialog component
- App.tsx or main router component
- Check for setup completion check

**Backend**:
- `internal/config/config.go` - Configuration structure
- Check if there's a `setup_complete` or `initialized` field
- API endpoint that returns config

### Suggested Fix

**Backend** - Add setup tracking to config:
```go
// In internal/config/config.go
type Config struct {
    // ... existing fields ...
    SetupComplete bool `yaml:"setup_complete" json:"setup_complete"`
    RootDir       string `yaml:"root_dir" json:"root_dir"`
}

// Mark setup as complete
func MarkSetupComplete(cfg *Config) error {
    cfg.SetupComplete = true
    return SaveConfig(cfg)
}
```

**Backend** - Update config endpoint:
```go
// In cmd/server/handlers/config.go
func (h *ConfigHandler) UpdateConfig(c *gin.Context) {
    var updates map[string]interface{}
    if err := c.ShouldBindJSON(&updates); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // If root_dir is set, mark setup as complete
    if rootDir, ok := updates["root_dir"].(string); ok && rootDir != "" {
        h.config.SetupComplete = true
        h.config.RootDir = rootDir
    }

    if err := config.SaveConfig(h.config); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save config"})
        return
    }

    c.JSON(http.StatusOK, h.config)
}
```

**Frontend** - Check setup status:
```typescript
// In App.tsx or main component
const [showWelcome, setShowWelcome] = useState(false);
const [loading, setLoading] = useState(true);

useEffect(() => {
  const checkSetup = async () => {
    try {
      const config = await api.getConfig();
      // Show welcome if setup not complete OR root_dir not set
      setShowWelcome(!config.setup_complete || !config.root_dir);
    } catch (error) {
      console.error('Failed to check setup status:', error);
      setShowWelcome(true); // Show welcome on error
    } finally {
      setLoading(false);
    }
  };

  checkSetup();
}, []);

return loading ? (
  <CircularProgress />
) : showWelcome ? (
  <WelcomeDialog onComplete={() => setShowWelcome(false)} />
) : (
  <Router>
    {/* Main app routes */}
  </Router>
);
```

---

## 🐛 Bug #4: Can't Import File & Multiple File Selection

### Problem
1. Cannot import a file (import functionality broken)
2. Import dialog should allow selecting multiple files at once

### Expected Behavior
- User can select one or more audiobook files to import
- Multiple file selection should be supported (Cmd+Click or Shift+Click)
- Files are imported into the database and shown in Library

### Steps to Reproduce
1. Try to import a file (via UI or drag-drop)
2. Observe: Import fails or doesn't work
3. Try to select multiple files
4. Observe: Can only select one file at a time

### Files to Check
**Frontend**:
- Import dialog component
- File input element - check for `multiple` attribute
- Drag-drop handler

**Backend**:
- `cmd/server/handlers/import.go` or similar
- Import file endpoint
- Check if batch import is supported

### Suggested Fix

**Frontend** - Enable multiple file selection:
```typescript
// In import dialog component
<input
  type="file"
  multiple // Add this attribute
  accept=".m4b,.mp3,.m4a,.aac,.flac,.ogg,.wma" // Audiobook formats
  onChange={handleFileSelect}
  style={{ display: 'none' }}
  ref={fileInputRef}
/>

const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
  const files = Array.from(e.target.files || []);
  if (files.length === 0) return;

  try {
    setImporting(true);

    // Import files in parallel or sequentially
    for (const file of files) {
      await api.importFile(file);
    }

    setSuccess(`Imported ${files.length} files successfully`);
  } catch (error) {
    setError(`Failed to import files: ${error.message}`);
  } finally {
    setImporting(false);
  }
};
```

**Backend** - Ensure import endpoint works:
```go
// In cmd/server/handlers/import.go
func (h *ImportHandler) ImportFile(c *gin.Context) {
    file, err := c.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
        return
    }

    // Save file to temp location
    tempPath := filepath.Join(os.TempDir(), file.Filename)
    if err := c.SaveUploadedFile(file, tempPath); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
        return
    }

    // Scan the file and add to database
    audiobook, err := h.scanner.ScanFile(tempPath)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to scan file: %v", err)})
        return
    }

    // Save to database
    if err := h.store.CreateAudiobook(audiobook); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save audiobook: %v", err)})
        return
    }

    c.JSON(http.StatusOK, audiobook)
}

// Register route
api.POST("/import/file", h.ImportFile)
```

**Frontend** - Add drag-drop support:
```typescript
const handleDragOver = (e: React.DragEvent) => {
  e.preventDefault();
  e.stopPropagation();
};

const handleDrop = async (e: React.DragEvent) => {
  e.preventDefault();
  e.stopPropagation();

  const files = Array.from(e.dataTransfer.files);
  // Filter for audiobook files
  const audioFiles = files.filter(f =>
    /\.(m4b|mp3|m4a|aac|flac|ogg|wma)$/i.test(f.name)
  );

  if (audioFiles.length === 0) {
    setError('No audiobook files found');
    return;
  }

  // Import all dropped files
  for (const file of audioFiles) {
    await api.importFile(file);
  }
};

<Box
  onDragOver={handleDragOver}
  onDrop={handleDrop}
  sx={{
    border: '2px dashed #ccc',
    borderRadius: 2,
    p: 4,
    textAlign: 'center',
    cursor: 'pointer',
  }}
>
  <Typography>Drag & drop audiobook files here</Typography>
  <Typography variant="caption">or</Typography>
  <Button onClick={() => fileInputRef.current?.click()}>
    Browse Files
  </Button>
</Box>
```

---

## 🐛 Bug #5: Settings Page Completely Broken

### Problem
The Settings page doesn't work at all.

### Symptoms (Need to Specify)
- Does it load but show errors?
- Does it crash/blank screen?
- Do changes not save?
- Do controls not respond?
- Network errors?

### Files to Check
**Frontend**:
- `web/src/pages/Settings.tsx` - Main settings page
- Check browser console for errors
- Check Network tab for failed API calls

**Backend**:
- `cmd/server/handlers/config.go` - Config endpoints
- Check server logs for errors

### Debug Steps
1. Open http://localhost:8080/settings
2. Open browser DevTools → Console
3. Look for JavaScript errors
4. Check Network tab for failed requests
5. Try to interact with settings controls
6. Check server logs: `tail -f /tmp/audiobook-server.log`

### Common Issues to Check

**TypeScript errors preventing render**:
```bash
# Check browser console for:
# - "blocker?.proceed is not a function"
# - Type errors
# - Undefined variable errors
```

**API endpoint missing**:
```typescript
// Check if these endpoints exist:
GET  /api/config
PUT  /api/config
GET  /api/import-paths
POST /api/import-paths
```

**Navigation blocker issue**:
```typescript
// In Settings.tsx, check around line 1220
// The blocker code had TypeScript errors - may need fixing:
const handleSaveAndNavigate = async () => {
  const success = await handleSave();
  if (success && blocker) {
    blocker.proceed?.(); // Use optional chaining
  }
};
```

### Suggested Investigation

**Frontend debugging**:
```typescript
// Add console logs to Settings.tsx
useEffect(() => {
  console.log('Settings mounted');
  console.log('Current config:', config);
  console.log('Has unsaved changes:', hasUnsavedChanges);
}, []);

const handleSave = async () => {
  console.log('Saving settings...', settings);
  try {
    const result = await api.updateConfig(settings);
    console.log('Save successful:', result);
  } catch (error) {
    console.error('Save failed:', error);
  }
};
```

**Backend debugging**:
```go
// Add logging to handlers
func (h *ConfigHandler) UpdateConfig(c *gin.Context) {
    log.Printf("Received config update request")

    var updates map[string]interface{}
    if err := c.ShouldBindJSON(&updates); err != nil {
        log.Printf("Failed to parse config: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    log.Printf("Config updates: %+v", updates)
    // ... rest of handler
}
```

---

## 🔧 Quick Fixes Summary

### Priority Order
1. **Bug #2 (Disconnection)** - Blocks all other testing
2. **Bug #3 (Welcome loop)** - Annoying, prevents normal use
3. **Bug #5 (Settings broken)** - Blocks configuration
4. **Bug #1 (Browse button)** - UX issue but not blocking
5. **Bug #4 (Import)** - Feature addition, nice to have

### Testing After Fixes

**Bug #2 Fix Verification**:
```bash
# Monitor connection stability
open http://localhost:8080
# Leave open for 5-10 minutes
# Check if API calls still work
curl http://localhost:8080/api/health
```

**Bug #3 Fix Verification**:
```bash
# Clear browser data (to reset)
# Open app, complete setup
# Close browser
# Reopen app
# Should go straight to dashboard, NOT welcome screen
```

**Bug #5 Fix Verification**:
```bash
# Open Settings page
# Try to change library path
# Click Save
# Check if persists after page reload
```

---

## 📋 Development Environment

**Server**:
- Running at: http://localhost:8080
- PID: Check `/tmp/audiobook-server.pid`
- Logs: `/tmp/audiobook-server.log`
- Database: PebbleDB with 2 books, 2 authors

**Build Info**:
- Go version: 1.25
- Frontend: React 18 + TypeScript + Material-UI v5
- Build: Embedded frontend (built with `go build -tags embed_frontend`)

**Stop/Restart**:
```bash
# Stop
kill $(cat /tmp/audiobook-server.pid)

# Rebuild and restart
go build -tags embed_frontend -o audiobook-organizer .
nohup ./audiobook-organizer serve --port 8080 > /tmp/audiobook-server.log 2>&1 &
echo $! > /tmp/audiobook-server.pid
```

---

## 🎯 Success Criteria

After fixing these bugs, user should be able to:
- ✅ Open app and see welcome screen ONCE
- ✅ Browse and select home directory properly
- ✅ Complete setup and have it persist
- ✅ Navigate to Settings without crashes
- ✅ Change settings and save successfully
- ✅ Import single or multiple audiobook files
- ✅ Use app for 10+ minutes without disconnection
- ✅ Refresh browser and stay logged in/configured

---

**End of Bug Report**

**Created**: 2026-01-27
**Tested By**: User (manual testing)
**Next Step**: Fix bugs in priority order, test fixes, iterate
