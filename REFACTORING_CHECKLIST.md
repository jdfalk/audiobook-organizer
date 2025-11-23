<!-- file: REFACTORING_CHECKLIST.md -->
<!-- version: 1.0.0 -->
<!-- guid: f1e2d3c4-b5a6-9c8d-7e6f-5a4b3c2d1e0f -->

# LibraryFolder → ImportPath Refactoring Checklist

**Purpose**: Complete item-by-item checklist for the refactoring task.

**Instructions**: Check off each item as you complete it. Test after each major section.

---

## Part 1: Database Layer (Go)

### 1.1 Core Type Definition

**File**: `internal/database/store.go`

- [ ] Line 219-220: Rename `type LibraryFolder struct` → `type ImportPath struct`
- [ ] Line 219 comment: Update "managed library folder" → "managed import path"
- [ ] Line 56 comment: Update "// Library Folders" → "// Import Paths"

### 1.2 Store Interface Methods

**File**: `internal/database/store.go`

- [ ] Line 57: `GetAllLibraryFolders() ([]LibraryFolder, error)` → `GetAllImportPaths() ([]ImportPath, error)`
- [ ] Line 58: `GetLibraryFolderByID(id int) (*LibraryFolder, error)` → `GetImportPathByID(id int) (*ImportPath, error)`
- [ ] Line 59: `GetLibraryFolderByPath(path string) (*LibraryFolder, error)` → `GetImportPathByPath(path string) (*ImportPath, error)`
- [ ] Line 60: `CreateLibraryFolder(path, name string) (*LibraryFolder, error)` → `CreateImportPath(path, name string) (*ImportPath, error)`
- [ ] Line 61: `UpdateLibraryFolder(id int, folder *LibraryFolder) error` → `UpdateImportPath(id int, importPath *ImportPath) error`
- [ ] Line 62: `DeleteLibraryFolder(id int) error` → `DeleteImportPath(id int) error`

### 1.3 PebbleDB Store Implementation

**File**: `internal/database/pebble_store.go`

Key prefix changes:

- [ ] Line 31 comment: `library:<id>` → `import_path:<id>`, update comment "LibraryFolder JSON" → "ImportPath JSON"
- [ ] Line 42 comment: `counter:library` → `counter:import_path`, update "next library folder ID" → "next import path ID"

Method implementations:

- [ ] Line 1027 comment: "Library Folder operations" → "Import Path operations"
- [ ] Line 1029: Function `GetAllLibraryFolders` → `GetAllImportPaths`, return type `[]LibraryFolder` → `[]ImportPath`
- [ ] Line 1030: Variable `var folders []LibraryFolder` → `var importPaths []ImportPath`
- [ ] Line 1046: Variable `var folder LibraryFolder` → `var importPath ImportPath`
- [ ] Line 1056: Function `GetLibraryFolderByID` → `GetImportPathByID`, return type `*LibraryFolder` → `*ImportPath`
- [ ] Line 1067: Variable `var folder LibraryFolder` → `var importPath ImportPath`
- [ ] Line 1074: Function `GetLibraryFolderByPath` → `GetImportPathByPath`, return type `*LibraryFolder` → `*ImportPath`
- [ ] Line 1090: Call `p.GetLibraryFolderByID(id)` → `p.GetImportPathByID(id)`
- [ ] Line 1093: Function `CreateLibraryFolder` → `CreateImportPath`, return type `*LibraryFolder` → `*ImportPath`
- [ ] Line 1099: Variable `folder := &LibraryFolder{` → `importPath := &ImportPath{`
- [ ] Line 1133: Function `UpdateLibraryFolder` → `UpdateImportPath`, parameter `folder *LibraryFolder` → `importPath *ImportPath`
- [ ] Line 1144: Function `DeleteLibraryFolder` → `DeleteImportPath`
- [ ] Line 1145: Call `p.GetLibraryFolderByID(id)` → `p.GetImportPathByID(id)`

Database keys (search and replace in pebble_store.go):

- [ ] All `"library:"` prefixes → `"import_path:"`
- [ ] All `"counter:library"` → `"counter:import_path"`

### 1.4 SQLite Store Implementation

**File**: `internal/database/sqlite_store.go`

Table schema:

