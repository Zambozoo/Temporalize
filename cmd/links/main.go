package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"

	"temporalize/internal/models"
)

const (
	songsCSV   = "assets/songs.csv"
	outputJSON = "links.json"

	spotifyKey      = "spotify"
	youtubeMusicKey = "youtubeMusic"
	appleMusicKey   = "appleMusic"
	amazonMusicKey  = "amazonMusic"

	unitedStatesCountryKey = "US"

	spotifyPrefix      = "https://open.spotify.com/track/"
	youtubeMusicPrefix = "https://music.youtube.com/watch?v="
	appleMusicPrefix   = "https://geo.music.apple.com/us/album/_/"
	appleMusicInfo     = "?i="
	appleMusicSuffix   = "&mt=1&app=music&ls=1&at=1000lHKX&ct=api_http&itscg=30200&itsct=odsl_m"
	amazonMusicPrefix  = "https://music.amazon.com/albums/"
	amazonMusicInfix   = "?trackAsin="
)

type odesliResponse struct {
	LinksByPlatform map[string]struct {
		URL string `json:"url"`
	} `json:"linksByPlatform"`
	EntitiesByUniqueID map[string]struct {
		Thumbnail string `json:"thumbnailUrl"`
	} `json:"entitiesByUniqueId"`
	Error string `json:"error"`
}

type PlatformData struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	IsInvalid bool   `json:"is_invalid,omitempty"`
}

type SongOutput struct {
	Title        string       `json:"title"`
	Artist       string       `json:"artist"`
	Year         int          `json:"year"`
	Spotify      PlatformData `json:"spotify"`
	AppleMusic   PlatformData `json:"appleMusic"`
	AmazonMusic  PlatformData `json:"amazonMusic"`
	YoutubeMusic PlatformData `json:"youtubeMusic"`
}

func main() {
	// Create retryable client
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 5
	retryClient.Logger = nil // Suppress verbose logs
	// Increase timeout for individual requests
	retryClient.HTTPClient.Timeout = 15 * time.Second

	f, err := os.Open(songsCSV)
	if err != nil {
		log.Fatalf("failed to open songs csv: %v", err)
	}
	defer f.Close()

	r := csv.NewReader(f)

	// Read and check header
	_, err = r.Read() // Skip header
	if err != nil {
		log.Fatalf("failed to read header: %v", err)
	}

	var songs []SongOutput
	errorCounts := map[string]int{
		appleMusicKey:   0,
		amazonMusicKey:  0,
		youtubeMusicKey: 0,
	}

	processedCount := 0
	startTime := time.Now()

	fmt.Println("Starting processing...")

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("failed to read csv record: %v", err)
			continue
		}

		var song models.Song
		if err := song.UnmarshalCSV(record); err != nil {
			log.Printf("Skipping invalid record: %v", err)
			continue
		}

		processedCount++
		if processedCount%10 == 0 {
			fmt.Printf("Processed %d songs... (Time: %v)\n", processedCount, time.Since(startTime).Round(time.Second))
		}

		// Construct Spotify URI
		spotifyURI := "spotify:track:" + song.Spotify

		links, err := fetchOdesliLinks(retryClient, spotifyURI)
		if err != nil {
			log.Printf("Failed to fetch Odesli links for %s: %v", song.Title, err)
			continue
		}

		output := SongOutput{
			Title:  song.Title,
			Artist: song.Artists[0], // Using primary artist
			Year:   song.Year,
		}

		// Spotify (Source of Truth)
		output.Spotify = PlatformData{
			ID:  song.Spotify,
			URL: spotifyPrefix + song.Spotify,
		}

		// Helper to process platform
		processPlatform := func(key, prefix, infix, suffix string, validateFunc func(*retryablehttp.Client, string, string, string) error) (PlatformData, bool) {
			data := PlatformData{}

			// Extract ID from Odesli response
			id, ok := validateAndTrimLink(links.LinksByPlatform, key, prefix, infix, suffix)
			if !ok {
				return data, false // Not found
			}
			data.ID = id

			// Reconstruct Full URL
			fullURL := prefix + strings.ReplaceAll(id, ":", infix) + suffix
			// Special handling for Apple Music infix replacement if needed (logic in validateAndTrimLink replaced ?i= with :)
			if key == appleMusicKey {
				fullURL = prefix + strings.ReplaceAll(id, ":", appleMusicInfo) + suffix
			}
			data.URL = fullURL

			// Validate
			if validateFunc != nil {
				// For Amazon, we need special URL construction for validation
				validationURL := fullURL
				if key == amazonMusicKey {
					parts := strings.Split(id, ":")
					if len(parts) == 2 {
						validationURL = fmt.Sprintf("https://music.amazon.com/embed/%s", parts[1])
					}
				}

				if err := validateFunc(retryClient, validationURL, song.Title, song.Artists[0]); err != nil {
					// log.Printf("[%s] Validation failed for %s: %v", key, song.Title, err)
					data.IsInvalid = true
					return data, true // Invalid
				}
			}

			return data, false // Valid
		}

		// Apple Music
		appleData, appleInvalid := processPlatform(appleMusicKey, appleMusicPrefix, appleMusicInfo, appleMusicSuffix, validateAppleMusic)
		output.AppleMusic = appleData
		if appleInvalid {
			errorCounts[appleMusicKey]++
		}

		// Amazon Music
		amazonData, amazonInvalid := processPlatform(amazonMusicKey, amazonMusicPrefix, amazonMusicInfix, "", validateAmazonMusic)
		output.AmazonMusic = amazonData
		if amazonInvalid {
			errorCounts[amazonMusicKey]++
		}

		// YouTube Music
		youtubeData, youtubeInvalid := processPlatform(youtubeMusicKey, youtubeMusicPrefix, "", "", validateYoutubeMusic)
		output.YoutubeMusic = youtubeData
		if youtubeInvalid {
			errorCounts[youtubeMusicKey]++
		}

		songs = append(songs, output)
	}

	// Write JSON output
	jsonFile, err := os.Create(outputJSON)
	if err != nil {
		log.Fatalf("failed to create output json: %v", err)
	}
	defer jsonFile.Close()

	encoder := json.NewEncoder(jsonFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(songs); err != nil {
		log.Fatalf("failed to encode json: %v", err)
	}

	fmt.Println("\nProcessing Complete!")
	fmt.Printf("Total Songs Processed: %d\n", len(songs))
	fmt.Println("Validation Errors by Platform:")
	for k, v := range errorCounts {
		fmt.Printf("  %s: %d\n", k, v)
	}
	fmt.Printf("Output written to %s\n", outputJSON)
}

