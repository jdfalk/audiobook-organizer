// file: internal/acoustid/client.go
// version: 1.0.0
// guid: 5d6e7f80-9a1b-2c3d-4e5f-607182931a2b
// last-edited: 2026-05-31

// Package acoustid is a thin client for the acoustid.org /v2/lookup API.
// We only need the smallest slice of the response — top-scoring
// MusicBrainz recording ID + score — so this is intentionally not a
// generated SDK.
//
// API docs: https://acoustid.org/webservice
//
// Rate limits: the free tier permits 3 requests/second per API key. The
// caller (acoustid.lookup-online op) is responsible for spacing requests
// — this client does not throttle internally.
package acoustid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrNoAPIKey is returned by Lookup when the env var ACOUSTID_API_KEY is unset.
var ErrNoAPIKey = errors.New("acoustid: ACOUSTID_API_KEY is not set")

// LookupResult is the minimal subset of /v2/lookup's response we care about.
type LookupResult struct {
	// RecordingID is the top-scoring MusicBrainz recording ID, or "" when
	// the API has no match.
	RecordingID string
	// Score is the top match's similarity score (0..1).
	Score float64
	// Status is the API's top-level status field (e.g. "ok", "error").
	Status string
	// Raw is the unmodified JSON response, for diagnostic logging. Empty
	// when the request failed before the response body arrived.
	Raw []byte
}

// Client wraps an *http.Client with the AcoustID API base URL + key.
type Client struct {
	HTTP    *http.Client
	BaseURL string // default "https://api.acoustid.org/v2/lookup"
	APIKey  string // from env ACOUSTID_API_KEY
}

// NewClient constructs a Client that reads the API key from env. The
// caller checks the returned key emptiness via ErrNoAPIKey in Lookup.
func NewClient(apiKey string) *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 15 * time.Second},
		BaseURL: "https://api.acoustid.org/v2/lookup",
		APIKey:  apiKey,
	}
}

// Lookup runs a single /v2/lookup against acoustid.org. The fingerprint
// must be the canonical chromaprint base64 form (with the 4-byte v1
// header) — internal/fingerprint.EncodeWholeFingerprint produces this
// from the raw fp bytes stored on BookFile.AcoustIDFingerprint.
//
// duration is the file's measured duration in seconds (integer); the API
// uses it to disambiguate fingerprints that overlap on short clips.
//
// Returns the top result and the raw JSON response. An API-level error
// (status != "ok") is returned as a non-nil error with the API's error
// message wrapped.
func (c *Client) Lookup(ctx context.Context, fingerprint string, durationSec int) (LookupResult, error) {
	if c.APIKey == "" {
		return LookupResult{}, ErrNoAPIKey
	}

	form := url.Values{}
	form.Set("client", c.APIKey)
	form.Set("duration", strconv.Itoa(durationSec))
	form.Set("fingerprint", fingerprint)
	form.Set("meta", "recordings")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return LookupResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return LookupResult{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return LookupResult{}, err
	}

	if resp.StatusCode/100 != 2 {
		return LookupResult{Raw: body}, fmt.Errorf("acoustid: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Status string `json:"status"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
		Results []struct {
			ID         string  `json:"id"`
			Score      float64 `json:"score"`
			Recordings []struct {
				ID string `json:"id"`
			} `json:"recordings,omitempty"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return LookupResult{Raw: body}, fmt.Errorf("acoustid: decode response: %w", err)
	}
	if parsed.Error != nil {
		return LookupResult{Raw: body, Status: parsed.Status},
			fmt.Errorf("acoustid: API error %d: %s", parsed.Error.Code, parsed.Error.Message)
	}

	out := LookupResult{Status: parsed.Status, Raw: body}
	for _, r := range parsed.Results {
		if r.Score <= out.Score {
			continue
		}
		out.Score = r.Score
		// Pick the first recording from the top result. Files with multiple
		// recording matches (e.g. an audiobook chapter pulled from a
		// re-issue) all point at the same MusicBrainz work via this id;
		// the caller can fetch siblings later via the MB API if needed.
		if len(r.Recordings) > 0 {
			out.RecordingID = r.Recordings[0].ID
		} else {
			// AcoustID's `id` field is the AcoustID UUID, not a MB id, but
			// keep it as a fallback so the caller has *something* to log.
			out.RecordingID = r.ID
		}
	}
	return out, nil
}
