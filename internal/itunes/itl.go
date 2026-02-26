// file: internal/itunes/itl.go
// version: 1.2.0
// guid: 7f2a8b3c-4d5e-6f01-a2b3-c4d5e6f7a8b9

package itunes

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// itlAESKey is the hardcoded AES key used by iTunes for ITL encryption.
var itlAESKey = []byte("BHUILuilfghuila3")

// macEpoch is 1904-01-01 00:00:00 UTC, the Mac HFS+ epoch.
var macEpoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)

// ITLPlaylist represents a playlist from the ITL binary.
type ITLPlaylist struct {
	PersistentID  [8]byte // 8-byte playlist persistent ID (ppid)
	Title         string  // From hohm type 0x64
	IsFolder      bool    // True if this is a playlist folder (issue #11) — TODO: reliable detection
	IsSmart       bool    // Has smart criteria
	Items         []int   // Track IDs referenced by this playlist (hptm records)
	SmartInfo     []byte  // Raw smart info blob (hohm 0x66)
	SmartCriteria []byte  // Raw smart criteria blob (hohm 0x65)
}

// ITLLibrary represents a parsed iTunes .itl binary library.
type ITLLibrary struct {
	Version         string
	HeaderRemainder []byte
	Tracks          []ITLTrack
	Playlists       []ITLPlaylist
	UseCompression  bool
	rawData         []byte // decrypted (and decompressed) payload
	unknown         uint32 // from hdfm header
}

// ITLTrack represents a track from the ITL binary.
type ITLTrack struct {
	TrackID           int
	PersistentID      [8]byte
	AlbumPersistentID [8]byte
	Name              string
	Album             string
	Artist            string
	Genre             string
	Kind              string
	Location          string // hohm type 0x0D
	LocalURL          string // hohm type 0x0B
	Size              int
	TotalTime         int // milliseconds
	TrackNumber       int
	TrackCount        int
	DiscNumber        int
	DiscCount         int
	Year              int
	BitRate           int
	SampleRate        int
	PlayCount         int
	Rating            int // 0-100
	DateModified      time.Time
	DateAdded         time.Time
	LastPlayDate      time.Time
}

// ITLLocationUpdate maps a persistent ID to a new file location.
type ITLLocationUpdate struct {
	PersistentID string // hex-encoded 8-byte ID
	NewLocation  string
}

// ITLWriteBackResult contains results of updating an ITL file.
type ITLWriteBackResult struct {
	UpdatedCount int
	BackupPath   string
	OutputPath   string
}

// ITLNewTrack describes a track to insert into an ITL file.
type ITLNewTrack struct {
	Location    string
	Name        string
	Album       string
	Artist      string
	Genre       string
	Kind        string // e.g. "MPEG audio file", "AAC audio file"
	Size        int
	TotalTime   int // milliseconds
	TrackNumber int
	DiscNumber  int
	Year        int
	BitRate     int
	SampleRate  int
}

// ITLNewPlaylist describes a playlist to insert into an ITL file.
type ITLNewPlaylist struct {
	Title    string
	TrackIDs []int // Song IDs to include
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func readTag(data []byte, offset int) string {
	if offset+4 > len(data) {
		return ""
	}
	return string(data[offset : offset+4])
}

func readUint32BE(data []byte, offset int) uint32 {
	if offset+4 > len(data) {
		return 0
	}
	return binary.BigEndian.Uint32(data[offset : offset+4])
}

func readUint16BE(data []byte, offset int) uint16 {
	if offset+2 > len(data) {
		return 0
	}
	return binary.BigEndian.Uint16(data[offset : offset+2])
}

func writeUint32BE(buf []byte, offset int, val uint32) {
	if offset+4 <= len(buf) {
		binary.BigEndian.PutUint32(buf[offset:offset+4], val)
	}
}

func pidToHex(pid [8]byte) string {
	return hex.EncodeToString(pid[:])
}

func hexToPID(h string) ([8]byte, error) {
	var pid [8]byte
	b, err := hex.DecodeString(h)
	if err != nil {
		return pid, fmt.Errorf("invalid hex persistent ID: %w", err)
	}
	if len(b) != 8 {
		return pid, fmt.Errorf("persistent ID must be 8 bytes, got %d", len(b))
	}
	copy(pid[:], b)
	return pid, nil
}

func macDateToTime(seconds uint32) time.Time {
	if seconds == 0 {
		return time.Time{}
	}
	return macEpoch.Add(time.Duration(seconds) * time.Second)
}

func isVersionAtLeast(version string, major int) bool {
	if version == "" {
		return false
	}
	parts := strings.SplitN(version, ".", 2)
	v, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	return v >= major
}

// ---------------------------------------------------------------------------
// AES-128/ECB encrypt/decrypt
// ---------------------------------------------------------------------------

func itlDecrypt(version string, data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	block, err := aes.NewCipher(itlAESKey)
	if err != nil {
		return data
	}
	bs := block.BlockSize()

	limit := len(data)
	if isVersionAtLeast(version, 10) {
		if limit > 102400 {
			limit = 102400
		}
	}
	// Align to block size
	limit = (limit / bs) * bs

	out := make([]byte, len(data))
	copy(out, data)

	for i := 0; i < limit; i += bs {
		block.Decrypt(out[i:i+bs], data[i:i+bs])
	}
	return out
}

func itlEncrypt(version string, data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	block, err := aes.NewCipher(itlAESKey)
	if err != nil {
		return data
	}
	bs := block.BlockSize()

	limit := len(data)
	if isVersionAtLeast(version, 10) {
		if limit > 102400 {
			limit = 102400
		}
	}
	limit = (limit / bs) * bs

	out := make([]byte, len(data))
	copy(out, data)

	for i := 0; i < limit; i += bs {
		block.Encrypt(out[i:i+bs], data[i:i+bs])
	}
	return out
}

// ---------------------------------------------------------------------------
// Zlib compression
// ---------------------------------------------------------------------------

func itlInflate(data []byte) ([]byte, bool) {
	if len(data) == 0 || data[0] != 0x78 {
		return data, false
	}
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return data, false
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return data, false
	}
	return out, true
}

