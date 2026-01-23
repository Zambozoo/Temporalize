package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/skip2/go-qrcode"
)

const (
	qrcodeDir = "assets/qrcodes"
	linksFile = "links_fixed.json"
)

type PlatformData struct {
	ID string `json:"id"`
}

type SongEntry struct {
	Title        string       `json:"title"`
	Artist       string       `json:"artist"`
	Year         int          `json:"year"`
	Explicit     bool         `json:"explicit"`
	Spotify      PlatformData `json:"spotify"`
	AppleMusic   PlatformData `json:"appleMusic"`
	AmazonMusic  PlatformData `json:"amazonMusic"`
	YoutubeMusic PlatformData `json:"youtubeMusic"`
}

func (s *SongEntry) FileName() string {
	filename := fmt.Sprintf("%d-%s", s.Year, s.Title)
	// Define a regular expression to match invalid characters and replace them with an underscore
	sanitized := strings.TrimSpace(regexp.MustCompile(`[/\?%*:|"<>\x00-\x1F]`).ReplaceAllString(filename, "_"))
	// remove multiple consecutive underscores
	return regexp.MustCompile(`_+`).ReplaceAllString(sanitized, "_")
}

func qrCodeBytes(explicit bool, strs ...string) []byte {
	var bytes []byte

	// First byte: 1 if explicit, 0 if not
	if explicit {
		bytes = append(bytes, 1)
	} else {
		bytes = append(bytes, 0)
	}

	for _, s := range strs {
		if len(s) == 0 {
			continue
		}
		b := []byte(s)
		// Offset the first byte by 128
		b[0] = byte(int(b[0]) + 128)
		bytes = append(bytes, b...)
	}
	return bytes
}

func generateQRCode(s *SongEntry) {
	filename := fmt.Sprintf("%s/%s.png", qrcodeDir, s.FileName())

	// Order: Amazon, Apple, Spotify, YouTube
	var ids []string

	if s.AmazonMusic.ID != "" {
		ids = append(ids, strings.Split(s.AmazonMusic.ID, ":")...)
	}
	if s.AppleMusic.ID != "" {
		ids = append(ids, strings.Split(s.AppleMusic.ID, ":")...)
	}
	if s.Spotify.ID != "" {
		ids = append(ids, s.Spotify.ID)
	}
	if s.YoutubeMusic.ID != "" {
		ids = append(ids, s.YoutubeMusic.ID)
	}

	qrBytes := qrCodeBytes(s.Explicit, ids...)

	// skip2/go-qrcode takes a string, but treats it as bytes for data encoding
	// It automatically selects the mode. If we give it bytes that aren't valid UTF-8,
	// we need to make sure it handles them.
	// Actually, the Encode function takes string.
	// Let's rely on Go's string(byteSlice) conversion which preserves bytes.

	err := qrcode.WriteFile(string(qrBytes), qrcode.Medium, 256, filename)
	if err != nil {
		fmt.Printf("Error creating QR code for %s: %v\n", s.Title, err)
		return
	}
}

func main() {
	f, err := os.Open(linksFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	var songs []SongEntry
	if err := json.NewDecoder(f).Decode(&songs); err != nil {
		panic(err)
	}

	fmt.Printf("Generating QR codes for %d songs...\n", len(songs))
	for i := range songs {
		generateQRCode(&songs[i])
	}
	fmt.Println("Done.")
}
