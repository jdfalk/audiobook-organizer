//go:build restore_wire_handlers
// +build restore_wire_handlers

package server

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type treeEntry struct {
	mode string
	name string
	hash string
}

func TestRestoreWireHandlers(t *testing.T) {
	data, err := catGitBlob("internal/server/wire_handlers.go")
	if err != nil {
		t.Fatalf("failed to read git blob: %v", err)
	}
	if err := os.WriteFile("internal/server/wire_handlers.go", data, 0o644); err != nil {
		t.Fatalf("failed to write wire_handlers.go: %v", err)
	}
}

func catGitBlob(path string) ([]byte, error) {
	head, err := readHeadCommit()
	if err != nil {
		return nil, err
	}
	tree, err := readCommitTree(head)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(path, string(os.PathSeparator))
	hash, err := findBlobHash(tree, parts)
	if err != nil {
		return nil, err
	}
	data, objType, err := readObject(hash)
	if err != nil {
		return nil, err
	}
	if objType != "blob" {
		return nil, fmt.Errorf("expected blob object, got %s", objType)
	}
	return data, nil
}

func readHeadCommit() (string, error) {
	headData, err := os.ReadFile(filepath.Join(".git", "HEAD"))
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(string(headData))
	if strings.HasPrefix(head, "ref: ") {
		rel := strings.TrimSpace(strings.TrimPrefix(head, "ref: "))
		refPath := filepath.Join(".git", rel)
		refData, err := os.ReadFile(refPath)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(refData)), nil
	}
	return head, nil
}

func readCommitTree(hash string) (string, error) {
	data, objType, err := readObject(hash)
	if err != nil {
		return "", err
	}
	if objType != "commit" {
		return "", fmt.Errorf("object %s is not a commit", hash)
	}
	lines := bytes.Split(data, []byte{'\n'})
	for _, line := range lines {
		if bytes.HasPrefix(line, []byte("tree ")) {
			return string(line[len("tree "):]), nil
		}
	}
	return "", fmt.Errorf("commit %s missing tree", hash)
}

func findBlobHash(tree string, parts []string) (string, error) {
	current := tree
	for i, part := range parts {
		entries, err := readTreeEntries(current)
		if err != nil {
			return "", err
		}
		var entry *treeEntry
		for _, e := range entries {
			if e.name == part {
				entry = &e
				break
			}
		}
		if entry == nil {
			return "", fmt.Errorf("path segment %s not found", part)
		}
		if i == len(parts)-1 {
			return entry.hash, nil
		}
		if entry.mode != "40000" {
			return "", fmt.Errorf("path segment %s is not a directory", part)
		}
		current = entry.hash
	}
	return "", fmt.Errorf("path %s not found", strings.Join(parts, "/"))
}

func readTreeEntries(hash string) ([]treeEntry, error) {
	data, objType, err := readObject(hash)
	if err != nil {
		return nil, err
	}
	if objType != "tree" {
		return nil, fmt.Errorf("object %s is not a tree", hash)
	}
	var entries []treeEntry
	for i := 0; i < len(data); {
		space := bytes.IndexByte(data[i:], ' ')
		if space == -1 {
			break
		}
		mode := string(data[i : i+space])
		i += space + 1
		zero := bytes.IndexByte(data[i:], 0)
		if zero == -1 {
			break
		}
		name := string(data[i : i+zero])
		i += zero + 1
		if i+20 > len(data) {
			return nil, fmt.Errorf("tree entry truncated")
		}
		hash := fmt.Sprintf("%x", data[i:i+20])
		i += 20
		entries = append(entries, treeEntry{mode: mode, name: name, hash: hash})
	}
	return entries, nil
}

func readObject(hash string) ([]byte, string, error) {
	objPath := filepath.Join(".git", "objects", hash[:2], hash[2:])
	f, err := os.Open(objPath)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	zr, err := zlib.NewReader(f)
	if err != nil {
		return nil, "", err
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, "", err
	}
	null := bytes.IndexByte(raw, 0)
	if null == -1 {
		return nil, "", fmt.Errorf("invalid object %s", hash)
	}
	header := string(raw[:null])
	parts := strings.SplitN(header, " ", 2)
	if len(parts) < 1 {
		return nil, "", fmt.Errorf("invalid header for object %s", hash)
	}
	return raw[null+1:], parts[0], nil
}