func itlDeflate(data []byte) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// String encoding/decoding for hohm records
// ---------------------------------------------------------------------------

// decodeHohmString decodes a string from hohm payload data.
// encodingFlag: 0=ASCII, 1=UTF-16BE, 2=UTF-8, 3=Windows-1252
func decodeHohmString(data []byte, encodingFlag byte) (string, error) {
	switch encodingFlag {
	case 0: // ASCII
		return string(data), nil
	case 1: // UTF-16BE
		if len(data)%2 != 0 {
			data = append(data, 0)
		}
		runes := make([]rune, len(data)/2)
		for i := 0; i < len(data)/2; i++ {
			runes[i] = rune(binary.BigEndian.Uint16(data[i*2 : i*2+2]))
		}
		return string(runes), nil
	case 2: // UTF-8
		return string(data), nil
	case 3: // Windows-1252
		dec := charmap.Windows1252.NewDecoder()
		out, err := dec.Bytes(data)
		if err != nil {
			return string(data), err
		}
		return string(out), nil
	default:
		return string(data), fmt.Errorf("unknown hohm encoding flag: %d", encodingFlag)
	}
}

// encodeHohmString encodes a string for writing into a hohm record.
// Returns (encoded bytes, encoding flag).
// If all runes <= 0xFF, uses Windows-1252 (flag 3). Otherwise UTF-16BE (flag 1).
func encodeHohmString(s string) ([]byte, byte) {
	allLatin := true
	for _, r := range s {
		if r > 0xFF {
			allLatin = false
			break
		}
	}

	if allLatin {
		enc := charmap.Windows1252.NewEncoder()
		out, err := enc.Bytes([]byte(s))
		if err != nil {
			// Fallback to UTF-16BE
			return encodeUTF16BE(s), 1
		}
		return out, 3
	}

	return encodeUTF16BE(s), 1
}

func encodeUTF16BE(s string) []byte {
	runes := []rune(s)
	buf := make([]byte, len(runes)*2)
	for i, r := range runes {
		binary.BigEndian.PutUint16(buf[i*2:i*2+2], uint16(r))
	}
	return buf
}

// ---------------------------------------------------------------------------
// hdfm header parsing
// ---------------------------------------------------------------------------

type hdfmHeader struct {
	headerLen       uint32
	fileLen         uint32
	unknown         uint32
	version         string
	headerRemainder []byte
}

func parseHdfmHeader(data []byte) (*hdfmHeader, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("file too small for ITL header")
	}
	tag := readTag(data, 0)
	if tag != "hdfm" {
		return nil, fmt.Errorf("not an ITL file: expected 'hdfm', got %q", tag)
	}
	headerLen := readUint32BE(data, 4)
	if int(headerLen) > len(data) {
		return nil, fmt.Errorf("hdfm header length %d exceeds file size %d", headerLen, len(data))
	}
	if headerLen < 16 {
		return nil, fmt.Errorf("hdfm header too short: %d", headerLen)
	}

	fileLen := readUint32BE(data, 8)
	unknown := readUint32BE(data, 12)

	// Version string length is a single byte (per Java: readUnsignedByte())
	off := 16
	if off+1 > int(headerLen) {
		return nil, fmt.Errorf("hdfm header truncated at version length")
	}
	verLen := int(data[off])
	off++
	if off+verLen > int(headerLen) {
		return nil, fmt.Errorf("hdfm version string exceeds header")
	}
	version := string(data[off : off+verLen])
	off += verLen

	var remainder []byte
	if off < int(headerLen) {
		remainder = make([]byte, int(headerLen)-off)
		copy(remainder, data[off:int(headerLen)])
	}

	return &hdfmHeader{
		headerLen:       headerLen,
		fileLen:         fileLen,
		unknown:         unknown,
		version:         version,
		headerRemainder: remainder,
	}, nil
}

func buildHdfmHeader(version string, remainder []byte, fileLen uint32, unknown uint32) []byte {
	verBytes := []byte(version)
	// Header: "hdfm"(4) + headerLen(4) + fileLen(4) + unknown(4) + verLen(1) + version(N) + remainder
	headerLen := 17 + len(verBytes) + len(remainder)
	buf := make([]byte, headerLen)
	copy(buf[0:4], "hdfm")
	writeUint32BE(buf, 4, uint32(headerLen))
	writeUint32BE(buf, 8, fileLen)
	writeUint32BE(buf, 12, unknown)
	buf[16] = byte(len(verBytes))
	copy(buf[17:17+len(verBytes)], verBytes)
	if len(remainder) > 0 {
		copy(buf[17+len(verBytes):], remainder)
	}
	return buf
}

// ---------------------------------------------------------------------------
// Chunk walking for read path
// ---------------------------------------------------------------------------

// ParseITL reads and parses an iTunes .itl binary library file.
func ParseITL(path string) (*ITLLibrary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading ITL file: %w", err)
	}
	return parseITLData(data)
}

