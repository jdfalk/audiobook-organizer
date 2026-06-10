// file: internal/itunes/location_pair.go
// version: 1.0.0
// guid: 4d8e1a37-2c9b-4f06-8e51-6a0d7c3b29f4

// LocationPair — the single source of truth for the two derived renderings of a
// track location (fable5 TASK-006, CRIT-2).
//
// WHY this type exists (SPEC §1b, census-verified against itunes-lib-good.itl,
// SHA-256-identical to golden — 93,014 local tracks + 1,187 podcasts, zero
// counter-examples in 187,215 censused location blocks):
//
// A track's location is ONE Windows absolute path. iTunes renders it into the
// .itl TWICE, into two different hohm types:
//
//	0x0D Location  — the PLAIN Windows path. Backslashes. NO "file://". NO
//	                 percent-escaping.        e.g. W:\itunes\...\01 Foo - 1.mp3
//	0x0B LocalURL  — "file://localhost/" + the SAME path with '\'→'/' and
//	                 RFC-3986 percent-escaping.
//	                 e.g. file://localhost/W:/itunes/...%2001%20Foo%20-%201.mp3
//
// The historical CRIT-2 bug (in one sentence): callers passed URL-shaped
// f.ITunesPath and the writer copied it into 0x0D VERBATIM, so both fields held
// the URL form (83,783 blocks in damaged-1/3; 34 in damaged-4, some pointing into
// our ".itunes-writeback/" staging dir) — which iTunes rejects as "(Damaged)".
//
// LocationPair makes that bug unrepresentable: no caller passes a raw string to
// either field; both fields are DERIVED from one validated LocationPair, and
// LocationPairFromWinPath(p).URL round-trips to LocationPairFromURL(url).WinPath.
//
// Pairing rules (SPEC §1b):
//  1. Never write a URL into 0x0D; never write a bare path into 0x0B.
//  2. Every local-file track has BOTH fields and they round-trip.
//  3. Podcast/stream tracks have NO 0x0D and carry http(s):// in 0x0B — those are
//     handled at the call site (they are never location-updated into a 0x0D), not
//     by this type, which models the local-file path pair only.
//
// Encoding (UTF-16LE for non-ASCII paths) is orthogonal and handled by T005's
// encodeMhohITunes — LocationPair carries the correct *string*; the encoder
// stamps the correct bytes. Both must land for writeback to be safe (SPEC §1b
// rule 4).

package itunes

import (
	"fmt"
	"strings"
)

// localURLPrefix is the exact 0x0B prefix iTunes writes for local-file tracks.
// Census: golden 0x0B = 94,201 blocks, 93,014 with this exact prefix.
const localURLPrefix = "file://localhost/"

// LocationPair is the validated single source of truth for a local-file track
// location. WinPath is the native Windows path written verbatim into hohm 0x0D;
// URL is the percent-escaped file://localhost/ rendering written into hohm 0x0B.
// The two are always mutually consistent: building from one derives the other.
type LocationPair struct {
	WinPath string // hohm 0x0D — plain Windows path, backslashes, no escaping
	URL     string // hohm 0x0B — file://localhost/ + slash-flipped, RFC-3986 escaped
}

// LocationPairFromWinPath builds a LocationPair from a native Windows absolute
// path (the canonical source-of-truth form). It validates the drive-letter shape,
// rejects relative paths and staging-dir leaks, and derives the 0x0B URL with the
// SAME RFC-3986 escaping the T003 location-form guard expects (winPathToLocalURL),
// so guard round-trip is byte-identical.
//
// Rejected (returns a non-nil error, never a partial pair):
//   - not "<drive>:\..." (relative paths, UNC "\\server\share", URLs)
//   - any value containing ".itunes-writeback/" (staging-dir leak — damaged-4)
//   - already URL-shaped ("file://"/"http://"/"https://") — caller passed the
//     wrong field; use LocationPairFromURL instead.
func LocationPairFromWinPath(winPath string) (LocationPair, error) {
	p := strings.TrimSpace(winPath)
	if p == "" {
		return LocationPair{}, fmt.Errorf("location: empty Windows path")
	}
	if lp := strings.ToLower(p); strings.HasPrefix(lp, "file://") ||
		strings.HasPrefix(lp, "http://") || strings.HasPrefix(lp, "https://") {
		return LocationPair{}, fmt.Errorf("location: %q is URL-shaped, expected a native Windows path (CRIT-2)", truncStr(p))
	}
	if strings.Contains(p, ".itunes-writeback/") || strings.Contains(p, ".itunes-writeback\\") {
		return LocationPair{}, fmt.Errorf("location: %q points into the .itunes-writeback/ staging dir (CRIT-2)", truncStr(p))
	}
	// UNC paths ("\\server\share\...") have no drive letter and cannot be rendered
	// into the "<drive>:/" 0x0B form iTunes uses; reject rather than mis-encode.
	if strings.HasPrefix(p, "\\\\") || strings.HasPrefix(p, "//") {
		return LocationPair{}, fmt.Errorf("location: %q is a UNC path; only drive-lettered paths are supported (CRIT-2)", truncStr(p))
	}
	if !isWindowsAbsPath(p) {
		return LocationPair{}, fmt.Errorf("location: %q is not a Windows absolute path (expected '<drive>:\\...')", truncStr(p))
	}
	return LocationPair{WinPath: p, URL: winPathToLocalURL(p)}, nil
}

