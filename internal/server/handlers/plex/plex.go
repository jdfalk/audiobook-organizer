// file: internal/server/handlers/plex/plex.go
// minimal, Plex-style browsing/streaming facade for Audiobook Organizer
// Provides a tiny subset of endpoints to browse a single "Audiobooks" section,
// list items, fetch basic metadata, covers, and stream a file.
// NOTE: This is intentionally lightweight and "Plex-style" rather than a
// byte-for-byte Plex protocol clone. It exists to let generic media clients
// discover and play audiobooks via a familiar shape.

package plex

import (
	"encoding/xml"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// Store is the subset of database.Store required by the Plex handler.
// Narrow interface keeps coupling small and testable.
type Store interface {
	GetAllBooks(limit, offset int) ([]database.Book, error)
	GetBookByID(id string) (*database.Book, error)
	GetBookFiles(bookID string) ([]database.BookFile, error)
}

// Handler serves the Plex-style API.
type Handler struct {
	store      Store
	basePrefix string // mounted prefix (e.g. "/api/v1/plex") for self links
}

// New constructs a Handler.
func New(store Store, basePrefix string) *Handler {
	return &Handler{store: store, basePrefix: basePrefix}
}

// MediaContainer is the root XML element used by Plex servers/clients.
type MediaContainer struct {
	XMLName           xml.Name    `xml:"MediaContainer"`
	Size              int         `xml:"size,attr,omitempty"`
	FriendlyName      string      `xml:"friendlyName,attr,omitempty"`
	MachineIdentifier string      `xml:"machineIdentifier,attr,omitempty"`
	Version           string      `xml:"version,attr,omitempty"`
	Directory         []Directory `xml:"Directory,omitempty"`
	Metadata          []Metadata  `xml:"Metadata,omitempty"`
}

type Directory struct {
	Key              string `xml:"key,attr"`
	Title            string `xml:"title,attr"`
	Type             string `xml:"type,attr,omitempty"`
	LibrarySectionID string `xml:"librarySectionID,attr,omitempty"`
}

type Metadata struct {
	RatingKey string `xml:"ratingKey,attr"`
	Key       string `xml:"key,attr"`
	Type      string `xml:"type,attr"`
	Title     string `xml:"title,attr"`
	Summary   string `xml:"summary,attr,omitempty"`
	Thumb     string `xml:"thumb,attr,omitempty"`
	Year      int    `xml:"year,attr,omitempty"`
	AddedAt   int64  `xml:"addedAt,attr,omitempty"`
}

// Identity returns a minimal identity document.
func (h *Handler) Identity(c *gin.Context) {
	mc := MediaContainer{
		FriendlyName:      "Audiobook Organizer",
		MachineIdentifier: "audiobook-organizer",
		Version:           "1",
	}
	c.XML(http.StatusOK, mc)
}

// ListSections exposes a single logical section: "Audiobooks" with ID 1.
func (h *Handler) ListSections(c *gin.Context) {
	mc := MediaContainer{
		Size: 1,
		Directory: []Directory{{
			Key:              filepath.ToSlash(filepath.Join(h.basePrefix, "/library/sections/1/all")),
			Title:            "Audiobooks",
			Type:             "audio",
			LibrarySectionID: "1",
		}},
	}
	c.XML(http.StatusOK, mc)
}

// ListSectionAll returns all audiobooks as Metadata entries.
func (h *Handler) ListSectionAll(c *gin.Context) {
	books, err := h.store.GetAllBooks(1_000_000, 0)
	if err != nil {
		c.XML(http.StatusInternalServerError, MediaContainer{})
		return
	}
	out := make([]Metadata, 0, len(books))
	for i := range books {
		b := &books[i]
		out = append(out, h.toMetadata(b))
	}
	mc := MediaContainer{Size: len(out), Metadata: out}
	c.XML(http.StatusOK, mc)
}

// GetMetadata returns a single item's Metadata record.
func (h *Handler) GetMetadata(c *gin.Context) {
	id := c.Param("id")
	b, err := h.store.GetBookByID(id)
	if err != nil || b == nil {
		c.XML(http.StatusNotFound, MediaContainer{})
		return
	}
	mc := MediaContainer{Size: 1, Metadata: []Metadata{h.toMetadata(b)}}
	c.XML(http.StatusOK, mc)
}

// GetThumb redirects to the app's cover endpoint for the book.
func (h *Handler) GetThumb(c *gin.Context) {
	id := c.Param("id")
	// Reuse the existing cover route. Keep it behind the same auth session.
	c.Redirect(http.StatusTemporaryRedirect, filepath.ToSlash(filepath.Join("/api/v1/audiobooks/", id, "cover")))
}

// StreamFile streams the primary audio file for a book, supporting range requests.
func (h *Handler) StreamFile(c *gin.Context) {
	id := c.Param("id")
	b, err := h.store.GetBookByID(id)
	if err != nil || b == nil {
		c.Status(http.StatusNotFound)
		return
	}
	path := h.chooseFilePath(b)
	if path == "" {
		c.Status(http.StatusNotFound)
		return
	}
	// Let net/http handle Range and Content-Type
	http.ServeFile(c.Writer, c.Request, path)
}

func (h *Handler) chooseFilePath(b *database.Book) string {
	if fileExists(b.FilePath) {
		return b.FilePath
	}
	if b.ID == "" {
		return ""
	}
	files, err := h.store.GetBookFiles(b.ID)
	if err != nil || len(files) == 0 {
		return ""
	}
	for _, f := range files {
		if fileExists(f.FilePath) {
			return f.FilePath
		}
	}
	return ""
}

func (h *Handler) toMetadata(b *database.Book) Metadata {
	m := Metadata{
		RatingKey: b.ID,
		Key:       filepath.ToSlash(filepath.Join(h.basePrefix, "/library/metadata/", b.ID)),
		Type:      "track",
		Title:     b.Title,
		Thumb:     filepath.ToSlash(filepath.Join(h.basePrefix, "/library/metadata/", b.ID, "/thumb")),
	}
	if b.Description != nil {
		m.Summary = *b.Description
	}
	// Approximate a year: prefer explicit years, else derive from CreatedAt
	if b.AudiobookReleaseYear != nil {
		m.Year = *b.AudiobookReleaseYear
	} else if b.PrintYear != nil {
		m.Year = *b.PrintYear
	} else if b.CreatedAt != nil {
		year := b.CreatedAt.Year()
		m.Year = year
	}
	if b.CreatedAt != nil {
		m.AddedAt = b.CreatedAt.Unix()
	} else {
		m.AddedAt = time.Now().Unix()
	}
	return m
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