func parseITLData(data []byte) (*ITLLibrary, error) {
	hdr, err := parseHdfmHeader(data)
	if err != nil {
		return nil, err
	}

	// Decrypt payload (everything after hdfm header)
	payload := data[hdr.headerLen:]
	decrypted := itlDecrypt(hdr.version, payload)

	// Decompress
	decompressed, wasCompressed := itlInflate(decrypted)

	// Check endianness
	littleEndian := false
	if len(decompressed) >= 4 && readTag(decompressed, 0) == "msdh" {
		littleEndian = true
	}
	_ = littleEndian // We don't fully handle LE yet, but note it

	lib := &ITLLibrary{
		Version:         hdr.version,
		HeaderRemainder: hdr.headerRemainder,
		UseCompression:  wasCompressed,
		rawData:         decompressed,
		unknown:         hdr.unknown,
	}

	// Walk chunks
	walkChunks(decompressed, lib)

	return lib, nil
}

func walkChunks(data []byte, lib *ITLLibrary) {
	offset := 0
	var currentTrack *ITLTrack
	var currentPlaylist *ITLPlaylist

	for offset+8 <= len(data) {
		tag := readTag(data, offset)
		if tag == "" {
			break
		}
		length := int(readUint32BE(data, offset+4))
		if length < 8 || offset+length > len(data) {
			break
		}

		switch tag {
		case "hdsm":
			// hdsm: extended length at offset+8 per PR #36
			extLen := int(readUint32BE(data, offset+8))
			// The hdsm contains sub-chunks; we process them inside
			// For parsing, we walk into hdsm's sub-content
			subStart := offset + 12
			if extLen > length && offset+extLen <= len(data) {
				// Extra data between length and extLen
				walkHdsmContent(data, subStart, offset+extLen, lib, &currentTrack, &currentPlaylist)
				offset += extLen
			} else {
				walkHdsmContent(data, subStart, offset+length, lib, &currentTrack, &currentPlaylist)
				offset += length
			}
			continue

		case "htim":
			// htim: track record
			currentPlaylist = nil
			t := parseHtim(data, offset, length)
			lib.Tracks = append(lib.Tracks, t)
			currentTrack = &lib.Tracks[len(lib.Tracks)-1]

		case "hpim":
			// hpim: playlist record
			currentTrack = nil
			p := parseHpim(data, offset, length)
			lib.Playlists = append(lib.Playlists, p)
			currentPlaylist = &lib.Playlists[len(lib.Playlists)-1]

		case "hptm":
			// hptm: playlist item
			if currentPlaylist != nil {
				trackID := parseHptm(data, offset, length)
				if trackID >= 0 {
					currentPlaylist.Items = append(currentPlaylist.Items, trackID)
				}
			}
			// TODO: extract checked state from hptm

		case "hohm":
			if currentTrack != nil {
				parseHohm(data, offset, length, currentTrack)
			} else if currentPlaylist != nil {
				parsePlaylistHohm(data, offset, length, currentPlaylist)
			}
		}

		offset += length
	}
}

func walkHdsmContent(data []byte, start, end int, lib *ITLLibrary, currentTrack **ITLTrack, currentPlaylist **ITLPlaylist) {
	offset := start
	for offset+8 <= end {
		tag := readTag(data, offset)
		if tag == "" {
			break
		}
		length := int(readUint32BE(data, offset+4))
		if length < 8 || offset+length > end {
			break
		}

		switch tag {
		case "htim":
			*currentPlaylist = nil
			t := parseHtim(data, offset, length)
			lib.Tracks = append(lib.Tracks, t)
			*currentTrack = &lib.Tracks[len(lib.Tracks)-1]

		case "hpim":
			*currentTrack = nil
			p := parseHpim(data, offset, length)
			lib.Playlists = append(lib.Playlists, p)
			*currentPlaylist = &lib.Playlists[len(lib.Playlists)-1]

		case "hptm":
			if *currentPlaylist != nil {
				trackID := parseHptm(data, offset, length)
				if trackID >= 0 {
					(*currentPlaylist).Items = append((*currentPlaylist).Items, trackID)
				}
			}

		case "hohm":
			if *currentTrack != nil {
				parseHohm(data, offset, length, *currentTrack)
			} else if *currentPlaylist != nil {
				parsePlaylistHohm(data, offset, length, *currentPlaylist)
			}
		}
		offset += length
	}
}