- [ ] Line 168: `CREATE TABLE IF NOT EXISTS library_folders` → `CREATE TABLE IF NOT EXISTS import_paths`
- [ ] Line 178: `CREATE INDEX IF NOT EXISTS idx_library_folders_path ON library_folders(path)` → `CREATE INDEX IF NOT EXISTS idx_import_paths_path ON import_paths(path)`

Method implementations:

- [ ] Line 858 comment: "Library Folder operations" → "Import Path operations"
- [ ] Line 860: Function `GetAllLibraryFolders` → `GetAllImportPaths`, return type `[]LibraryFolder` → `[]ImportPath`
- [ ] Line 862: SQL `FROM library_folders` → `FROM import_paths`
- [ ] Line 869: Variable `var folders []LibraryFolder` → `var importPaths []ImportPath`
- [ ] Line 871: Variable `var folder LibraryFolder` → `var importPath ImportPath`
- [ ] Line 881: Function `GetLibraryFolderByID` → `GetImportPathByID`, return type `*LibraryFolder` → `*ImportPath`
- [ ] Line 882: Variable `var folder LibraryFolder` → `var importPath ImportPath`
- [ ] Line 884: SQL `FROM library_folders WHERE` → `FROM import_paths WHERE`
- [ ] Line 896: Function `GetLibraryFolderByPath` → `GetImportPathByPath`, return type `*LibraryFolder` → `*ImportPath`
- [ ] Line 897: Variable `var folder LibraryFolder` → `var importPath ImportPath`
- [ ] Line 899: SQL `FROM library_folders WHERE` → `FROM import_paths WHERE`
- [ ] Line 911: Function `CreateLibraryFolder` → `CreateImportPath`, return type `*LibraryFolder` → `*ImportPath`
- [ ] Line 912: SQL `INSERT INTO library_folders` → `INSERT INTO import_paths`
- [ ] Line 920: Variable `return &LibraryFolder{` → `return &ImportPath{`
- [ ] Line 930: Function `UpdateLibraryFolder` → `UpdateImportPath`, parameter `folder *LibraryFolder` → `importPath *ImportPath`
- [ ] Line 931: SQL `UPDATE library_folders SET` → `UPDATE import_paths SET`
- [ ] Line 937: Function `DeleteLibraryFolder` → `DeleteImportPath`
- [ ] Line 938: SQL `DELETE FROM library_folders WHERE` → `DELETE FROM import_paths WHERE`

### 1.5 Database Initialization

**File**: `internal/database/database.go`

- [ ] Line 109 comment: "Create library_folders table" → "Create import_paths table"
- [ ] Line 111: SQL `CREATE TABLE IF NOT EXISTS library_folders` → `CREATE TABLE IF NOT EXISTS import_paths`

### 1.6 Web Helper Functions

**File**: `internal/database/web.go`

- [ ] Line 12 comment: "LibraryFolder" → "ImportPath"
- [ ] Line 15 comment: "Library folder operations" → "Import path operations"
- [ ] Line 17-18: Function `GetLibraryFolders() ([]LibraryFolder, error)` → `GetImportPaths() ([]ImportPath, error)`
- [ ] Line 21: SQL `FROM library_folders` → `FROM import_paths`
- [ ] Line 30: Variable `var folders []LibraryFolder` → `var importPaths []ImportPath`
- [ ] Line 32: Variable `var folder LibraryFolder` → `var importPath ImportPath`
- [ ] Line 46-47: Function `AddLibraryFolder(path, name string) (*LibraryFolder, error)` → `AddImportPath(path, name string) (*ImportPath, error)`
- [ ] Line 49: SQL `INSERT INTO library_folders` → `INSERT INTO import_paths`
- [ ] Line 62: Call `GetLibraryFolderByID` → `GetImportPathByID`
- [ ] Line 65-66: Function `GetLibraryFolderByID(id int) (*LibraryFolder, error)` → `GetImportPathByID(id int) (*ImportPath, error)`
- [ ] Line 69: SQL `FROM library_folders` → `FROM import_paths`
- [ ] Line 74: Variable `var folder LibraryFolder` → `var importPath ImportPath`
- [ ] Line 86-87: Function `UpdateLibraryFolder` → `UpdateImportPath`
- [ ] Line 89: SQL `UPDATE library_folders` → `UPDATE import_paths`
- [ ] Line 97-98: Function `RemoveLibraryFolder` → `RemoveImportPath`
- [ ] Line 99: SQL `DELETE FROM library_folders` → `DELETE FROM import_paths`