// LocationPairFromURL builds a LocationPair from a 0x0B-form file://localhost/ URL
// (the form that may already live, wrongly, in f.ITunesPath / a DB column). It
// percent-decodes, flips '/'→'\', and validates the result as a Windows path —
// then re-derives a canonical URL so the pair is internally consistent even if the
// input URL had non-canonical (but still valid) escaping.
//
// Rejected: http(s):// stream URLs (podcasts — those have no 0x0D and must not be
// turned into a local path), non-file URLs, and any decoded value that fails the
// WinPath validation (relative, staging leak, …).
func LocationPairFromURL(url string) (LocationPair, error) {
	u := strings.TrimSpace(url)
	if u == "" {
		return LocationPair{}, fmt.Errorf("location: empty URL")
	}
	lu := strings.ToLower(u)
	if strings.HasPrefix(lu, "http://") || strings.HasPrefix(lu, "https://") {
		return LocationPair{}, fmt.Errorf("location: %q is an http(s) stream URL (podcast); it has no 0x0D Location and must not be mapped to a Windows path", truncStr(u))
	}

	// Accept the canonical "file://localhost/" prefix and the looser "file://"
	// form seen in iTunes Library.xml exports (SPEC §1b rule 5).
	var rest string
	switch {
	case strings.HasPrefix(u, localURLPrefix):
		rest = u[len(localURLPrefix):]
	case strings.HasPrefix(lu, "file:///"):
		rest = u[len("file:///"):]
	case strings.HasPrefix(lu, "file://"):
		// "file://W:/..." — host-less form; strip the scheme only.
		rest = u[len("file://"):]
	default:
		return LocationPair{}, fmt.Errorf("location: %q is not a file:// URL", truncStr(u))
	}

	decoded, err := percentDecode(rest)
	if err != nil {
		return LocationPair{}, fmt.Errorf("location: cannot percent-decode %q: %w", truncStr(u), err)
	}
	winPath := strings.ReplaceAll(decoded, "/", "\\")

	// Validate + re-derive the canonical URL via FromWinPath so the returned pair
	// is byte-consistent with what the guard expects, regardless of the input's
	// escaping choices.
	return LocationPairFromWinPath(winPath)
}

// normalizeLocationValue accepts a raw location value of EITHER form (a native
// Windows path or a file://localhost/ URL, as may sit in f.ITunesPath) and
// returns the canonical LocationPair. This is the single normalization entry
// point every writer call site uses: it detects the form and routes to the right
// constructor, so URL-shaped values that historically leaked into 0x0D are
// converted (never written raw) and unmappable values surface a per-item error.
//
// WHY detect-and-route here (CRIT-2): f.ITunesPath has held both forms across the
// library's history (URLs in 83,783 0x0D blocks). Forcing every site through one
// detector means the 0x0B/0x0D contract holds no matter which form the DB carries.
func normalizeLocationValue(raw string) (LocationPair, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return LocationPair{}, fmt.Errorf("location: empty value")
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "file://") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return LocationPairFromURL(s)
	}
	return LocationPairFromWinPath(s)
}

// NewLocationPair is the exported normalization entry point for callers outside
// this package (the writeback service). It accepts a raw location value of EITHER
// form (native Windows path or file://localhost/ URL) and returns the canonical
// LocationPair, or an error for unmappable values (relative path, staging-dir
// leak, podcast http(s) URL). Service call sites use this to convert f.ITunesPath
// before building an ITLLocationUpdate, so the 0x0B/0x0D contract holds regardless
// of which form the DB carries (CRIT-2 / SPEC §1b).
func NewLocationPair(raw string) (LocationPair, error) {
	return normalizeLocationValue(raw)
}

// percentDecode decodes RFC-3986 percent-escapes (%XX) in s. Unlike net/url it
// does NOT treat '+' as a space (file URLs never use form encoding) and leaves
// every non-escaped byte untouched, so it is the exact inverse of the guard's
// winPathToLocalURL escaping.
func percentDecode(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		if i+2 >= len(s) {
			return "", fmt.Errorf("truncated percent-escape at offset %d", i)
		}
		hi, ok1 := fromHexDigit(s[i+1])
		lo, ok2 := fromHexDigit(s[i+2])
		if !ok1 || !ok2 {
			return "", fmt.Errorf("invalid percent-escape %q at offset %d", s[i:i+3], i)
		}
		b.WriteByte(hi<<4 | lo)
		i += 2
	}
	return b.String(), nil
}

func fromHexDigit(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
