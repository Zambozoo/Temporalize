package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	minPopularity        = 40
	maxTracksPerCategory = 10

	defaultStartYear  = 1970
	defaultEndYear    = 2025
	defaultOutputFile = "collect.json"
)

var (
	spotifyClientID     = os.Getenv("SPOTIFY_CLIENT_ID")
	spotifyClientSecret = os.Getenv("SPOTIFY_CLIENT_SECRET")

	ErrMissingEnvVars = errors.New("missing SPOTIFY_CLIENT_ID or SPOTIFY_CLIENT_SECRET environment variables")
)

type CollectedSong struct {
	URL   string `json:"url"`
	Genre string `json:"genre"`
	Year  int    `json:"year"`
}

func main() {
	outputFile := flag.String("output", defaultOutputFile, "Output JSON file")
	startYear := flag.Int("start", defaultStartYear, "Start year")
	endYear := flag.Int("end", defaultEndYear, "End year")
	flag.Parse()

	if err := run(*outputFile, *startYear, *endYear); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(outputFile string, startYear, endYear int) error {
	if spotifyClientID == "" || spotifyClientSecret == "" {
		return ErrMissingEnvVars
	}

	ctx := context.Background()
	client, err := setupSpotifyClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup spotify client: %w", err)
	}

	uniqueLinks := make(map[string]bool)

	// Define genre groups to search
	// We search for specific terms to ensure we get a good mix of songs
	genreGroups := map[string][]string{
		"pop":     {"pop", "dance pop", "electropop", "synthpop", "r&b", "soul", "disco"},
		"rock":    {"rock", "hard rock", "classic rock", "alternative rock", "punk", "metal", "indie"},
		"hip-hop": {"hip hop", "rap", "trap"},
		"country": {"country", "folk", "americana", "bluegrass"},
		"jazz":    {"jazz", "blues", "funk"},
	}

	// Sort keys for deterministic iteration
	var genreKeys []string
	for k := range genreGroups {
		genreKeys = append(genreKeys, k)
	}
	sort.Strings(genreKeys)

	// Open output file in append mode or create if not exists
	// Actually, streaming JSON array is tricky if we want valid JSON at all times.
	// But if we just want to write as we go, we can open the file once and write to it.
	// However, standard JSON requires the whole array to be in memory or carefully managed commas.
	// Let's stick to accumulating in memory for now unless memory is an issue (it's not for <10k items).
	// The user asked "Can we stream writing to output files?".
	// To truly stream, we should open the file at the start, write "[", and then append items.

	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString("[\n"); err != nil {
		return err
	}

	encoder := json.NewEncoder(f)
	encoder.SetIndent("  ", "  ")

	firstItem := true

	for year := startYear; year <= endYear; year++ {
		fmt.Printf("Collecting songs for %d...\n", year)

		// Collect candidates for this year
		// Map URL -> Genre (first one wins)
		yearSongs := make(map[string]string)

		for _, group := range genreKeys {
			subgenres := genreGroups[group]
			links, err := getTopSongs(ctx, client, year, subgenres)
			if err != nil {
				log.Printf("Failed to get songs for %d (group %s): %v", year, group, err)
				continue
			}
			for _, link := range links {
				if _, exists := yearSongs[link]; !exists {
					yearSongs[link] = group
				}
			}
		}

		// Add unique new links to the master list
		countAdded := 0
		// We want to add them in a deterministic order if possible, or just iterate.
		// Since map iteration is random, let's sort the URLs we found this year to be stable.
		var yearURLs []string
		for link := range yearSongs {
			yearURLs = append(yearURLs, link)
		}
		sort.Strings(yearURLs)

		for _, link := range yearURLs {
			if !uniqueLinks[link] {
				uniqueLinks[link] = true
				genre := yearSongs[link]
				song := CollectedSong{URL: link, Genre: genre, Year: year}

				// Write to file immediately
				if !firstItem {
					if _, err := f.WriteString(",\n"); err != nil {
						return err
					}
				}
				if err := encoder.Encode(song); err != nil {
					return err
				}
				firstItem = false

				countAdded++
			}
		}
		fmt.Printf("  -> Added %d unique songs for %d\n", countAdded, year)
	}

	// Write closing bracket
	if _, err := f.WriteString("]"); err != nil {
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
	// Enable retry logic in the Spotify client if possible, or we rely on the underlying transport
	// zmb3/spotify/v2 has built-in retry if configured
	return spotify.New(httpClient, spotify.WithRetry(true)), nil
}

func getTopSongs(ctx context.Context, client *spotify.Client, year int, genres []string) ([]string, error) {
	trackIDs := make(map[spotify.ID]spotify.FullTrack)

	for _, genre := range genres {
		query := fmt.Sprintf("genre:%q year:%d", genre, year)
		// Fetch up to 200 tracks per genre (4 pages of 50)
		for offset := 0; offset < 500; offset += 50 {
			results, err := client.Search(ctx, query, spotify.SearchTypeTrack, spotify.Limit(50), spotify.Offset(offset))
			if err != nil {
				log.Printf("Error searching Spotify for genre %q year %d (offset %d): %v", genre, year, offset, err)
				continue
			}

			for _, item := range results.Tracks.Tracks {
				if item.Popularity >= minPopularity {
					trackIDs[item.ID] = item
				}
			}
			if len(results.Tracks.Tracks) < 50 { // Less than a full page, no more results
				break
			}
		}
	}

	// Convert map to slice for sorting
	var tracks []spotify.FullTrack
	for _, track := range trackIDs {
		tracks = append(tracks, track)
	}

	// Sort by popularity descending
	sort.Slice(tracks, func(i, j int) bool {
		return tracks[i].Popularity > tracks[j].Popularity
	})

	// Take top N unique tracks
	var topSpotifyLinks []string
	for i, track := range tracks {
		if i >= maxTracksPerCategory {
			break
		}
		// Use ExternalURLs["spotify"] if available, otherwise construct URI
		link := track.ExternalURLs["spotify"]
		if link == "" {
			link = "https://open.spotify.com/track/" + string(track.ID)
		}
		topSpotifyLinks = append(topSpotifyLinks, link)
	}

	return topSpotifyLinks, nil
}