### 1.7 Database Migrations

**File**: `internal/database/migrations.go`

- [ ] Line 49: Description "Add library folders and operations tables" → "Add import paths and operations tables"
- [ ] Line 187 comment: "adds library folders and operations support" → "adds import paths and operations support"
- [ ] Line 190: Log message "Adding library folders and operations support" → "Adding import paths and operations support"

### 1.8 Database Tests

**File**: `internal/database/pebble_store_test.go`

- [ ] Line 580: Comment "TestPebbleLibraryFolders tests library folder management" → "TestPebbleImportPaths tests import path management"
- [ ] Line 581: Function name `TestPebbleLibraryFolders` → `TestPebbleImportPaths`
- [ ] Line 586 comment: "Create library folder" → "Create import path"
- [ ] Line 587: Call `CreateLibraryFolder` → `CreateImportPath`
- [ ] Line 589: Error message "Failed to create library folder" → "Failed to create import path"
- [ ] Line 594: Error message "Expected non-zero library folder ID" → "Expected non-zero import path ID"
- [ ] Line 597 comment: "Get library folder by ID" → "Get import path by ID"
- [ ] Line 598: Call `GetLibraryFolderByID` → `GetImportPathByID`
- [ ] Line 600: Error message "Failed to get library folder" → "Failed to get import path"
- [ ] Line 607 comment: "Get library folder by path" → "Get import path by path"
- [ ] Line 608: Call `GetLibraryFolderByPath` → `GetImportPathByPath`
- [ ] Line 610: Error message "Failed to get library folder by path" → "Failed to get import path by path"
- [ ] Line 617 comment: "List all library folders" → "List all import paths"
- [ ] Line 618: Call `GetAllLibraryFolders` → `GetAllImportPaths`
- [ ] Line 620: Error message "Failed to get all library folders" → "Failed to get all import paths"
- [ ] Line 624: Error message "Expected 1 library folder" → "Expected 1 import path"

**Testing after Part 1**:

```bash
# Run database tests
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/database/... -v

# Should see all tests passing with new names
```

---

## Part 2: Server/API Layer (Go)

### 2.1 Server Routes and Handlers

**File**: `internal/server/server.go`

Route definitions:

- [ ] Line 253 comment: "Library folder routes" → "Import path routes"
- [ ] Line 254: Route `api.GET("/library/folders", s.listLibraryFolders)` → `api.GET("/import-paths", s.listImportPaths)`
- [ ] Line 255: Route `api.POST("/library/folders", s.addLibraryFolder)` → `api.POST("/import-paths", s.addImportPath)`
- [ ] Line 256: Route `api.DELETE("/library/folders/:id", s.removeLibraryFolder)` → `api.DELETE("/import-paths/:id", s.removeImportPath)`

Handler functions:

- [ ] Line 870: Function `listLibraryFolders` → `listImportPaths`
- [ ] Line 875: Call `database.GlobalStore.GetAllLibraryFolders()` → `database.GlobalStore.GetAllImportPaths()`
- [ ] Line 883: Type `[]database.LibraryFolder{}` → `[]database.ImportPath{}`
- [ ] Line 889: Function `addLibraryFolder` → `addImportPath`
- [ ] Line 903: Call `database.GlobalStore.CreateLibraryFolder` → `database.GlobalStore.CreateImportPath`
- [ ] Line 910: Call `database.GlobalStore.UpdateLibraryFolder` → `database.GlobalStore.UpdateImportPath`
- [ ] Line 985: Call `database.GlobalStore.UpdateLibraryFolder` → `database.GlobalStore.UpdateImportPath`
- [ ] Line 1034: Call `database.GlobalStore.UpdateLibraryFolder` → `database.GlobalStore.UpdateImportPath`
- [ ] Line 1042: Function `removeLibraryFolder` → `removeImportPath`
- [ ] Line 1050: Error message "invalid library folder id" → "invalid import path id"
- [ ] Line 1053: Call `database.GlobalStore.DeleteLibraryFolder` → `database.GlobalStore.DeleteImportPath`

References in other server functions:

