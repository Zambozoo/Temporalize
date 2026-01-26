package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"temporalize/internal/models"
)

func main() {
	inputFile := flag.String("input", "lookup.json", "Path to input JSON file")
	outputDir := flag.String("output", "assets/generated", "Output directory for generated assets")
	flag.Parse()

	if err := run(*inputFile, *outputDir); err != nil {
		panic(err)
	}
}

func run(inputFile, outputDir string) error {
	// Read Generated Songs
	genSongs, err := readGeneratedSongs(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read generated songs: %w", err)
	}

	fmt.Printf("Loaded %d songs from %s\n", len(genSongs), inputFile)

	for i, genSong := range genSongs {
		if genSong.Invalid {
			continue
		}

		fmt.Printf("[%d/%d] Generating assets for %s...\n", i+1, len(genSongs), genSong.Title)

		// Convert back to models.Song
		song := &models.Song{
			Title:        genSong.Title,
			Artists:      genSong.Artists,
			Year:         genSong.Year,
			Explicit:     genSong.Explicit,
			Genre:        genSong.Genre,
			ThumbnailURL: genSong.ThumbnailURL,
			Spotify:      extractSpotifyID(genSong.Spotify),
			AppleMusic:   extractAppleMusicID(genSong.AppleMusic),
			AmazonMusic:  extractAmazonMusicID(genSong.AmazonMusic),
			YoutubeMusic: extractYoutubeMusicID(genSong.YoutubeMusic),
		}

		// F. Generate Assets
		// 1. QR Code
		qrImg, err := createQRCodeImage(song)
		if err != nil {
			fmt.Printf("  -> Failed to generate QR code: %v\n", err)
			continue
		}

		// 2. Card Front
		if err := generateCardFront(song, outputDir); err != nil {
			fmt.Printf("  -> Failed to generate Card Front: %v\n", err)
			continue
		}

		// 3. Card Back
		if err := generateCardBack(song, qrImg, outputDir); err != nil {
			fmt.Printf("  -> Failed to generate Card Back: %v\n", err)
			continue
		}
	}
	return nil
}

func readGeneratedSongs(path string) ([]models.GeneratedSong, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var songs []models.GeneratedSong
	if err := json.NewDecoder(f).Decode(&songs); err != nil {
		return nil, err
	}
	return songs, nil
}

// Extraction Helpers

func extractSpotifyID(link string) string {
	// Expected: https://open.spotify.com/track/<ID>
	prefix := "https://open.spotify.com/track/"
	if strings.HasPrefix(link, prefix) {
		return strings.TrimPrefix(link, prefix)
	}
	return ""
}

func extractAppleMusicID(link string) string {
	// Expected: https://music.apple.com/us/album/_/<AlbumID>?i=<TrackID>
	// Output: <AlbumID>:<TrackID>
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	trackID := u.Query().Get("i")

	// Extract Album ID from path
	// Path is /us/album/_/<AlbumID>
	parts := strings.Split(u.Path, "/")
	if len(parts) > 0 {
		albumID := parts[len(parts)-1]
		if albumID != "" && trackID != "" {
			return fmt.Sprintf("%s:%s", albumID, trackID)
		}
	}
	return ""
}

func extractAmazonMusicID(link string) string {
	// Expected: https://music.amazon.com/albums/<AlbumASIN>?trackAsin=<TrackASIN>
	// Output: <AlbumASIN>:<TrackASIN>
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	trackASIN := u.Query().Get("trackAsin")

	// Extract Album ASIN from path
	// Path is /albums/<AlbumASIN>
	parts := strings.Split(u.Path, "/")
	if len(parts) > 0 {
		albumASIN := parts[len(parts)-1]
		if albumASIN != "" && trackASIN != "" {
			return fmt.Sprintf("%s:%s", albumASIN, trackASIN)
		}
	}
	return ""
}

func extractYoutubeMusicID(link string) string {
	// Expected: https://music.youtube.com/watch?v=<VideoID>
	// Output: <VideoID>
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	return u.Query().Get("v")
}
