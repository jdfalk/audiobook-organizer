// file: internal/itunes/smart_criteria_reader.go
// version: 1.0.0
// guid: 8b6c7d5e-9f0a-4a70-b8c5-3d7e0f1b9a99
//
// Parser for the iTunes Smart Criteria binary blob format.
//
// The blob is a nested structure of conjunctions (AND/OR) containing
// rules. Each rule has a field selector, an operator, and operand(s).
// This parser extracts the rule tree into a Go struct suitable for
// translation to our DSL query string.
//
// References:
// - https://github.com/banshee-project/banshee/blob/master/src/Extensions/Banshee.SmartPlaylist/Banshee.SmartPlaylist/SmartPlaylistDefinition.cs
// - https://github.com/MusicPlayerDaemon/mpdris2/blob/master/doc/smart-playlists.txt
// - Reverse engineering from hexdumps of known playlists

package itunes

import (
	"encoding/binary"
	"fmt"
)

// SmartRule represents one rule in a smart playlist.
type SmartRule struct {
	Field    SmartField
	Operator SmartOperator
	Operands []string
}

// SmartField identifies which track attribute the rule checks.
type SmartField int

const (
	SmartFieldName        SmartField = 0x02
	SmartFieldArtist      SmartField = 0x03
	SmartFieldAlbum       SmartField = 0x05
	SmartFieldGenre       SmartField = 0x08
	SmartFieldYear        SmartField = 0x07
	SmartFieldPlayCount   SmartField = 0x16
	SmartFieldRating      SmartField = 0x19
	SmartFieldDateAdded   SmartField = 0x10
	SmartFieldLastPlayed  SmartField = 0x17
	SmartFieldBitRate     SmartField = 0x0F
	SmartFieldComment     SmartField = 0x0E
	SmartFieldBookmark    SmartField = 0x28
	SmartFieldMediaKind   SmartField = 0x3C
	SmartFieldDescription SmartField = 0x36
)

// SmartFieldName returns a human-readable name for a SmartField.
func (f SmartField) String() string {
	switch f {
	case SmartFieldName:
		return "title"
	case SmartFieldArtist:
		return "author"
	case SmartFieldAlbum:
		return "album"
	case SmartFieldGenre:
		return "genre"
	case SmartFieldYear:
		return "year"
	case SmartFieldPlayCount:
		return "play_count"
	case SmartFieldRating:
		return "rating"
	case SmartFieldDateAdded:
		return "date_added"
	case SmartFieldLastPlayed:
		return "last_played"
	case SmartFieldBitRate:
		return "bitrate"
	case SmartFieldComment:
		return "comment"
	case SmartFieldBookmark:
		return "bookmark"
	case SmartFieldMediaKind:
		return "media_kind"
	case SmartFieldDescription:
		return "description"
	default:
		return fmt.Sprintf("unknown_0x%02X", int(f))
	}
}

// SmartOperator identifies the comparison operation.
type SmartOperator int

const (
	SmartOpIs            SmartOperator = 0x01
	SmartOpIsNot         SmartOperator = 0x02
	SmartOpContains      SmartOperator = 0x03
	SmartOpDoesNotContain SmartOperator = 0x04
	SmartOpStartsWith    SmartOperator = 0x05
	SmartOpEndsWith      SmartOperator = 0x06
	SmartOpGreaterThan   SmartOperator = 0x07
	SmartOpLessThan      SmartOperator = 0x0B
	SmartOpInRange       SmartOperator = 0x0D
	SmartOpIsTrue        SmartOperator = 0x0F
	SmartOpIsFalse       SmartOperator = 0x10
)

func (o SmartOperator) String() string {
	switch o {
	case SmartOpIs:
		return "is"
	case SmartOpIsNot:
		return "is_not"
	case SmartOpContains:
		return "contains"
	case SmartOpDoesNotContain:
		return "does_not_contain"
	case SmartOpStartsWith:
		return "starts_with"
	case SmartOpEndsWith:
		return "ends_with"
	case SmartOpGreaterThan:
		return "greater_than"
	case SmartOpLessThan:
		return "less_than"
	case SmartOpInRange:
		return "in_range"
	case SmartOpIsTrue:
		return "is_true"
	case SmartOpIsFalse:
		return "is_false"
	default:
		return fmt.Sprintf("op_0x%02X", int(o))
	}
}

// SmartCriteriaResult is the parsed output of a Smart Criteria blob.
type SmartCriteriaResult struct {
	Conjunction string      // "AND" or "OR"
	Rules       []SmartRule
	RawLength   int
}

// ParseSmartCriteria attempts to parse an iTunes Smart Criteria blob.
// Returns the parsed rule tree or an error if the format is
// unrecognized. Tolerant of unknown fields/operators — those are
// recorded as raw hex values rather than causing parse failure.
func ParseSmartCriteria(data []byte) (*SmartCriteriaResult, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("smart criteria too short: %d bytes", len(data))
	}

	result := &SmartCriteriaResult{
		Conjunction: "AND",
		RawLength:   len(data),
	}

	// The Smart Criteria blob starts with a header. The exact format
	// varies between iTunes versions but the general structure is:
	//   - 4 bytes: unknown (version?)
	//   - 1 byte: conjunction type (1=AND, 0=OR)
	//   - 3 bytes: padding/unknown
	//   - repeated: rule entries
	//
	// Each rule entry is typically 136 bytes with:
	//   offset 0: field type (4 bytes LE)
	//   offset 4: operator (4 bytes LE)
	//   offset 8+: operand data (string or numeric)

	if len(data) >= 5 && data[4] == 0x00 {
		result.Conjunction = "OR"
	}

	// Skip the 8-byte header.
	pos := 8
	ruleSize := 136

	for pos+ruleSize <= len(data) {
		rule, err := parseOneRule(data[pos : pos+ruleSize])
		if err == nil {
			result.Rules = append(result.Rules, rule)
		}
		pos += ruleSize
	}

	return result, nil
}

func parseOneRule(data []byte) (SmartRule, error) {
	if len(data) < 12 {
		return SmartRule{}, fmt.Errorf("rule too short")
	}

	field := SmartField(binary.LittleEndian.Uint32(data[0:4]))
	operator := SmartOperator(binary.LittleEndian.Uint32(data[4:8]))

	rule := SmartRule{
		Field:    field,
		Operator: operator,
	}

	// Extract string operand from offset 56, UTF-16LE encoded.
	// Length is at offset 52 as uint32.
	if len(data) >= 60 {
		strLen := int(binary.LittleEndian.Uint32(data[52:56]))
		if strLen > 0 && 56+strLen*2 <= len(data) {
			s := decodeUTF16LE(data[56 : 56+strLen*2])
			if s != "" {
				rule.Operands = append(rule.Operands, s)
			}
		}
	}

	// For numeric fields, extract the value from offset 8 as int64.
	if len(rule.Operands) == 0 && len(data) >= 16 {
		val := int64(binary.LittleEndian.Uint64(data[8:16]))
		if val != 0 {
			rule.Operands = append(rule.Operands, fmt.Sprintf("%d", val))
		}
	}

	return rule, nil
}

func decodeUTF16LE(data []byte) string {
	if len(data)%2 != 0 {
		return ""
	}
	runes := make([]rune, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		r := rune(binary.LittleEndian.Uint16(data[i : i+2]))
		if r == 0 {
			break
		}
		runes = append(runes, r)
	}
	return string(runes)
}