- [ ] Line 146: Call `database.GlobalStore.GetAllLibraryFolders()` → `database.GlobalStore.GetAllImportPaths()`
- [ ] Line 148: Log message "Got %d library folders" → "Got %d import paths"
- [ ] Line 150: Log message "Failed to get library folders" → "Failed to get import paths"
- [ ] Line 981 comment: "Update book count for this library folder" → "Update book count for this import path"
- [ ] Line 1107: Log message "Full rescan: including library folder %s" → "Full rescan: including library path %s"
- [ ] Line 1110 comment: "Add all library folders (import paths)" → "Add all import paths"
- [ ] Line 1111: Call `database.GlobalStore.GetAllLibraryFolders()` → `database.GlobalStore.GetAllImportPaths()`
- [ ] Line 1113: Error message "failed to get library folders" → "failed to get import paths"
- [ ] Line 1239 comment: "Update book count for this library folder" → "Update book count for this import path"
- [ ] Line 1240: Call `database.GlobalStore.GetAllLibraryFolders()` → `database.GlobalStore.GetAllImportPaths()`
- [ ] Line 1244: Call `database.GlobalStore.UpdateLibraryFolder` → `database.GlobalStore.UpdateImportPath`
- [ ] Line 1379 comment: "Trigger automatic rescan of library folder" → "Trigger automatic rescan of import paths"
- [ ] Line 1381: Log message "Starting automatic rescan of library folder" → "Starting automatic rescan of import paths"
- [ ] Line 1654: Call `database.GlobalStore.GetAllLibraryFolders()` → `database.GlobalStore.GetAllImportPaths()`
- [ ] Line 1656: Log message "Failed to get library folders" → "Failed to get import paths"
- [ ] Line 1657: Variable `importFolders = []database.LibraryFolder{}` → `importPaths = []database.ImportPath{}`
- [ ] Line 1659: Log message "Got %d library folders" → "Got %d import paths"
- [ ] Line 1695 comment: "Disk usage for library folders (cached)" → "Disk usage for import paths (cached)"

Variable renames:

- [ ] All `importFolders` → `importPaths` (or keep as is if referring to folders in general)
- [ ] All `folder` variables that refer to import paths → `importPath`
- [ ] All `folders` variables that refer to import path lists → `importPaths`

### 2.2 Server Tests

**File**: `internal/server/server_test.go`

- [ ] Line 295 comment: "TestListLibraryFolders tests listing library folders" → "TestListImportPaths tests listing import paths"
- [ ] Line 296: Function `TestListLibraryFolders` → `TestListImportPaths`
- [ ] Line 300: URL `/api/v1/library/folders` → `/api/v1/import-paths`
- [ ] Line 590 comment: "List library folders" → "List import paths"
- [ ] Line 591: Test name "list library folders" → "list import paths"
- [ ] Line 592: URL `/api/v1/library/folders` → `/api/v1/import-paths`

### 2.3 Command Line Interface

**File**: `cmd/root.go`

- [ ] Line 247 comment: "Log library folders count" → "Log import paths count"
- [ ] Line 248: Call `database.GlobalStore.GetAllLibraryFolders()` → `database.GlobalStore.GetAllImportPaths()`
- [ ] Line 249: Message "Library folders (scan paths): %d configured" → "Import paths (scan paths): %d configured"

**Testing after Part 2**:

```bash
# Build and test
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build -o ~/audiobook-organizer-embedded

# Kill old server
killall audiobook-organizer-embedded

# Start new server
cd /Users/jdfalk/ao-library
~/audiobook-organizer-embedded &

# Test new endpoint
curl -s http://localhost:8888/api/v1/import-paths | jq .

# Should return import paths list (previously at /library/folders)
```

---

## Part 3: Models and Metrics (Go)

### 3.1 Audiobook Model

**File**: `internal/models/audiobook.go`

- [ ] Line 128: Field `LibraryFolders int` tag `json:"library_folders"` → Field `ImportPaths int` tag `json:"import_paths"`

**File**: `internal/models/audiobook_test.go`

- [ ] Line 415: Field `LibraryFolders: 3,` → `ImportPaths: 3,`

### 3.2 Metrics

**File**: `internal/metrics/metrics.go`

- [ ] Line 51: Metric name `"library_folders_total"` → `"import_paths_total"`
- [ ] Line 52: Help text "Current total number of enabled library folders" → "Current total number of enabled import paths"

**Testing after Part 3**:

```bash
# Run all Go tests
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./... -v

# Should see all passing
```

---

## Part 4: Frontend TypeScript/React

