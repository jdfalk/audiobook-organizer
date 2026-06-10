// file: internal/itunes/location_pair_test.go
// version: 1.0.0
// guid: 9c1f6b2e-7d40-4a83-b5e9-1c2a8f0d63b7

// Tests for the LocationPair single-source-of-truth type (fable5 TASK-006,
// CRIT-2 / SPEC §1b). Covers construction edge cases (spaces, % in filenames,
// UNC, non-ASCII), the bidirectional round-trip property, and rejection of the
// historical corruption shapes (URL-in-0x0D, staging-dir leak, relative paths).

package itunes

import (
	"strings"
	"testing"
)

func TestLocationPairFromWinPath_DerivesURL(t *testing.T) {
	cases := []struct {
		name    string
		winPath string
		wantURL string
	}{
		{
			name:    "plain ascii path",
			winPath: `W:\itunes\iTunes Media\Audiobooks\Adrian Tchaikovsky\01 Children of Time - 1.mp3`,
			wantURL: "file://localhost/W:/itunes/iTunes%20Media/Audiobooks/Adrian%20Tchaikovsky/01%20Children%20of%20Time%20-%201.mp3",
		},
		{
			name:    "no spaces",
			winPath: `C:\Music\track.m4b`,
			wantURL: "file://localhost/C:/Music/track.m4b",
		},
		{
			name:    "percent literal in filename is escaped",
			winPath: `W:\books\50% Off.mp3`,
			wantURL: "file://localhost/W:/books/50%25%20Off.mp3",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := LocationPairFromWinPath(tc.winPath)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.WinPath != tc.winPath {
				t.Errorf("WinPath = %q, want %q", p.WinPath, tc.winPath)
			}
			if p.URL != tc.wantURL {
				t.Errorf("URL = %q, want %q", p.URL, tc.wantURL)
			}
			// The derived URL MUST equal the T003 guard's canonical rendering, or
			// the location-form guard would reject our own output.
			if got := winPathToLocalURL(tc.winPath); got != p.URL {
				t.Errorf("URL %q != guard winPathToLocalURL %q", p.URL, got)
			}
		})
	}
}

func TestLocationPairFromWinPath_NonASCII(t *testing.T) {
	// Non-ASCII path (CJK). The 0x0D side keeps the raw UTF-8 string (encoded
	// UTF-16LE by T005's encoder downstream); the 0x0B URL percent-escapes every
	// non-unreserved byte.
	win := `W:\books\日本語\track.mp3`
	p, err := LocationPairFromWinPath(win)
	if err != nil {
		t.Fatalf("non-ASCII path should be accepted: %v", err)
	}
	if p.WinPath != win {
		t.Errorf("WinPath mangled: %q", p.WinPath)
	}
	if strings.Contains(p.URL, "日") {
		t.Errorf("URL must percent-escape non-ASCII bytes, got %q", p.URL)
	}
	if !strings.HasPrefix(p.URL, "file://localhost/W:/books/") {
		t.Errorf("URL prefix wrong: %q", p.URL)
	}
	// Round-trip must recover the exact bytes through the UTF-16LE encoder guards.
	back, err := LocationPairFromURL(p.URL)
	if err != nil {
		t.Fatalf("round-trip FromURL failed: %v", err)
	}
	if back.WinPath != win {
		t.Errorf("non-ASCII round-trip lost bytes: %q != %q", back.WinPath, win)
	}
}

func TestLocationPairFromWinPath_Rejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"relative path", `iTunes Media\track.mp3`},
		{"bare filename", "track.mp3"},
		{"empty", ""},
		{"whitespace only", "   "},
		{"unc path", `\\server\share\track.mp3`},
		{"staging dir leak", `W:\audiobook-organizer\.itunes-writeback\iTunes Media\track.mp3`},
		{"url shaped into winpath ctor", "file://localhost/W:/track.mp3"},
		{"http url", "https://feeds.example.com/podcast.mp3"},
		{"no drive colon", `W\itunes\track.mp3`},
		{"forward slashes", "W:/itunes/track.mp3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LocationPairFromWinPath(tc.in); err == nil {
				t.Errorf("expected rejection for %q, got nil error", tc.in)
			}
		})
	}
}

func TestLocationPairFromURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantWin string
	}{
		{
			name:    "canonical localhost form",
			url:     "file://localhost/W:/itunes/iTunes%20Media/track.mp3",
			wantWin: `W:\itunes\iTunes Media\track.mp3`,
		},
		{
			name:    "xml export file:// host-less form",
			url:     "file://W:/itunes/track.mp3",
			wantWin: `W:\itunes\track.mp3`,
		},
		{
			name:    "triple-slash form",
			url:     "file:///W:/Music/a%2Bb.mp3",
			wantWin: `W:\Music\a+b.mp3`,
		},
		{
			name:    "escaped percent",
			url:     "file://localhost/W:/books/50%25%20Off.mp3",
			wantWin: `W:\books\50% Off.mp3`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := LocationPairFromURL(tc.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.WinPath != tc.wantWin {
				t.Errorf("WinPath = %q, want %q", p.WinPath, tc.wantWin)
			}
		})
	}
}

func TestLocationPairFromURL_RejectsPodcast(t *testing.T) {
	// Podcast/stream tracks carry http(s) in 0x0B and have NO 0x0D — they must
	// never be mapped to a Windows path (SPEC §1b rule 3).
	for _, u := range []string{
		"https://feeds.example.com/podcast.mp3",
		"http://stream.example.com/live",
	} {
		if _, err := LocationPairFromURL(u); err == nil {
			t.Errorf("expected rejection of stream URL %q", u)
		}
	}
}

// TestLocationPair_RoundTripProperty is the normative bidirectional property
// (SPEC §1b rule 2): FromWinPath(p).URL → FromURL(url).WinPath == p.
func TestLocationPair_RoundTripProperty(t *testing.T) {
	paths := []string{
		`W:\itunes\iTunes Media\Audiobooks\Adrian Tchaikovsky\01 Children of Time - 1.mp3`,
		`C:\Music\track.m4b`,
		`W:\books\50% Off.mp3`,
		`D:\a\b c\d-e_f.mp3`,
		`W:\books\日本語\track.mp3`,
		`E:\émigré\café.mp3`,
		`W:\folder\file with  double  spaces.mp3`,
		`Z:\#hashtag\&ampersand\=equals.mp3`,
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			fwd, err := LocationPairFromWinPath(p)
			if err != nil {
				t.Fatalf("FromWinPath(%q): %v", p, err)
			}
			back, err := LocationPairFromURL(fwd.URL)
			if err != nil {
				t.Fatalf("FromURL(%q): %v", fwd.URL, err)
			}
			if back.WinPath != p {
				t.Errorf("round-trip mismatch: FromURL(FromWinPath(%q).URL).WinPath = %q", p, back.WinPath)
			}
			if back.URL != fwd.URL {
				t.Errorf("round-trip URL drift: %q != %q", back.URL, fwd.URL)
			}
		})
	}
}

func TestNewLocationPair_DetectsForm(t *testing.T) {
	// Native path form.
	if p, err := NewLocationPair(`W:\itunes\track.mp3`); err != nil || p.WinPath != `W:\itunes\track.mp3` {
		t.Fatalf("native path: pair=%+v err=%v", p, err)
	}
	// URL form gets routed to FromURL and normalized to a WinPath.
	if p, err := NewLocationPair("file://localhost/W:/itunes/My%20Track.mp3"); err != nil || p.WinPath != `W:\itunes\My Track.mp3` {
		t.Fatalf("url form: pair=%+v err=%v", p, err)
	}
	// Staging leak rejected regardless of form.
	if _, err := NewLocationPair("file://localhost/W:/audiobook-organizer/.itunes-writeback/iTunes%20Media/x.mp3"); err == nil {
		t.Error("staging-leak URL should be rejected")
	}
}
