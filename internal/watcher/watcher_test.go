// file: internal/watcher/watcher_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package watcher

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsAudioFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"book.mp3", true},
		{"book.m4b", true},
		{"book.m4a", true},
		{"book.flac", true},
		{"book.ogg", true},
		{"book.opus", true},
		{"book.wma", true},
		{"book.aac", true},
		{"book.MP3", true},
		{"book.txt", false},
		{"book.jpg", false},
		{"book", false},
		{".mp3", true},
	}
	for _, tt := range tests {
		if got := IsAudioFile(tt.name); got != tt.want {
			t.Errorf("IsAudioFile(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestDebounceSingleEvent(t *testing.T) {
	dir := t.TempDir()

	var calls atomic.Int32
	w := New(func(rootDir string) {
		calls.Add(1)
	}, 100*time.Millisecond)

	if err := w.Start(dir); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// Create an audio file.
	f := filepath.Join(dir, "test.mp3")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + buffer.
	time.Sleep(300 * time.Millisecond)

	if c := calls.Load(); c != 1 {
		t.Errorf("expected 1 callback, got %d", c)
	}
}

func TestDebounceMultipleEvents(t *testing.T) {
	dir := t.TempDir()

	var calls atomic.Int32
	w := New(func(rootDir string) {
		calls.Add(1)
	}, 200*time.Millisecond)

	if err := w.Start(dir); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// Rapid-fire create multiple files within debounce window.
	for i := 0; i < 5; i++ {
		f := filepath.Join(dir, "test"+string(rune('a'+i))+".m4b")
		_ = os.WriteFile(f, []byte("data"), 0644)
		time.Sleep(30 * time.Millisecond)
	}

	// Wait for debounce to fire.
	time.Sleep(400 * time.Millisecond)

	if c := calls.Load(); c != 1 {
		t.Errorf("expected exactly 1 debounced callback, got %d", c)
	}
}

func TestNonAudioFilesIgnored(t *testing.T) {
	dir := t.TempDir()

	var calls atomic.Int32
	w := New(func(rootDir string) {
		calls.Add(1)
	}, 100*time.Millisecond)

	if err := w.Start(dir); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// Create non-audio files only.
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("img"), 0644)

	time.Sleep(300 * time.Millisecond)

	if c := calls.Load(); c != 0 {
		t.Errorf("expected 0 callbacks for non-audio files, got %d", c)
	}
}

func TestRecursiveWatching(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "author", "book")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	w := New(func(rootDir string) {
		calls.Add(1)
	}, 100*time.Millisecond)

	if err := w.Start(dir); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// Create audio file in nested subdir.
	_ = os.WriteFile(filepath.Join(subdir, "chapter1.flac"), []byte("audio"), 0644)

	time.Sleep(300 * time.Millisecond)

	if c := calls.Load(); c != 1 {
		t.Errorf("expected 1 callback for nested dir, got %d", c)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	w := New(func(string) {}, 100*time.Millisecond)
	if err := w.Start(dir); err != nil {
		t.Fatal(err)
	}
	w.Stop()
	w.Stop() // should not panic
}

func TestStartIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	w := New(func(string) {}, 100*time.Millisecond)
	if err := w.Start(dir); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	// Second start should be a no-op.
	if err := w.Start(dir); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteTriggers(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "book.mp3")
	_ = os.WriteFile(f, []byte("data"), 0644)

	var mu sync.Mutex
	var called bool
	w := New(func(string) {
		mu.Lock()
		called = true
		mu.Unlock()
	}, 100*time.Millisecond)

	if err := w.Start(dir); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// Give watcher time to register.
	time.Sleep(50 * time.Millisecond)

	_ = os.Remove(f)
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !called {
		t.Error("expected callback on file deletion")
	}
}