### 4.1 API Client and Types

**File**: `web/src/services/api.ts`

Interface definition:

- [ ] Line 64: `export interface LibraryFolder {` → `export interface ImportPath {`

API functions:

- [ ] Line 274 comment: "Library Folders (Import Paths)" → "Import Paths"
- [ ] Line 275: Function `getLibraryFolders(): Promise<LibraryFolder[]>` → `getImportPaths(): Promise<ImportPath[]>`
- [ ] Line 276: URL `${API_BASE}/library/folders` → `${API_BASE}/import-paths`
- [ ] Line 277: Error "Failed to fetch library folders" → "Failed to fetch import paths"
- [ ] Line 282: Function `addLibraryFolder` → `addImportPath`
- [ ] Line 285: Return type `Promise<LibraryFolder>` → `Promise<ImportPath>`
- [ ] Line 286: URL `${API_BASE}/library/folders` → `${API_BASE}/import-paths`
- [ ] Line 291: Error "Failed to add library folder" → "Failed to add import path"
- [ ] Line 293 comment: "{ folder: LibraryFolder" → "{ importPath: ImportPath"
- [ ] Line 295: Cast `data.folder` → `data.importPath`, type `LibraryFolder` → `ImportPath`
- [ ] Line 299: Interface `AddLibraryFolderDetailedResponse` → `AddImportPathDetailedResponse`
- [ ] Line 300: Field `folder: LibraryFolder;` → `importPath: ImportPath;`
- [ ] Line 304: Function `addLibraryFolderDetailed` → `addImportPathDetailed`
- [ ] Line 307: Return type `Promise<AddLibraryFolderDetailedResponse>` → `Promise<AddImportPathDetailedResponse>`
- [ ] Line 308: URL `${API_BASE}/library/folders` → `${API_BASE}/import-paths`
- [ ] Line 313: Error "Failed to add library folder" → "Failed to add import path"
- [ ] Line 324: Function `removeLibraryFolder` → `removeImportPath`
- [ ] Line 325: URL `${API_BASE}/library/folders/${id}` → `${API_BASE}/import-paths/${id}`
- [ ] Line 328: Error "Failed to remove library folder" → "Failed to remove import path"

### 4.2 Component: LibraryFolderCard → ImportPathCard

**File**: Rename `web/src/components/filemanager/LibraryFolderCard.tsx` → `web/src/components/filemanager/ImportPathCard.tsx`

- [ ] Line 1: File path comment `LibraryFolderCard.tsx` → `ImportPathCard.tsx`
- [ ] Line 26: Interface `LibraryFolder` → `ImportPath`
- [ ] Line 36: Interface `LibraryFolderCardProps` → `ImportPathCardProps`
- [ ] Line 37: Field `folder: LibraryFolder` → `importPath: ImportPath`
- [ ] Line 38: Parameter `(folder: LibraryFolder)` → `(importPath: ImportPath)`
- [ ] Line 39: Parameter `(folder: LibraryFolder)` → `(importPath: ImportPath)`
- [ ] Line 42: Component name `LibraryFolderCard` → `ImportPathCard`, props type `LibraryFolderCardProps` → `ImportPathCardProps`
- [ ] Update all `folder` references to `importPath` inside component

### 4.3 Page: FileManager

**File**: `web/src/pages/FileManager.tsx`