func parseHtim(data []byte, offset, length int) ITLTrack {
	t := ITLTrack{}
	// htim layout from Java titl ParseLibrary.readHtim():
	// +0:  "htim" tag (4)
	// +4:  length (4) — header length
	// +8:  recordLength (4) — total record length including sub-blocks
	// +12: subblocks count (4)
	// +16: song ID (4)
	// +20: block type (4)
	// +24: unknown (4)
	// +28: Mac OS file type (4)
	// +32: modification date (4)
	// +36: file size (4)
	// +40: playtime ms (4)
	// +44: track number (4) — PR #36 reads as int, not short
	// +48: track count (4)
	// +52: unknown (2)
	// +54: year (2)
	// +56: unknown (2)
	// +58: bit rate (2)
	// +60: sample rate (2)
	// +62: unknown (2)
	// +64: volume adjust (4)
	// +68: start time (4)
	// +72: end time (4)
	// +76: play count (4)
	// +80: unknown (2)
	// +82: compilation (2)
	// +84: unknown (12)
	// +96: play count again (4)
	// +100: last play date (4)
	// +104: disc number (1) + pad(1) + disc count(1) + pad(1) — PR #36
	// +108: rating (1)
	// +109: unknown (11)
	// +120: add date (4)
	// +124: unknown (4)
	// +128: persistent ID (8)
	// +136: unknown (20)
	// ... optionally album persistent ID at +300 (length > 156+144+8)
	if length < 24 {
		return t
	}

	base := offset
	safe := func(off, size int) bool { return base+off+size <= len(data) }

	if safe(16, 4) {
		t.TrackID = int(readUint32BE(data, base+16))
	}
	if safe(32, 4) {
		t.DateModified = macDateToTime(readUint32BE(data, base+32))
	}
	if safe(36, 4) {
		t.Size = int(readUint32BE(data, base+36))
	}
	if safe(40, 4) {
		t.TotalTime = int(readUint32BE(data, base+40))
	}
	if safe(44, 4) {
		t.TrackNumber = int(readUint32BE(data, base+44))
	}
	if safe(48, 4) {
		t.TrackCount = int(readUint32BE(data, base+48))
	}
	if safe(54, 2) {
		t.Year = int(int16(readUint16BE(data, base+54)))
	}
	if safe(58, 2) {
		t.BitRate = int(readUint16BE(data, base+58))
	}
	if safe(60, 2) {
		t.SampleRate = int(readUint16BE(data, base+60))
	}
	if safe(76, 4) {
		t.PlayCount = int(readUint32BE(data, base+76))
	}
	if safe(100, 4) {
		t.LastPlayDate = macDateToTime(readUint32BE(data, base+100))
	}
	if safe(104, 1) {
		t.DiscNumber = int(data[base+104])
	}
	if safe(106, 1) {
		t.DiscCount = int(data[base+106])
	}
	if safe(108, 1) {
		t.Rating = int(data[base+108])
	}
	if safe(120, 4) {
		t.DateAdded = macDateToTime(readUint32BE(data, base+120))
	}
	if safe(128, 8) {
		copy(t.PersistentID[:], data[base+128:base+136])
	}
	// Album persistent ID: at +300 if header is big enough (length > 308)
	if length > 308 && safe(300, 8) {
		copy(t.AlbumPersistentID[:], data[base+300:base+308])
	}

	return t
}

func parseHohm(data []byte, offset, length int, track *ITLTrack) {
	// hohm layout:
	// +0: tag (4), +4: length (4), +8: recLength (4), +12: hohmType (4)
	// +16: 12-byte header (byte 11 = encoding flag)
	// +28: 4-byte string data length
	// +32: 8-byte zeros
	// +40: string data
	if length < 40 {
		return
	}
	hohmType := int(readUint32BE(data, offset+12))
	encodingFlag := data[offset+16+11] // byte 11 of the 12-byte header

	strDataLen := int(readUint32BE(data, offset+28))
	strStart := offset + 40
	if strStart+strDataLen > offset+length || strStart+strDataLen > len(data) {
		// Clamp to available
		strDataLen = offset + length - strStart
		if strDataLen < 0 {
			return
		}
	}

	s, err := decodeHohmString(data[strStart:strStart+strDataLen], encodingFlag)
	if err != nil {
		return
	}

	switch hohmType {
	case 0x02:
		track.Name = s
	case 0x03:
		track.Album = s
	case 0x04:
		track.Artist = s
	case 0x05:
		track.Genre = s
	case 0x06:
		track.Kind = s
	case 0x0B:
		track.LocalURL = s
	case 0x0D:
		track.Location = s
	}
}

// parseHpim parses a playlist header (hpim) chunk.
func parseHpim(data []byte, offset, length int) ITLPlaylist {
	p := ITLPlaylist{}
	// hpim layout:
	// +0: "hpim" (4), +4: length (4), +8: recordLength (4), +12: subblocks (4)
	// +16: item count (4)
	// Remaining starts at offset+20, persistent ID at remaining[420:428]
	remaining := length - 20
	if remaining >= 428 {
		base := offset + 20
		copy(p.PersistentID[:], data[base+420:base+428])
	}
	return p
}

// parseHptm parses a playlist item (hptm) chunk and returns the track ID.
func parseHptm(data []byte, offset, length int) int {
	// hptm layout:
	// +0: "hptm" (4), +4: length (4)
	// +8: 16 unknown bytes
	// +24: track key/song ID (4)
	if length < 28 || offset+28 > len(data) {
		return -1
	}
	return int(readUint32BE(data, offset+24))
	// TODO: extract checked state from hptm
}

// parsePlaylistHohm parses a hohm chunk in playlist context.
func parsePlaylistHohm(data []byte, offset, length int, playlist *ITLPlaylist) {
	if length < 16 {
		return
	}
	hohmType := int(readUint32BE(data, offset+12))

	switch hohmType {
	case 0x64:
		// Playlist title — same string format as track hohm
		if length < 40 {
			return
		}
		encodingFlag := data[offset+16+11]
		strDataLen := int(readUint32BE(data, offset+28))
		strStart := offset + 40
		if strStart+strDataLen > offset+length || strStart+strDataLen > len(data) {
			strDataLen = offset + length - strStart
			if strDataLen < 0 {
				return
			}
		}
		s, err := decodeHohmString(data[strStart:strStart+strDataLen], encodingFlag)
		if err != nil {
			return
		}
		playlist.Title = s

	case 0x65:
		// Smart criteria: 8 zero bytes + raw blob
		blobStart := offset + 40 + 8
		if blobStart < offset+length && blobStart < len(data) {
			end := offset + length
			if end > len(data) {
				end = len(data)
			}
			playlist.SmartCriteria = make([]byte, end-blobStart)
			copy(playlist.SmartCriteria, data[blobStart:end])
			playlist.IsSmart = true
		}

	case 0x66:
		// Smart info: 8 zero bytes + raw blob
		blobStart := offset + 40 + 8
		if blobStart < offset+length && blobStart < len(data) {
			end := offset + length
			if end > len(data) {
				end = len(data)
			}
			playlist.SmartInfo = make([]byte, end-blobStart)
			copy(playlist.SmartInfo, data[blobStart:end])
		}
	}
}

