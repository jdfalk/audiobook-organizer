package itunes

import (
	"fmt"
	"os"
	"time"

	"howett.net/plist"
)

// plistLibrary represents the raw plist structure from iTunes
type plistLibrary struct {
	MajorVersion       int                       `plist:"Major Version"`
	MinorVersion       int                       `plist:"Minor Version"`
	ApplicationVersion string                    `plist:"Application Version"`
	MusicFolder        string                    `plist:"Music Folder"`
	Tracks             map[string]*plistTrack    `plist:"Tracks"`
	Playlists          []*plistPlaylist          `plist:"Playlists"`
}

// plistTrack represents a single track in the plist format
type plistTrack struct {
	TrackID       int       `plist:"Track ID"`
	PersistentID  string    `plist:"Persistent ID"`
	Name          string    `plist:"Name"`
	Artist        string    `plist:"Artist"`
	AlbumArtist   string    `plist:"Album Artist"`
	Album         string    `plist:"Album"`
	Genre         string    `plist:"Genre"`
	Kind          string    `plist:"Kind"`
	Year          int       `plist:"Year"`
	Comments      string    `plist:"Comments"`
	Location      string    `plist:"Location"`
	Size          int64     `plist:"Size"`
	TotalTime     int64     `plist:"Total Time"`     // milliseconds
	DateAdded     time.Time `plist:"Date Added"`
	PlayCount     int       `plist:"Play Count"`
	PlayDate      int64     `plist:"Play Date"`      // Unix timestamp
	PlayDateUTC   time.Time `plist:"Play Date UTC"`
	Rating        int       `plist:"Rating"`         // 0-100 scale
	Bookmark      int64     `plist:"Bookmark"`       // milliseconds
	Bookmarkable  bool      `plist:"Bookmarkable"`
}

// plistPlaylist represents a playlist in the plist format
type plistPlaylist struct {
	Name          string                   `plist:"Name"`
	PlaylistID    int                      `plist:"Playlist ID"`
	PlaylistItems []*plistPlaylistItem     `plist:"Playlist Items"`
}

// plistPlaylistItem represents a track reference in a playlist
type plistPlaylistItem struct {
	TrackID int `plist:"Track ID"`
}

// parsePlist parses an iTunes plist format XML file
func parsePlist(data []byte) (*Library, error) {
	var raw plistLibrary

	// Decode the plist XML
	_, err := plist.Unmarshal(data, &raw)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal plist: %w", err)
	}

	// Convert to our internal Library structure
	library := &Library{
		MajorVersion:       raw.MajorVersion,
		MinorVersion:       raw.MinorVersion,
		ApplicationVersion: raw.ApplicationVersion,
		MusicFolder:        raw.MusicFolder,
		Tracks:             make(map[string]*Track),
		Playlists:          make([]*Playlist, 0, len(raw.Playlists)),
	}

	// Convert tracks
	for id, rawTrack := range raw.Tracks {
		// Guard against int64 overflow: iTunes occasionally writes file sizes
		// as unsigned 64-bit integers. Values > math.MaxInt64 wrap negative
		// when parsed into int64; reset to 0 so the os.Stat fallback in
		// buildBookFromTrack populates the correct value.
		size := rawTrack.Size
		if size < 0 {
			size = 0
		}

		library.Tracks[id] = &Track{
			TrackID:      rawTrack.TrackID,
			PersistentID: rawTrack.PersistentID,
			Name:         rawTrack.Name,
			Artist:       rawTrack.Artist,
			AlbumArtist:  rawTrack.AlbumArtist,
			Album:        rawTrack.Album,
			Genre:        rawTrack.Genre,
			Kind:         rawTrack.Kind,
			Year:         rawTrack.Year,
			Comments:     rawTrack.Comments,
			Location:     rawTrack.Location,
			Size:         size,
			TotalTime:    rawTrack.TotalTime,
			DateAdded:    rawTrack.DateAdded,
			PlayCount:    rawTrack.PlayCount,
			PlayDate:     rawTrack.PlayDate,
			Rating:       rawTrack.Rating,
			Bookmark:     rawTrack.Bookmark,
			Bookmarkable: rawTrack.Bookmarkable,
		}
	}

	// Convert playlists
	for _, rawPlaylist := range raw.Playlists {
		if rawPlaylist.PlaylistItems == nil {
			continue
		}

		trackIDs := make([]int, 0, len(rawPlaylist.PlaylistItems))
		for _, item := range rawPlaylist.PlaylistItems {
			trackIDs = append(trackIDs, item.TrackID)
		}

		library.Playlists = append(library.Playlists, &Playlist{
			PlaylistID: rawPlaylist.PlaylistID,
			Name:       rawPlaylist.Name,
			TrackIDs:   trackIDs,
		})
	}

	return library, nil
}

// writePlist writes a Library structure to an iTunes plist XML file
func writePlist(library *Library, path string) error {
	// Convert from our internal structure to plist structure
	raw := &plistLibrary{
		MajorVersion:       library.MajorVersion,
		MinorVersion:       library.MinorVersion,
		ApplicationVersion: library.ApplicationVersion,
		MusicFolder:        library.MusicFolder,
		Tracks:             make(map[string]*plistTrack),
		Playlists:          make([]*plistPlaylist, 0, len(library.Playlists)),
	}

	// Convert tracks
	for id, track := range library.Tracks {
		raw.Tracks[id] = &plistTrack{
			TrackID:      track.TrackID,
			PersistentID: track.PersistentID,
			Name:         track.Name,
			Artist:       track.Artist,
			AlbumArtist:  track.AlbumArtist,
			Album:        track.Album,
			Genre:        track.Genre,
			Kind:         track.Kind,
			Year:         track.Year,
			Comments:     track.Comments,
			Location:     track.Location,
			Size:         track.Size,
			TotalTime:    track.TotalTime,
			DateAdded:    track.DateAdded,
			PlayCount:    track.PlayCount,
			PlayDate:     track.PlayDate,
			Rating:       track.Rating,
			Bookmark:     track.Bookmark,
			Bookmarkable: track.Bookmarkable,
		}
	}

	// Convert playlists
	for _, playlist := range library.Playlists {
		items := make([]*plistPlaylistItem, 0, len(playlist.TrackIDs))
		for _, trackID := range playlist.TrackIDs {
			items = append(items, &plistPlaylistItem{TrackID: trackID})
		}

		raw.Playlists = append(raw.Playlists, &plistPlaylist{
			Name:          playlist.Name,
			PlaylistID:    playlist.PlaylistID,
			PlaylistItems: items,
		})
	}

	// Create a temporary file
	tempFile := path + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		file.Close()
		os.Remove(tempFile) // Clean up temp file on error
	}()

	// Encode to plist XML format
	encoder := plist.NewEncoder(file)
	encoder.Indent("\t") // Use tabs for indentation like iTunes
	if err := encoder.Encode(raw); err != nil {
		return fmt.Errorf("failed to encode plist: %w", err)
	}

	// Close file before rename
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