- [ ] Line 33: Import `LibraryFolderCard` → `ImportPathCard`
- [ ] Line 34: Import `LibraryFolder` → `ImportPath`
- [ ] Line 35: Import path `LibraryFolderCard` → `ImportPathCard`
- [ ] Line 38: State `libraryFolders` → `importPaths`, type `LibraryFolder[]` → `ImportPath[]`
- [ ] Line 50: Comment `library-folders` → `import-paths`
- [ ] Line 56: Type `newFolder: LibraryFolder` → `newPath: ImportPath`
- [ ] Line 63: `setLibraryFolders` → `setImportPaths`
- [ ] Line 67: Error message "Failed to add library folder" → "Failed to add import path"
- [ ] Line 71: Parameter `folder: LibraryFolder` → `importPath: ImportPath`
- [ ] Line 72: Message "Remove library folder: ${folder.path}?" → "Remove import path: ${importPath.path}?"
- [ ] Line 76: Comment `library-folders` → `import-paths`
- [ ] Line 80: `setLibraryFolders` → `setImportPaths`
- [ ] Line 82: Error message "Failed to remove library folder" → "Failed to remove import path"
- [ ] Line 86: Parameter `folder: LibraryFolder` → `importPath: ImportPath`
- [ ] Line 89: Comment `library-folders` → `import-paths`
- [ ] Line 93: `setLibraryFolders` → `setImportPaths`
- [ ] Line 105: `setLibraryFolders` → `setImportPaths`
- [ ] Line 122: Error message "Failed to scan library folder" → "Failed to scan import path"
- [ ] Line 123: `setLibraryFolders` → `setImportPaths`
- [ ] Line 191: Button text "Add Library Folder" → "Add Import Path"
- [ ] Line 211: Heading "Library Folders" → "Import Paths"
- [ ] Line 213: `libraryFolders.length` → `importPaths.length`
- [ ] Line 215: Text "No library folders added yet. Click "Add Library Folder"" → "No import paths added yet. Click "Add Import Path""
- [ ] Line 220: Map `libraryFolders.map` → `importPaths.map`
- [ ] Line 222: Component `<LibraryFolderCard` → `<ImportPathCard`
- [ ] Line 287: Dialog title "Add Library Folder" → "Add Import Path"

### 4.4 Page: Library

**File**: `web/src/pages/Library.tsx`

- [ ] Line 73: State `hasLibraryFolders` → `hasImportPaths`
- [ ] Line 249 comment: "audiobooks and library folders" → "audiobooks and import paths"
- [ ] Line 254: Call `api.getLibraryFolders()` → `api.getImportPaths()`
- [ ] Line 306: `setHasLibraryFolders` → `setHasImportPaths`
- [ ] Line 487: Call `api.addLibraryFolderDetailed` → `api.addImportPathDetailed`
- [ ] Line 518: Call `api.getLibraryFolders()` → `api.getImportPaths()`
- [ ] Line 555: Call `api.removeLibraryFolder` → `api.removeImportPath`
- [ ] Line 572: Call `api.getLibraryFolders()` → `api.getImportPaths()`
- [ ] Line 919: Condition `!hasLibraryFolders` → `!hasImportPaths`

### 4.5 Page: Settings

**File**: `web/src/pages/Settings.tsx`

- [ ] Line 87: State `importFolders` type `api.LibraryFolder[]` → `api.ImportPath[]`
- [ ] Line 395: Call `api.getLibraryFolders()` → `api.getImportPaths()`
- [ ] Line 406: Call `api.addLibraryFolder` → `api.addImportPath`
- [ ] Line 421: Call `api.removeLibraryFolder` → `api.removeImportPath`
- [ ] Line 1702: Text "Select the library folder where organized audiobooks will be stored" → "Select the library path where organized audiobooks will be stored"

### 4.6 Page: Dashboard

**File**: `web/src/pages/Dashboard.tsx`

- [ ] Line 37: Field `library_folders: number;` → `import_paths: number;`
- [ ] Line 57: Field `library_folders: 0,` → `import_paths: 0,`
- [ ] Line 80: Log message 'Library folder_count' → 'Import path count'
- [ ] Line 90: Field `library_folders:` → `import_paths:`

### 4.7 Component: StorageTab

**File**: `web/src/components/system/StorageTab.tsx`

- [ ] Line 24: Interface `LibraryFolder` → `ImportPath`
- [ ] Line 36: Field `folders: LibraryFolder[];` → `folders: ImportPath[];`
- [ ] Line 54: Call `api.getLibraryFolders()` → `api.getImportPaths()`
- [ ] Line 172 comment: "Library Folders" → "Import Paths"

### 4.8 Component: SystemInfoTab

**File**: `web/src/components/system/SystemInfoTab.tsx`

- [ ] Line 338: Text "Library Folders" → "Import Paths"

### 4.9 Component: WelcomeWizard

**File**: `web/src/components/wizard/WelcomeWizard.tsx`

- [ ] Line 39 comment: "Set library folder path" → "Set library path"
- [ ] Line 132: Call `api.addLibraryFolder` → `api.addImportPath`

### 4.10 Page: Logs

**File**: `web/src/pages/Logs.tsx`

- [ ] Line 82: Example log "Scanning library folder: /audiobooks/import" → "Scanning import path: /audiobooks/import"

**Testing after Part 4**:

