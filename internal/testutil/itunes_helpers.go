// file: internal/testutil/itunes_helpers.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-234567890abc

package testutil

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	"github.com/stretchr/testify/require"
)

// ITunesTestTrack defines a track for generating test iTunes XML.
type ITunesTestTrack struct {
	TrackID      int
	PersistentID string
	Name         string
	Artist       string
	AlbumArtist  string
	Album        string
	Genre        string
	Kind         string
	Year         int
	FilePath     string
	TotalTime    int // milliseconds
	Comments     string
}

// GenerateITunesXML creates a synthetic iTunes Library XML pointing to real files.
func GenerateITunesXML(t *testing.T, tracks []ITunesTestTrack, outputPath string) {
	t.Helper()
	var sb strings.Builder
	sb.WriteString(itunesXMLHeader)
	for _, track := range tracks {
		sb.WriteString(fmt.Sprintf(itunesTrackTemplate,
			track.TrackID, track.TrackID, track.PersistentID,
			track.Name, track.Artist, track.AlbumArtist, track.Album,
			track.Genre, track.Kind, track.Year,
			itunes.EncodeLocation(track.FilePath),
			track.TotalTime, track.Comments,
		))
	}
	sb.WriteString(itunesXMLFooter)
	require.NoError(t, os.WriteFile(outputPath, []byte(sb.String()), 0644))
}

const itunesXMLHeader = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Major Version</key><integer>1</integer>
	<key>Minor Version</key><integer>1</integer>
	<key>Application Version</key><string>12.0</string>
	<key>Music Folder</key><string>file:///tmp/test-music/</string>
	<key>Tracks</key>
	<dict>
`

// Template args: TrackID, TrackID, PersistentID, Name, Artist, AlbumArtist, Album, Genre, Kind, Year, Location, TotalTime, Comments
const itunesTrackTemplate = `		<key>%d</key>
		<dict>
			<key>Track ID</key><integer>%d</integer>
			<key>Persistent ID</key><string>%s</string>
			<key>Name</key><string>%s</string>
			<key>Artist</key><string>%s</string>
			<key>Album Artist</key><string>%s</string>
			<key>Album</key><string>%s</string>
			<key>Genre</key><string>%s</string>
			<key>Kind</key><string>%s</string>
			<key>Year</key><integer>%d</integer>
			<key>Location</key><string>%s</string>
			<key>Total Time</key><integer>%d</integer>
			<key>Comments</key><string>%s</string>
		</dict>
`

const itunesXMLFooter = `	</dict>
	<key>Playlists</key>
	<array>
	</array>
</dict>
</plist>
`
