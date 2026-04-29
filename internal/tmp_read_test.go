package internal

import (
    "os"
    "strings"
    "testing"
)

func TestTmpReadLibrary(t *testing.T) {
    path := "web/src/pages/Library.tsx"
    data, err := os.ReadFile(path)
    if err != nil {
        t.Fatalf("read %s: %v", path, err)
    }
    lower := strings.ToLower(string(data))
    idx := strings.Index(lower, "discovery")
    if idx == -1 {
        t.Fatalf("no discovery string in %s", path)
    }
    start := idx - 400
    if start < 0 {
        start = 0
    }
    end := idx + 400
    if end > len(data) {
        end = len(data)
    }
    snippet := data[start:end]
    t.Logf("snippet near 'discovery':\n%s", snippet)
}
