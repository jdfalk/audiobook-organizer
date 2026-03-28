package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

func main() {
	lib, err := itunes.ParseITL(os.Args[1])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Printf("Tracks: %d\n", len(lib.Tracks))

	raw := lib.RawData()
	// Find the track msdh (blockType 0x01)
	offset := 0
	for offset+16 <= len(raw) {
		tag := string(raw[offset:offset+4])
		if tag != "msdh" { break }
		tl := le32(raw, offset+8)
		bt := le32(raw, offset+12)
		if bt == 0x01 {
			// Found track msdh. Get the first mith.
			hl := le32(raw, offset+4)
			sub := offset + int(hl) // skip msdh header

			// Skip mlth
			if string(raw[sub:sub+4]) == "mlth" {
				mlthLen := le32(raw, sub+4)
				sub += int(mlthLen)
			}

			// Should be at first mith
			if string(raw[sub:sub+4]) == "mith" {
				mithHL := le32(raw, sub+4)
				mithTL := le32(raw, sub+8)
				fmt.Printf("\nFirst mith at offset %d (relative to msdh):\n", sub-offset)
				fmt.Printf("  headerLen=%d totalLen=%d\n", mithHL, mithTL)

				// Dump first 256 bytes of mith
				end := sub + 256
				if end > sub + int(mithTL) { end = sub + int(mithTL) }
				if end > len(raw) { end = len(raw) }
				fmt.Printf("\nmith header (first 256 bytes):\n")
				fmt.Println(hex.Dump(raw[sub:end]))

				// Show bytes at offset 128 (claimed PID location)
				if sub+136 <= len(raw) {
					fmt.Printf("Bytes at offset+128: %x\n", raw[sub+128:sub+136])
				}

				// Dump the mhoh sub-blocks (starting at mithHL)
				mhohStart := sub + int(mithHL)
				if mhohStart < sub + int(mithTL) {
					fmt.Printf("\nmhoh sub-blocks (starting at offset %d = mith + headerLen %d):\n", mhohStart-offset, mithHL)
					mhohEnd := mhohStart + 512
					if mhohEnd > sub + int(mithTL) { mhohEnd = sub + int(mithTL) }
					if mhohEnd > len(raw) { mhohEnd = len(raw) }
					fmt.Println(hex.Dump(raw[mhohStart:mhohEnd]))
				}
			} else {
				fmt.Printf("Expected mith but got %q at sub offset %d\n", string(raw[sub:sub+4]), sub-offset)
			}
			break
		}
		offset += int(tl)
	}
}

func le32(data []byte, offset int) uint32 {
	return uint32(data[offset]) | uint32(data[offset+1])<<8 |
		uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
}