// ---------------------------------------------------------------------------
// Chunk builders for write path
// ---------------------------------------------------------------------------

// buildHohmChunk builds a hohm chunk for a given type and string value.
func buildHohmChunk(hohmType uint32, value string) []byte {
	encodedStr, encFlag := encodeHohmString(value)
	chunkLen := 40 + len(encodedStr)
	buf := make([]byte, chunkLen)
	copy(buf[0:4], "hohm")
	writeUint32BE(buf, 4, uint32(chunkLen))
	writeUint32BE(buf, 8, uint32(chunkLen))
	writeUint32BE(buf, 12, hohmType)
	buf[16+11] = encFlag
	writeUint32BE(buf, 28, uint32(len(encodedStr)))
	// bytes 32-39 are zero (already)
	copy(buf[40:], encodedStr)
	return buf
}

// buildHtimChunk builds a 156-byte htim chunk for a new track.
func buildHtimChunk(trackID int, track ITLNewTrack) []byte {
	htimLen := 156
	buf := make([]byte, htimLen)
	copy(buf[0:4], "htim")
	writeUint32BE(buf, 4, uint32(htimLen))
	writeUint32BE(buf, 8, uint32(htimLen)) // recordLength
	writeUint32BE(buf, 16, uint32(trackID))
	writeUint32BE(buf, 36, uint32(track.Size))
	writeUint32BE(buf, 40, uint32(track.TotalTime))
	writeUint32BE(buf, 44, uint32(track.TrackNumber))
	if track.Year > 0 {
		binary.BigEndian.PutUint16(buf[54:56], uint16(track.Year))
	}
	if track.BitRate > 0 {
		binary.BigEndian.PutUint16(buf[58:60], uint16(track.BitRate))
	}
	if track.SampleRate > 0 {
		binary.BigEndian.PutUint16(buf[60:62], uint16(track.SampleRate))
	}
	buf[104] = byte(track.DiscNumber)
	// Random persistent ID
	var pid [8]byte
	_, _ = rand.Read(pid[:])
	copy(buf[128:136], pid[:])
	return buf
}

// buildHpimChunk builds an hpim chunk for a new playlist.
func buildHpimChunk(itemCount int) []byte {
	// Minimum hpim: 20 bytes header + 428 bytes remaining (for persistent ID at [420:428])
	hpimLen := 20 + 428
	buf := make([]byte, hpimLen)
	copy(buf[0:4], "hpim")
	writeUint32BE(buf, 4, uint32(hpimLen))
	writeUint32BE(buf, 8, uint32(hpimLen)) // recordLength
	writeUint32BE(buf, 16, uint32(itemCount))
	// Random persistent ID at remaining[420:428] = offset 20+420 = 440
	var pid [8]byte
	_, _ = rand.Read(pid[:])
	copy(buf[440:448], pid[:])
	return buf
}

// buildHptmChunk builds an hptm chunk referencing a track ID.
func buildHptmChunk(trackID int) []byte {
	hptmLen := 28
	buf := make([]byte, hptmLen)
	copy(buf[0:4], "hptm")
	writeUint32BE(buf, 4, uint32(hptmLen))
	// 16 unknown bytes at [8:24] (zero)
	writeUint32BE(buf, 24, uint32(trackID))
	return buf
}

// ---------------------------------------------------------------------------
// ValidateITL performs a quick validation of an ITL file.
// ---------------------------------------------------------------------------

// ValidateITL checks that a file is a valid ITL by reading and decrypting the header.
func ValidateITL(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading ITL: %w", err)
	}
	if len(data) < 20 {
		return fmt.Errorf("file too small to be ITL: %d bytes", len(data))
	}

	hdr, err := parseHdfmHeader(data)
	if err != nil {
		return err
	}

	payload := data[hdr.headerLen:]
	if len(payload) == 0 {
		return fmt.Errorf("ITL has no payload after header")
	}

	decrypted := itlDecrypt(hdr.version, payload)
	decompressed, _ := itlInflate(decrypted)

	if len(decompressed) < 4 {
		return fmt.Errorf("decrypted ITL payload too short")
	}

	tag := readTag(decompressed, 0)
	validTags := map[string]bool{"hdsm": true, "msdh": true, "htim": true, "hohm": true}
	if !validTags[tag] {
		return fmt.Errorf("invalid first chunk tag after decryption: %q", tag)
	}

	return nil
}

// ---------------------------------------------------------------------------
// UpdateITLLocations — the write path (ProcessLibrary port)
// ---------------------------------------------------------------------------

