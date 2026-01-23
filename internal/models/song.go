package models

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// CSV headers
func CSVHeaders() []string {
	return []string{
		"year",
		"genre",
		"popularity",
		"name",
		"explicit",
		"artists",
		"spotify",
		"youtubeMusic",
		"appleMusic",
		"amazonMusic",
		"thumbnailURL",
	}
}

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

func (s *Song) MarshalCSV() []string {
	return []string{
		strconv.Itoa(s.Year),
		s.Genre,
		strconv.Itoa(s.Popularity),
		s.Title,
		strconv.FormatBool(s.Explicit),
		strings.Join(s.Artists, "|"),
		s.Spotify,
		s.YoutubeMusic,
		s.AppleMusic,
		s.AmazonMusic,
		s.ThumbnailURL,
	}
}

func (s *Song) UnmarshalCSV(csvRow []string) error {
	if len(csvRow) != len(CSVHeaders()) {
		return fmt.Errorf("invalid CSV row length: %d", len(csvRow))
	}

	var (
		err        error
		year       int
		popularity int
		explicit   bool
	)
	if year, err = strconv.Atoi(csvRow[0]); err != nil {
		return err
	}
	if popularity, err = strconv.Atoi(csvRow[2]); err != nil {
		return err
	}
	if explicit, err = strconv.ParseBool(csvRow[4]); err != nil {
		return err
	}

	*s = Song{
		Year:         year,
		Genre:        csvRow[1],
		Popularity:   popularity,
		Explicit:     explicit,
		Title:        csvRow[3],
		Artists:      strings.Split(csvRow[5], "|"),
		Spotify:      csvRow[6],
		YoutubeMusic: csvRow[7],
		AppleMusic:   csvRow[8],
		AmazonMusic:  csvRow[9],
		ThumbnailURL: csvRow[10],
	}

	return nil
}

func (s *Song) FileName() string {
	filename := fmt.Sprintf("%d-%s", s.Year, s.Title)

	// Define a regular expression to match invalid characters and replace them with an underscore
	sanitized := strings.TrimSpace(regexp.MustCompile(`[/\?%*:|"<>\x00-\x1F]`).ReplaceAllString(filename, "_"))

	// remove multiple consecutive underscores
	return regexp.MustCompile(`_+`).ReplaceAllString(sanitized, "_")
}
