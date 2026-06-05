package patch

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestPatchTODO(t *testing.T) {
    todo := filepath.Join("..", "TODO.md")
    data, err := os.ReadFile(todo)
    if err != nil {
        t.Fatalf("read TODO: %v", err)
    }
    old := "- [ ] **ACOUSTID-STATS-2** `GET /maintenance/acoustid-stats` handler + route."
    new := "- [x] **ACOUSTID-STATS-2** `GET /maintenance/acoustid-stats` handler + route."
    if !strings.Contains(string(data), old) {
        t.Fatalf("pattern not found")
    }
    patched := strings.Replace(string(data), old, new, 1)
    if err := os.WriteFile(todo, []byte(patched), 0o644); err != nil {
        t.Fatalf("write TODO: %v", err)
    }
    testFile := filepath.Join("tmp_patch", "patch_todo_test.go")
    if err := os.Remove(testFile); err != nil {
        t.Logf("remove test file: %v", err)
    }
    if err := os.Remove("tmp_patch"); err != nil {
        t.Logf("remove tmp dir: %v", err)
    }
}