// UpdateITLLocations reads an ITL file, updates file locations for the specified
// persistent IDs, and writes the result to outputPath.
func UpdateITLLocations(inputPath, outputPath string, updates []ITLLocationUpdate) (*ITLWriteBackResult, error) {
	if len(updates) == 0 {
		return &ITLWriteBackResult{OutputPath: outputPath}, nil
	}

	// Build lookup map
	updateMap := make(map[string]string, len(updates))
	for _, u := range updates {
		updateMap[strings.ToLower(u.PersistentID)] = u.NewLocation
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("reading ITL: %w", err)
	}

	hdr, err := parseHdfmHeader(data)
	if err != nil {
		return nil, err
	}

	payload := data[hdr.headerLen:]
	decrypted := itlDecrypt(hdr.version, payload)
	decompressed, wasCompressed := itlInflate(decrypted)

	// Walk and rewrite
	newData, updatedCount := rewriteChunks(decompressed, updateMap)

	// Compress if original was
	var finalPayload []byte
	if wasCompressed {
		finalPayload = itlDeflate(newData)
	} else {
		finalPayload = newData
	}

	// Encrypt
	encrypted := itlEncrypt(hdr.version, finalPayload)

	// Build new file
	newFileLen := uint32(len(encrypted)) + hdr.headerLen
	newHeader := buildHdfmHeader(hdr.version, hdr.headerRemainder, newFileLen, hdr.unknown)

	outData := make([]byte, 0, len(newHeader)+len(encrypted))
	outData = append(outData, newHeader...)
	outData = append(outData, encrypted...)

	if err := os.WriteFile(outputPath, outData, 0644); err != nil {
		return nil, fmt.Errorf("writing ITL: %w", err)
	}

	return &ITLWriteBackResult{
		UpdatedCount: updatedCount,
		OutputPath:   outputPath,
	}, nil
}

// rewriteChunks walks through decompressed ITL data chunk by chunk,
// replacing location strings (hohm type 0x0D) for matching persistent IDs.
// Returns the new data buffer and count of updates made.
func rewriteChunks(data []byte, updateMap map[string]string) ([]byte, int) {
	var out bytes.Buffer
	offset := 0
	updatedCount := 0
	var currentPID string

	for offset+8 <= len(data) {
		tag := readTag(data, offset)
		if tag == "" {
			// Write remaining bytes
			out.Write(data[offset:])
			break
		}
		length := int(readUint32BE(data, offset+4))
		if length < 8 || offset+length > len(data) {
			out.Write(data[offset:])
			break
		}

		switch tag {
		case "hdsm":
			// Per PR #36: extendedLength at offset+8
			extLen := int(readUint32BE(data, offset+8))
			actualLen := length
			if extLen > length && offset+extLen <= len(data) {
				actualLen = extLen
			}
			// Write the hdsm chunk through, but process sub-chunks inside
			// For simplicity, recursively rewrite hdsm content
			hdsm := data[offset : offset+actualLen]
			rewritten, cnt := rewriteHdsmContent(hdsm, updateMap, &currentPID)
			out.Write(rewritten)
			updatedCount += cnt
			offset += actualLen

		case "htim":
			// Extract persistent ID from htim
			if offset+136 <= len(data) {
				pid := pidToHex([8]byte(data[offset+128 : offset+136]))
				currentPID = strings.ToLower(pid)
			}
			out.Write(data[offset : offset+length])
			offset += length

		case "hohm":
			if newLoc, ok := shouldUpdateHohm(data, offset, length, currentPID, updateMap); ok {
				rewritten := rewriteHohmLocation(data, offset, length, newLoc)
				out.Write(rewritten)
				updatedCount++
			} else {
				out.Write(data[offset : offset+length])
			}
			offset += length

		default:
			out.Write(data[offset : offset+length])
			offset += length
		}
	}

	return out.Bytes(), updatedCount
}

func rewriteHdsmContent(hdsm []byte, updateMap map[string]string, currentPID *string) ([]byte, int) {
	if len(hdsm) < 12 {
		return hdsm, 0
	}

	// hdsm header: tag(4) + length(4) + extLen(4) = 12 bytes minimum
	basicLen := int(readUint32BE(hdsm, 4))
	extLen := int(readUint32BE(hdsm, 8))

	// The hdsm header is 12 bytes, sub-chunks start at offset 12
	var out bytes.Buffer
	out.Write(hdsm[:12]) // Write hdsm header

	updatedCount := 0
	subOffset := 12

	// Determine where sub-content ends
	contentEnd := basicLen
	if extLen > basicLen && extLen <= len(hdsm) {
		contentEnd = extLen
	}
	if contentEnd > len(hdsm) {
		contentEnd = len(hdsm)
	}

	for subOffset+8 <= contentEnd {
		tag := readTag(hdsm, subOffset)
		if tag == "" {
			break
		}
		chunkLen := int(readUint32BE(hdsm, subOffset+4))
		if chunkLen < 8 || subOffset+chunkLen > contentEnd {
			break
		}

		switch tag {
		case "htim":
			if subOffset+108 <= len(hdsm) {
				pid := pidToHex([8]byte(hdsm[subOffset+100 : subOffset+108]))
				*currentPID = strings.ToLower(pid)
			}
			out.Write(hdsm[subOffset : subOffset+chunkLen])

		case "hohm":
			if newLoc, ok := shouldUpdateHohm(hdsm, subOffset, chunkLen, *currentPID, updateMap); ok {
				rewritten := rewriteHohmLocation(hdsm, subOffset, chunkLen, newLoc)
				out.Write(rewritten)
				updatedCount++
			} else {
				out.Write(hdsm[subOffset : subOffset+chunkLen])
			}

		default:
			out.Write(hdsm[subOffset : subOffset+chunkLen])
		}
		subOffset += chunkLen
	}

	// Write any trailing bytes
	if subOffset < len(hdsm) {
		out.Write(hdsm[subOffset:])
	}

	result := out.Bytes()

	// Update hdsm length fields
	newLen := uint32(len(result))
	writeUint32BE(result, 4, newLen)
	writeUint32BE(result, 8, newLen)

	return result, updatedCount
}

