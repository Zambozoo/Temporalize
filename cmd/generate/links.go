package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
)

const (
	unitedStatesCountryKey = "US"

	spotifyKey      = "spotify"
	youtubeMusicKey = "youtubeMusic"
	appleMusicKey   = "appleMusic"
	amazonMusicKey  = "amazonMusic"

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

func fetchLinks(client *retryablehttp.Client, spotifyID string) (map[string]string, error) {
	spotifyURI := "spotify:track:" + spotifyID
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

	links := make(map[string]string)

	// Apple Music
	if id, ok := validateAndTrimLink(result.LinksByPlatform, appleMusicKey, appleMusicPrefix, appleMusicInfo, appleMusicSuffix); ok {
		links["appleMusic"] = id
	}

	// Amazon Music
	if id, ok := validateAndTrimLink(result.LinksByPlatform, amazonMusicKey, amazonMusicPrefix, amazonMusicInfix, ""); ok {
		links["amazonMusic"] = id
	}

	// YouTube Music
	if id, ok := validateAndTrimLink(result.LinksByPlatform, youtubeMusicKey, youtubeMusicPrefix, "", ""); ok {
		links["youtubeMusic"] = id
	}

	return links, nil
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