```bash
# Build frontend
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/web
npm run build

# Rebuild server with embedded frontend
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build -tags embed -o ~/audiobook-organizer-embedded

# Restart and test in browser
killall audiobook-organizer-embedded
cd /Users/jdfalk/ao-library
~/audiobook-organizer-embedded &

# Open http://localhost:8888 and test:
# - Settings page import paths section
# - File Manager page
# - Dashboard import path count
```

---

## Part 5: Documentation and Comments

### 5.1 README.md

**File**: `README.md`

- [ ] Update glossary section if it contains any outdated references
- [ ] Search for "library folder" and update to "import path" where referring to monitored folders
- [ ] Ensure distinction between "Library" (root_dir) and "Import Path" (monitored folders) is clear

### 5.2 Code Comments

Search entire codebase for comment updates:

- [ ] "library folder" → "import path" (when referring to monitored folders)
- [ ] "library folders" → "import paths" (when referring to monitored folders)
- [ ] Ensure "library" alone still refers to the root_dir collection

**Global search commands**:

```bash
# Find remaining references
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
grep -r "library folder" --include="*.go" --include="*.ts" --include="*.tsx"
grep -r "library_folder" --include="*.go" --include="*.ts" --include="*.tsx"
grep -r "LibraryFolder" --include="*.go" --include="*.ts" --include="*.tsx"
```

---

## Part 6: Final Testing and Verification

### 6.1 Backend Tests

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer

# Run all tests
go test ./... -v

# Run specific packages
go test ./internal/database/... -v
go test ./internal/server/... -v
go test ./internal/models/... -v
```

### 6.2 Frontend Tests

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/web

# Run tests
npm test
```

### 6.3 Integration Testing

```bash
# Build and start server
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build -tags embed -o ~/audiobook-organizer-embedded
killall audiobook-organizer-embedded
cd /Users/jdfalk/ao-library
~/audiobook-organizer-embedded &

# Test API endpoints
curl -s http://localhost:8888/api/v1/import-paths | jq .
curl -X POST http://localhost:8888/api/v1/import-paths \
  -H "Content-Type: application/json" \
  -d '{"path":"/test/path","name":"Test Path"}' | jq .

# Test frontend
# Open http://localhost:8888
# Navigate to Settings → Import Paths
# Verify UI shows "Import Paths" not "Library Folders"
# Add/remove import paths
# Check Dashboard shows "Import Paths: N"
```

### 6.4 Database Migration

**Important**: This refactoring does NOT require a database migration if done correctly. The table/key names change but data structure remains the same.

However, if you want to rename the actual database table/keys:

**For SQLite** (if used):

```sql
-- Rename table
ALTER TABLE library_folders RENAME TO import_paths;

-- Recreate index with new name
DROP INDEX IF EXISTS idx_library_folders_path;
CREATE INDEX idx_import_paths_path ON import_paths(path);
```

**For PebbleDB** (if used):

Keys will need to be migrated in code. Add a migration function:

```go
// Migrate library:<id> keys to import_path:<id>
func migrateLibraryFolderKeys(db *pebble.DB) error {
    iter := db.NewIter(&pebble.IterOptions{
        LowerBound: []byte("library:"),
        UpperBound: []byte("library;"),
    })
    defer iter.Close()

    for iter.First(); iter.Valid(); iter.Next() {
        oldKey := string(iter.Key())
        value := iter.Value()

        // Create new key
        newKey := strings.Replace(oldKey, "library:", "import_path:", 1)

        // Write new key
        if err := db.Set([]byte(newKey), value, pebble.Sync); err != nil {
            return err
        }

        // Delete old key
        if err := db.Delete([]byte(oldKey), pebble.Sync); err != nil {
            return err
        }
    }

    // Migrate counter
    if val, closer, err := db.Get([]byte("counter:library")); err == nil {
        db.Set([]byte("counter:import_path"), val, pebble.Sync)
        db.Delete([]byte("counter:library"), pebble.Sync)
        closer.Close()
    }

    return nil
}
```

**Note**: This migration is optional. The refactoring can work with the old database by just changing code references. Decide based on project requirements.

---

## Part 7: Commit and PR

### 7.1 Final Rebase

```bash
# Update main one last time
git checkout main
git pull origin main

# Rebase your branch
git checkout your-refactoring-branch
git rebase main

# Resolve any conflicts
# Test everything again after rebase
```