func shouldUpdateHohm(data []byte, offset, length int, currentPID string, updateMap map[string]string) (string, bool) {
	if length < 40 {
		return "", false
	}
	hohmType := int(readUint32BE(data, offset+12))
	// 0x0D = file location, 0x0B = local URL (used by audiobooks/podcasts per titl issue #25)
	if hohmType != 0x0D && hohmType != 0x0B {
		return "", false
	}
	if currentPID == "" {
		return "", false
	}
	newLoc, ok := updateMap[currentPID]
	if ok && hohmType == 0x0B {
		// For URL-style locations, encode as file:// URL
		if !strings.HasPrefix(newLoc, "file://") {
			newLoc = "file://localhost/" + strings.TrimPrefix(newLoc, "/")
		}
	}
	return newLoc, ok
}

func rewriteHohmLocation(data []byte, offset, length int, newLocation string) []byte {
	// Encode new string
	encodedStr, encodingFlag := encodeHohmString(newLocation)

	// Build new hohm chunk
	// Header: tag(4) + length(4) + recLength(4) + hohmType(4) + 12-byte header + 4-byte strLen + 8-byte zeros + string data
	newStrDataLen := len(encodedStr)
	newChunkLen := 40 + newStrDataLen

	buf := make([]byte, newChunkLen)
	// Copy tag
	copy(buf[0:4], data[offset:offset+4])
	// New length
	writeUint32BE(buf, 4, uint32(newChunkLen))
	// New recLength (same as length for hohm)
	writeUint32BE(buf, 8, uint32(newChunkLen))
	// hohmType
	copy(buf[12:16], data[offset+12:offset+16])
	// Copy the 12-byte header, update encoding flag
	if offset+28 <= len(data) {
		copy(buf[16:28], data[offset+16:offset+28])
	}
	buf[16+11] = encodingFlag
	// String data length
	writeUint32BE(buf, 28, uint32(newStrDataLen))
	// 8 bytes zeros at 32-39 (already zero)
	// String data
	copy(buf[40:], encodedStr)

	return buf
}

// ---------------------------------------------------------------------------
// InsertITLTracks — insert new tracks into an ITL file
// ---------------------------------------------------------------------------

// InsertITLTracks reads an ITL file, appends new tracks after existing ones,
// and writes the result to outputPath.
func InsertITLTracks(inputPath, outputPath string, tracks []ITLNewTrack) (*ITLWriteBackResult, error) {
	if len(tracks) == 0 {
		return &ITLWriteBackResult{OutputPath: outputPath}, nil
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("reading ITL: %w", err)
	}

	hdr, err := parseHdfmHeader(data)
	if err != nil {
		return nil, err
	}

	payload := data[hdr.headerLen:]
	decrypted := itlDecrypt(hdr.version, payload)
	decompressed, wasCompressed := itlInflate(decrypted)

	// Find max track ID
	maxID := findMaxTrackID(decompressed)

	// Find insertion point: after last hohm that follows an htim, before first hpim
	insertOffset := findTrackInsertOffset(decompressed)

	// Build new track chunks
	var newChunks bytes.Buffer
	for i, tr := range tracks {
		trackID := maxID + 1 + i
		htim := buildHtimChunk(trackID, tr)
		newChunks.Write(htim)

		// Add hohm chunks for each non-empty string field
		if tr.Name != "" {
			newChunks.Write(buildHohmChunk(0x02, tr.Name))
		}
		if tr.Album != "" {
			newChunks.Write(buildHohmChunk(0x03, tr.Album))
		}
		if tr.Artist != "" {
			newChunks.Write(buildHohmChunk(0x04, tr.Artist))
		}
		if tr.Genre != "" {
			newChunks.Write(buildHohmChunk(0x05, tr.Genre))
		}
		if tr.Kind != "" {
			newChunks.Write(buildHohmChunk(0x06, tr.Kind))
		}
		if tr.Location != "" {
			newChunks.Write(buildHohmChunk(0x0D, tr.Location))
		}
	}

	// Splice: before insertOffset + newChunks + after insertOffset
	var newData bytes.Buffer
	newData.Write(decompressed[:insertOffset])
	newData.Write(newChunks.Bytes())
	newData.Write(decompressed[insertOffset:])

	return writeITLFile(outputPath, hdr, newData.Bytes(), wasCompressed, len(tracks))
}

// findMaxTrackID walks chunks to find the highest track ID.
func findMaxTrackID(data []byte) int {
	maxID := 0
	offset := 0
	for offset+8 <= len(data) {
		tag := readTag(data, offset)
		if tag == "" {
			break
		}
		length := int(readUint32BE(data, offset+4))
		if length < 8 || offset+length > len(data) {
			break
		}
		if tag == "htim" && offset+20 <= len(data) {
			id := int(readUint32BE(data, offset+16))
			if id > maxID {
				maxID = id
			}
		}
		offset += length
	}
	return maxID
}

