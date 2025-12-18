<!-- file: docs/mvp-tasks/task-3/ROOT-CAUSE-ANALYSIS.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a-4b5c-6d7e -->

# Task 3 Root Cause Analysis

**Date:** December 14, 2025 **Found In:** `internal/server/server.go` function
`getSystemStatus()` **Severity:** CRITICAL - MVP-blocking

---

## Issue Description

The `/api/v1/system/status` endpoint can return negative `import_size_bytes`
values due to a mathematical error.

---

## Root Cause

**Location:** Line 1761 in `internal/server/server.go`

```go
importSize = totalSize - librarySize  // BUG: Can be negative!
```

**Problem:** The code calculates sizes independently but then subtracts them:

- `totalSize` = cached sum of files in import paths
- `librarySize` = sum of files in root directory
- `importSize = totalSize - librarySize` ← **Can go negative if librarySize >
  totalSize**

**Why it happens:**

- Import path cache may be stale or smaller than actual library size
- If `librarySize > totalSize`, result is negative

---

## The Fix

Calculate sizes independently without subtraction:

```go
// Calculate size for library
librarySize := int64(0)
if rootDir != "" {
    filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
        if err == nil && !info.IsDir() {
            librarySize += info.Size()
        }
        return nil
    })
}

// Calculate size for import paths independently
importSize := int64(0)
for _, folder := range importFolders {
    if !folder.Enabled {
        continue
    }
    filepath.Walk(folder.Path, func(path string, info os.FileInfo, err error) error {
        if err == nil && !info.IsDir() {
            // Skip if already in library (avoid double counting)
            if rootDir != "" && strings.HasPrefix(path, rootDir) {
                return nil
            }
            importSize += info.Size()
        }
        return nil
    })
}

totalSize := librarySize + importSize
```

---

## Additional Issues to Address

1. **Cache naming:** `cachedLibrarySize` misleadingly caches import sizes
2. **Overlapping paths:** Files in rootDir subdirectories could be
   double-counted
3. **API response fields:** Should be named `library_size_bytes`,
   `import_size_bytes`, `total_size_bytes` not just `total_size`

---
