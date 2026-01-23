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

	"github.com/hashicorp/go-retryablehttp"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
)

var (
	spotifyClientID     = os.Getenv("SPOTIFY_CLIENT_ID")
	spotifyClientSecret = os.Getenv("SPOTIFY_CLIENT_SECRET")
)

type Config struct {
	InputFile  string
	OutputDir  string
	RetryCount int
}

func main() {
	inputFile := flag.String("input", "spotify_links.json", "Path to JSON file containing Spotify links")
	outputDir := flag.String("output", "assets/generated", "Output directory for generated assets")
	flag.Parse()

	if err := run(*inputFile, *outputDir); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(inputFile, outputDir string) error {
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
	links, err := readInputLinks(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	fmt.Printf("Loaded %d links from %s\n", len(links), inputFile)

	// 3. Process Each Link
	for i, link := range links {
		fmt.Printf("[%d/%d] Processing %s...\n", i+1, len(links), link)

		// A. Parse Spotify ID
		spotifyID := parseSpotifyID(link)
		if spotifyID == "" {
			log.Printf("  -> Invalid Spotify link: %s", link)
			continue
		}

		// B. Fetch Metadata (Spotify)
		song, err := fetchMetadata(ctx, spotifyClient, spotifyID)
		if err != nil {
			log.Printf("  -> Failed to fetch metadata: %v", err)
			continue
		}
		fmt.Printf("  -> Metadata: %s - %s (%d)\n", song.Title, song.Artists[0], song.Year)

		// C. Fetch Thumbnail
		if err := fetchThumbnail(retryClient, song); err != nil {
			log.Printf("  -> Failed to fetch thumbnail: %v", err)
			// Continue? Or fail? Let's continue but maybe skip image generation if critical
		} else {
			fmt.Println("  -> Thumbnail fetched")
		}

		// D. Fetch Other Links (Odesli)
		linksMap, err := fetchLinks(retryClient, spotifyID)
		if err != nil {
			log.Printf("  -> Failed to fetch links: %v", err)
			continue
		}

		// E. Validate & Fix Links
		// Map Odesli links to our Song struct fields
		song.AppleMusic = linksMap["appleMusic"]
		song.AmazonMusic = linksMap["amazonMusic"]
		song.YoutubeMusic = linksMap["youtubeMusic"]
		song.Spotify = spotifyID // Ensure ID is set

		// Fix logic (simplified version of cmd/fix/main.go)
		fixLinks(retryClient, song)

		// F. Generate Assets
		// 1. QR Code
		if err := generateQRCode(song, outputDir); err != nil {
			log.Printf("  -> Failed to generate QR code: %v", err)
		} else {
			fmt.Println("  -> QR Code generated")
		}

		// 2. Card Front
		if err := generateCardFront(song, outputDir); err != nil {
			log.Printf("  -> Failed to generate Card Front: %v", err)
		} else {
			fmt.Println("  -> Card Front generated")
		}

		// 3. Card Back
		if err := generateCardBack(song, outputDir); err != nil {
			log.Printf("  -> Failed to generate Card Back: %v", err)
		} else {
			fmt.Println("  -> Card Back generated")
		}
	}

	return nil
}

func setupSpotifyClient(ctx context.Context) (*spotify.Client, error) {
	config := &clientcredentials.Config{
		ClientID:     spotifyClientID,
		ClientSecret: spotifyClientSecret,
		TokenURL:     spotifyauth.TokenURL,
	}
	token, err := config.Token(ctx)
	if err != nil {
		return nil, err
	}
	httpClient := spotifyauth.New().Client(ctx, token)
	return spotify.New(httpClient), nil
}

func readInputLinks(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var links []string
	if err := json.NewDecoder(f).Decode(&links); err != nil {
		return nil, err
	}
	return links, nil
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
