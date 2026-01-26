package models

import (
	"fmt"
	"regexp"
	"strings"
)

type Song struct {
	Year         int
	Genre        string
	Title        string
	Artists      []string
	Explicit     bool
	Popularity   int
	Spotify      string
	YoutubeMusic string
	AppleMusic   string
	AmazonMusic  string
	ThumbnailURL string
}

func (s *Song) FileName() string {
	filename := fmt.Sprintf("%d-%s", s.Year, s.Title)

	// Define a regular expression to match invalid characters and replace them with an underscore
	sanitized := strings.TrimSpace(regexp.MustCompile(`[/\?%*:|"<>\x00-\x1F]`).ReplaceAllString(filename, "_"))

	// remove multiple consecutive underscores
	return regexp.MustCompile(`_+`).ReplaceAllString(sanitized, "_")
}

// GeneratedSong represents the output summary for a song from the lookup process
type GeneratedSong struct {
	Explicit     bool     `json:"explicit"`
	Year         int      `json:"year"`
	Artists      []string `json:"artists"`
	Genre        string   `json:"genre"`
	Title        string   `json:"title"`
	ThumbnailURL string   `json:"thumbnail_url"`
	Spotify      string   `json:"spotify"`
	AppleMusic   string   `json:"apple_music"`
	AmazonMusic  string   `json:"amazon_music"`
	YoutubeMusic string   `json:"youtube_music"`
	Invalid      bool     `json:"invalid"`
}
