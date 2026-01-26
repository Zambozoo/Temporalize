package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"temporalize/internal/models"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
)

var (
	spotifyClientID     = os.Getenv("SPOTIFY_CLIENT_ID")
	spotifyClientSecret = os.Getenv("SPOTIFY_CLIENT_SECRET")
)

// CollectedSong matches the output structure of cmd/collect
type CollectedSong struct {
	URL   string `json:"url"`
	Genre string `json:"genre"`
	Year  int    `json:"year"`
}

func main() {
	inputFile := flag.String("input", "collect.json", "Path to input JSON file")
	summaryFile := flag.String("summary", "lookup.json", "Output JSON file for generated songs summary")
	startYear := flag.Int("start", 1970, "Start year (inclusive)")
	endYear := flag.Int("end", 2025, "End year (inclusive)")
	flag.Parse()

	if err := run(*inputFile, *summaryFile, *startYear, *endYear); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(inputFile, summaryFile string, startYear, endYear int) error {
	if spotifyClientID == "" || spotifyClientSecret == "" {
		return fmt.Errorf("SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET environment variables must be set")
	}

	// 1. Setup Clients
	ctx := context.Background()
	spotifyClient, err := setupSpotifyClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup spotify client: %w", err)
	}

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 5
	retryClient.Logger = nil
	retryClient.HTTPClient.Timeout = 15 * time.Second

	// 2. Read Input
	songs, err := readInputLinks(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	fmt.Printf("Loaded %d songs from %s\n", len(songs), inputFile)

	// Open summary file for streaming
	fSummary, err := os.Create(summaryFile)
	if err != nil {
		return fmt.Errorf("failed to create summary file: %w", err)
	}
	defer fSummary.Close()

	// Write start of JSON array
	if _, err := fSummary.WriteString("[\n"); err != nil {
		return err
	}

	encoder := json.NewEncoder(fSummary)
	encoder.SetIndent("  ", "  ")

	firstItem := true

	// 3. Process Each Link
	currentYear := 0
	for _, songInput := range songs {
		if songInput.Year < startYear || songInput.Year > endYear {
			continue
		}

		if songInput.Year != 0 && songInput.Year != currentYear {
			currentYear = songInput.Year
			fmt.Printf("Processing Year %d...\n", currentYear)
		}

		// A. Parse Spotify ID
		spotifyID := parseSpotifyID(songInput.URL)
		if spotifyID == "" {
			continue
		}

		// B. Fetch Metadata (Spotify)
		// We pass the collected genre to fetchMetadata
		song, err := fetchMetadata(ctx, spotifyClient, spotifyID, songInput.Genre)
		if err != nil {
			log.Printf("Failed to fetch metadata for %s: %v", songInput.URL, err)
			continue
		}

		// Clean the title before using it
		song.Title = cleanTitle(song.Title)

		// C. Fetch Thumbnail
		if err := fetchThumbnail(retryClient, song); err != nil {
			log.Printf("Failed to fetch thumbnail for %s: %v", song.Title, err)
		}

		// D. Fetch Other Links (Odesli)
		linksMap, err := fetchLinks(retryClient, spotifyID)
		if err != nil {
			log.Printf("Failed to fetch links for %s: %v", song.Title, err)
			continue
		}

		// E. Validate & Fix Links
		// Map Odesli links to our Song struct fields
		song.AppleMusic = linksMap["appleMusic"]
		song.AmazonMusic = linksMap["amazonMusic"]
		song.YoutubeMusic = linksMap["youtubeMusic"]
		song.Spotify = spotifyID // Ensure ID is set

		// Fix logic (simplified version of cmd/fix/main.go)
		// fixLinks modifies the song object in place
		isValid := fixLinks(retryClient, song)

		// Construct output object
		genSong := models.GeneratedSong{
			Explicit:     song.Explicit,
			Year:         song.Year,
			Artists:      song.Artists,
			Genre:        song.Genre,
			Title:        song.Title,
			ThumbnailURL: song.ThumbnailURL,
			Spotify:      "https://open.spotify.com/track/" + song.Spotify,
			AppleMusic:   "",
			AmazonMusic:  "",
			YoutubeMusic: "",
			Invalid:      !isValid,
		}

		if song.AppleMusic != "" {
			parts := strings.Split(song.AppleMusic, ":")
			if len(parts) == 2 {
				genSong.AppleMusic = fmt.Sprintf("https://music.apple.com/us/album/_/%s?i=%s", parts[0], parts[1])
			}
		}
		if song.AmazonMusic != "" {
			parts := strings.Split(song.AmazonMusic, ":")
			if len(parts) == 2 {
				genSong.AmazonMusic = fmt.Sprintf("https://music.amazon.com/albums/%s?trackAsin=%s", parts[0], parts[1])
			}
		}
		if song.YoutubeMusic != "" {
			genSong.YoutubeMusic = "https://music.youtube.com/watch?v=" + song.YoutubeMusic
		}

		// Write to summary
		if !firstItem {
			if _, err := fSummary.WriteString(",\n"); err != nil {
				return err
			}
		}
		if err := encoder.Encode(genSong); err != nil {
			return err
		}
		firstItem = false

		// Sleep briefly to be nice to APIs (Odesli rate limits)
		time.Sleep(200 * time.Millisecond)
	}

	// Close JSON array
	if _, err := fSummary.WriteString("\n]"); err != nil {
		return err
	}

	return nil
}

func setupSpotifyClient(ctx context.Context) (*spotify.Client, error) {
	config := &clientcredentials.Config{
		ClientID:     spotifyClientID,
		ClientSecret: spotifyClientSecret,
		TokenURL:     spotifyauth.TokenURL,
	}
	httpClient := config.Client(ctx)
	return spotify.New(httpClient, spotify.WithRetry(true)), nil
}

func readInputLinks(path string) ([]CollectedSong, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var songs []CollectedSong

	// Try to decode as []CollectedSong
	if err := json.NewDecoder(f).Decode(&songs); err != nil {
		return nil, err
	}

	return songs, nil
}

func parseSpotifyID(link string) string {
	// Handle URL: https://open.spotify.com/track/ID?si=...
	// Handle URI: spotify:track:ID
	if strings.HasPrefix(link, "spotify:track:") {
		return strings.TrimPrefix(link, "spotify:track:")
	}
	if strings.Contains(link, "/track/") {
		parts := strings.Split(link, "/track/")
		if len(parts) > 1 {
			idPart := parts[1]
			// Remove query params
			if idx := strings.Index(idPart, "?"); idx != -1 {
				return idPart[:idx]
			}
			return idPart
		}
	}
	// Assume it might be just the ID if alphanumeric and length 22
	if len(link) == 22 {
		return link
	}
	return ""
}
