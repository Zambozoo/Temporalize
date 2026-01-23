package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"temporalize/internal/models"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/zmb3/spotify/v2"
)

const (
	thumbnailDir = "assets/thumbnails"
)

func fetchMetadata(ctx context.Context, client *spotify.Client, spotifyID string) (*models.Song, error) {
	track, err := client.GetTrack(ctx, spotify.ID(spotifyID))
	if err != nil {
		return nil, err
	}

	// Get Year
	year := 0
	if len(track.Album.ReleaseDate) >= 4 {
		year, _ = strconv.Atoi(track.Album.ReleaseDate[:4])
	}

	// Get Artists
	var artists []string
	for _, a := range track.Artists {
		artists = append(artists, a.Name)
	}

	// Get Genre (from primary artist)
	genre := "pop" // Default
	if len(track.Artists) > 0 {
		artist, err := client.GetArtist(ctx, track.Artists[0].ID)
		if err == nil && len(artist.Genres) > 0 {
			genre = artist.Genres[0]
		}
	}

	// Get Thumbnail (Largest)
	thumbnailURL := ""
	if len(track.Album.Images) > 0 {
		thumbnailURL = track.Album.Images[0].URL
	}

	return &models.Song{
		Title:        track.Name,
		Artists:      artists,
		Year:         year,
		Explicit:     track.Explicit,
		Popularity:   int(track.Popularity),
		Genre:        genre,
		Spotify:      spotifyID,
		ThumbnailURL: thumbnailURL,
	}, nil
}

func fetchThumbnail(client *retryablehttp.Client, s *models.Song) error {
	filename := fmt.Sprintf("%s/%s.jpeg", thumbnailDir, s.FileName())

	// Ensure directory exists
	if err := os.MkdirAll(thumbnailDir, 0755); err != nil {
		return err
	}

	if _, err := os.Stat(filename); err == nil {
		return nil // Already exists
	}

	resp, err := client.Get(s.ThumbnailURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}