// findTrackInsertOffset finds the byte offset where new tracks should be inserted.
// This is after the last track-related chunk (htim or hohm following htim) and
// before any playlist chunk (hpim).
func findTrackInsertOffset(data []byte) int {
	offset := 0
	lastTrackEnd := 0
	inTrackSection := false
	for offset+8 <= len(data) {
		tag := readTag(data, offset)
		if tag == "" {
			break
		}
		length := int(readUint32BE(data, offset+4))
		if length < 8 || offset+length > len(data) {
			break
		}
		switch tag {
		case "htim":
			inTrackSection = true
			lastTrackEnd = offset + length
		case "hohm":
			if inTrackSection {
				lastTrackEnd = offset + length
			}
		case "hpim":
			// Playlist section starts here; insert before it
			if lastTrackEnd > 0 {
				return lastTrackEnd
			}
			return offset
		}
		offset += length
	}
	if lastTrackEnd > 0 {
		return lastTrackEnd
	}
	return len(data)
}

// ---------------------------------------------------------------------------
// RewriteITLExtensions — rewrite file extensions in all location hohms
// ---------------------------------------------------------------------------

// RewriteITLExtensions reads an ITL file and replaces file extensions in all
// location strings (hohm 0x0D and 0x0B).
func RewriteITLExtensions(inputPath, outputPath string, oldExt, newExt string) (*ITLWriteBackResult, error) {
	// Normalize extensions to include dot
	if !strings.HasPrefix(oldExt, ".") {
		oldExt = "." + oldExt
	}
	if !strings.HasPrefix(newExt, ".") {
		newExt = "." + newExt
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("reading ITL: %w", err)
	}

	hdr, err := parseHdfmHeader(data)
	if err != nil {
		return nil, err
	}

	payload := data[hdr.headerLen:]
	decrypted := itlDecrypt(hdr.version, payload)
	decompressed, wasCompressed := itlInflate(decrypted)

	newData, count := rewriteExtensionsInChunks(decompressed, oldExt, newExt)

	return writeITLFile(outputPath, hdr, newData, wasCompressed, count)
}

// rewriteExtensionsInChunks walks chunks and rewrites extensions in location hohms.
func rewriteExtensionsInChunks(data []byte, oldExt, newExt string) ([]byte, int) {
	var out bytes.Buffer
	offset := 0
	count := 0

	for offset+8 <= len(data) {
		tag := readTag(data, offset)
		if tag == "" {
			out.Write(data[offset:])
			break
		}
		length := int(readUint32BE(data, offset+4))
		if length < 8 || offset+length > len(data) {
			out.Write(data[offset:])
			break
		}

		if tag == "hohm" && length >= 40 {
			hohmType := int(readUint32BE(data, offset+12))
			if hohmType == 0x0D || hohmType == 0x0B {
				// Read current string
				encodingFlag := data[offset+16+11]
				strDataLen := int(readUint32BE(data, offset+28))
				strStart := offset + 40
				if strStart+strDataLen <= offset+length && strStart+strDataLen <= len(data) {
					s, err := decodeHohmString(data[strStart:strStart+strDataLen], encodingFlag)
					if err == nil && strings.HasSuffix(strings.ToLower(s), strings.ToLower(oldExt)) {
						newLoc := s[:len(s)-len(oldExt)] + newExt
						rewritten := rewriteHohmLocation(data, offset, length, newLoc)
						out.Write(rewritten)
						count++
						offset += length
						continue
					}
				}
			}
		}

		out.Write(data[offset : offset+length])
		offset += length
	}

	return out.Bytes(), count
}

// ---------------------------------------------------------------------------
// InsertITLPlaylist — insert a new playlist into an ITL file
// ---------------------------------------------------------------------------

// InsertITLPlaylist reads an ITL file, appends a new playlist, and writes
// the result to outputPath.
func InsertITLPlaylist(inputPath, outputPath string, playlist ITLNewPlaylist) (*ITLWriteBackResult, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("reading ITL: %w", err)
	}

	hdr, err := parseHdfmHeader(data)
	if err != nil {
		return nil, err
	}

	payload := data[hdr.headerLen:]
	decrypted := itlDecrypt(hdr.version, payload)
	decompressed, wasCompressed := itlInflate(decrypted)

	// Build playlist chunks
	var plChunks bytes.Buffer
	plChunks.Write(buildHpimChunk(len(playlist.TrackIDs)))
	plChunks.Write(buildHohmChunk(0x64, playlist.Title))
	for _, tid := range playlist.TrackIDs {
		plChunks.Write(buildHptmChunk(tid))
	}

	// Append at end
	var newData bytes.Buffer
	newData.Write(decompressed)
	newData.Write(plChunks.Bytes())

	return writeITLFile(outputPath, hdr, newData.Bytes(), wasCompressed, 1)
}

// writeITLFile handles compression, encryption, and writing of an ITL file.
func writeITLFile(outputPath string, hdr *hdfmHeader, payload []byte, compress bool, count int) (*ITLWriteBackResult, error) {
	var finalPayload []byte
	if compress {
		finalPayload = itlDeflate(payload)
	} else {
		finalPayload = payload
	}

	encrypted := itlEncrypt(hdr.version, finalPayload)

	newFileLen := uint32(len(encrypted)) + hdr.headerLen
	newHeader := buildHdfmHeader(hdr.version, hdr.headerRemainder, newFileLen, hdr.unknown)

	outData := make([]byte, 0, len(newHeader)+len(encrypted))
	outData = append(outData, newHeader...)
	outData = append(outData, encrypted...)

	if err := os.WriteFile(outputPath, outData, 0644); err != nil {
		return nil, fmt.Errorf("writing ITL: %w", err)
	}

	return &ITLWriteBackResult{
		UpdatedCount: count,
		OutputPath:   outputPath,
	}, nil
}