### 7.2 Commit Message

```bash
git add .
git commit -m "refactor: rename LibraryFolder to ImportPath throughout codebase

BREAKING CHANGE: API endpoints changed from /library/folders to /import-paths

- Renamed database type LibraryFolder → ImportPath
- Updated all Store interface methods
- Changed API routes: /api/v1/library/folders → /api/v1/import-paths
- Updated frontend TypeScript interface and API client
- Renamed React component LibraryFolderCard → ImportPathCard
- Updated all references in UI components
- Improved terminology clarity: 'library' = root_dir, 'import path' = monitored folder

Affects ~150+ occurrences across:
- internal/database/*.go (store interface, implementations, tests)
- internal/server/server.go (API handlers and routes)
- internal/models/*.go (audiobook model)
- internal/metrics/metrics.go (prometheus metrics)
- web/src/**/*.{ts,tsx} (frontend API and components)

Tested:
- All Go tests passing
- Frontend builds successfully
- API endpoints functional
- UI displays correct terminology"
```

### 7.3 Create Pull Request

```bash
git push origin your-refactoring-branch
```

Create PR on GitHub with:

**Title**: `refactor: rename LibraryFolder to ImportPath for clarity`

**Description**:

```markdown
## Summary

Renames `LibraryFolder` to `ImportPath` throughout the codebase to resolve confusing terminology where "library folders" actually referred to monitored import paths, not the main library.

## Motivation

- The database table `library_folders` stored import paths (external monitored directories)
- This confused the concept of "library" (the root_dir organized collection)
- Terminology now matches the glossary in README.md

## Changes

### Backend (Go)
- Renamed `LibraryFolder` type → `ImportPath` in `internal/database/store.go`
- Updated all Store interface methods (`GetAllLibraryFolders` → `GetAllImportPaths`, etc.)
- Changed PebbleDB key prefixes: `library:` → `import_path:`
- Changed SQLite table: `library_folders` → `import_paths`
- Updated API routes: `/api/v1/library/folders` → `/api/v1/import-paths`
- Updated handler functions: `listLibraryFolders` → `listImportPaths`

### Frontend (TypeScript/React)
- Renamed `LibraryFolder` interface → `ImportPath`
- Updated API client functions: `getLibraryFolders` → `getImportPaths`
- Renamed component: `LibraryFolderCard` → `ImportPathCard`
- Updated all React components (Settings, FileManager, Library, Dashboard)
- Updated UI text: "Library Folders" → "Import Paths"

### Tests
- Updated all test names and assertions
- All tests passing after refactoring

## Testing

- ✅ All Go unit tests passing
- ✅ Frontend builds successfully
- ✅ API endpoints functional at new URLs
- ✅ UI correctly displays "Import Paths" terminology
- ✅ Manual testing of add/remove import paths

## Breaking Changes

⚠️ **API endpoints changed**:
- Old: `GET /api/v1/library/folders`
- New: `GET /api/v1/import-paths`

Frontend automatically updated to use new endpoints.

## Database Migration

No database migration required - data structure unchanged. Table/key names can be migrated optionally (see REFACTORING_CHECKLIST.md).

## Checklist

- [x] All occurrences of LibraryFolder renamed to ImportPath
- [x] API routes updated
- [x] Frontend API client updated
- [x] All tests updated and passing
- [x] UI text updated
- [x] Documentation reviewed
- [x] Rebased against latest main
```

---

## Completion Checklist

- [ ] All items in Parts 1-7 checked off
- [ ] All tests passing (Go and TypeScript)
- [ ] Server builds successfully
- [ ] Frontend builds successfully
- [ ] Manual testing completed
- [ ] Final rebase completed
- [ ] Commit created with proper message
- [ ] PR created with detailed description
- [ ] Requested review from team

---

## Support and Questions

If you encounter issues during this refactoring:

1. Check that you've completed all items in the checklist for each part
2. Run tests frequently to catch issues early
3. Test API endpoints with curl after backend changes
4. Test UI after frontend changes
5. Rebase frequently to avoid merge conflicts

Common issues:

- **Type errors**: Make sure all variable names match new types
- **Import errors**: Update import paths for renamed files
- **API 404s**: Verify routes match between frontend and backend
- **Test failures**: Update test assertions to use new names

Good luck with the refactoring! 🚀