func validateYoutubeMusic(client *retryablehttp.Client, url, title, artist string) error {
	return validatePageContent(client, url, title, artist)
}

func validateAmazonMusic(client *retryablehttp.Client, url, title, artist string) error {
	// validationURL is passed directly now
	return validatePageContent(client, url, title, artist)
}

func validateAppleMusic(client *retryablehttp.Client, url, title, artist string) error {
	return validatePageContent(client, url, title, artist)
}

func validatePageContent(client *retryablehttp.Client, url, title, artist string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	body := string(bodyBytes)

	// Simple check: Title and Artist should be in the page
	// Normalize for case-insensitive check
	bodyLower := strings.ToLower(body)
	titleLower := strings.ToLower(title)
	artistLower := strings.ToLower(artist)

	if !strings.Contains(bodyLower, titleLower) {
		return fmt.Errorf("title %q not found in page", title)
	}
	// Artist check might be tricky if "Yusuf / Cat Stevens" is formatted differently
	// Let's try splitting artist if it contains "/"
	artistsToCheck := strings.Split(artistLower, "/")
	foundArtist := false
	for _, a := range artistsToCheck {
		if strings.Contains(bodyLower, strings.TrimSpace(a)) {
			foundArtist = true
			break
		}
	}
	if !foundArtist {
		return fmt.Errorf("artist %q not found in page", artist)
	}

	return nil
}

func fetchOdesliLinks(client *retryablehttp.Client, spotifyURI string) (*odesliResponse, error) {
	apiURL := fmt.Sprintf("https://api.song.link/v1-alpha.1/links?url=%s&userCountry=%s", spotifyURI, unitedStatesCountryKey)

	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("odesli status: %s", http.StatusText(resp.StatusCode))
	}

	var result odesliResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Error != "" {
		return nil, fmt.Errorf("odesli error: %s", result.Error)
	}

	return &result, nil
}

func validateAndTrimLink(links map[string]struct {
	URL string `json:"url"`
}, key, prefix, infix, suffix string) (string, bool) {
	if link, ok := links[key]; ok {
		s := strings.TrimPrefix(strings.TrimSuffix(link.URL, suffix), prefix)
		if infix != "" {
			s = strings.ReplaceAll(s, infix, ":")
		}
		return s, true
	}
	return "", false
}
