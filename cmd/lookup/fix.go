package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"temporalize/internal/models"

	"github.com/hashicorp/go-retryablehttp"
	xhtml "golang.org/x/net/html"
)

const (
	appleSearchAPI = "https://itunes.apple.com/search"
	youtubeSearch  = "https://www.youtube.com/results"
	amazonSearch   = "https://www.amazon.com/s"
)

var (
	cleanPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s-\s.*Remaster.*`),
		regexp.MustCompile(`(?i)\s\(.*Remaster.*\)`),
		regexp.MustCompile(`(?i)\s-\s.*Mix.*`),
		regexp.MustCompile(`(?i)\s\(.*Mix.*\)`),
		regexp.MustCompile(`(?i)\s-\s.*Radio.*`),
		regexp.MustCompile(`(?i)\s\(.*Radio.*\)`),
		regexp.MustCompile(`(?i)\s-\s.*Live.*`),
		regexp.MustCompile(`(?i)\s\(.*Live.*\)`),
		regexp.MustCompile(`(?i)\s-\s.*Mono.*`),
		regexp.MustCompile(`(?i)\s\(.*Mono.*\)`),
		regexp.MustCompile(`(?i)\s-\s.*Stereo.*`),
		regexp.MustCompile(`(?i)\s\(.*Stereo.*\)`),
		regexp.MustCompile(`(?i)\s-\s.*Feat.*`),
		regexp.MustCompile(`(?i)\s\(.*Feat.*\)`),
		regexp.MustCompile(`(?i)\s-\s.*Ft\..*`),
		regexp.MustCompile(`(?i)\s\(.*Ft\..*\)`),
		regexp.MustCompile(`(?i)\s-\s.*Version\..*`),
		regexp.MustCompile(`(?i)\s\(.*Version\..*\)`),
	}

	errNoResults = errors.New("no results")
)

func cleanTitle(title string) string {
	newTitle := title
	for _, re := range cleanPatterns {
		newTitle = re.ReplaceAllString(newTitle, "")
	}
	return strings.TrimSpace(newTitle)
}

func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "&", "and")
	return s
}

func fixLinks(client *retryablehttp.Client, song *models.Song) bool {
	isValid := true
	// Apple Music
	shouldFix := true
	if parts := strings.Split(song.AppleMusic, ":"); len(parts) == 2 {
		url := fmt.Sprintf("https://music.apple.com/us/album/_/%s?i=%s", parts[0], parts[1])
		if err := validatePageContent(client, url, song.Title, song.Artists[0]); err == nil {
			shouldFix = false
		}
	}
	if shouldFix {
		if err := fixAppleMusic(client, song); err != nil {
			isValid = false
			fmt.Printf("Failed to fix Apple Music link for %s\n", song.Title)
		}
	}

	shouldFix = true
	// Amazon Music
	if parts := strings.Split(song.AmazonMusic, ":"); len(parts) == 2 {
		url := fmt.Sprintf("https://music.amazon.com/embed/%s", parts[1])
		if err := validatePageContent(client, url, song.Title, song.Artists[0]); err == nil {
			shouldFix = false
		}
	}
	if shouldFix {
		if err := fixAmazonMusic(client, song); err != nil {
			isValid = false
			fmt.Printf("Failed to fix Amazon Music link for %s\n", song.Title)
		}
	}

	// YouTube Music
	shouldFix = true
	if parts := strings.Split(song.YoutubeMusic, ":"); len(parts) == 2 {
		url := "https://music.youtube.com/watch?v=" + parts[1]
		if err := validateYoutubeViaOEmbed(client, url, song.Title, song.Artists[0]); err == nil {
			shouldFix = false
		}
	}
	if shouldFix {
		if err := fixYoutubeMusic(client, song); err != nil {
			isValid = false
			fmt.Printf("Failed to fix YouTube Music link for %s\n", song.Title)
		}
	}

	return isValid
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

	bodyUnescaped := html.UnescapeString(body)
	bodyLower := normalize(bodyUnescaped)
	titleLower := normalize(title)
	artistLower := normalize(artist)

	if !strings.Contains(bodyLower, titleLower) {
		return fmt.Errorf("title %q not found", title)
	}

	artistsToCheck := strings.Split(artistLower, "/")
	foundArtist := false
	for _, a := range artistsToCheck {
		cleanA := strings.TrimSpace(a)
		if len(cleanA) < 2 {
			continue
		}
		if strings.Contains(bodyLower, cleanA) {
			foundArtist = true
			break
		}
	}
	if !foundArtist {
		return fmt.Errorf("artist %q not found", artist)
	}

	return nil
}

// --- Fixers ---

func fixAppleMusic(client *retryablehttp.Client, song *models.Song) error {
	cleanTitleVal := cleanTitle(song.Title)
	candidates, err := searchAppleMusic(client, cleanTitleVal, song.Artists[0])
	if err != nil {
		return err
	}

	for _, newLink := range candidates {
		if err := validatePageContent(client, newLink, cleanTitleVal, song.Artists[0]); err == nil {
			song.AppleMusic = extractAppleIDs(newLink)
			return nil
		}
	}
	return errNoResults
}

func fixAmazonMusic(client *retryablehttp.Client, song *models.Song) error {
	cleanTitleVal := cleanTitle(song.Title)
	candidates, err := searchAmazonMusic(client, cleanTitleVal, song.Artists[0])
	if err != nil {
		return err
	}

	for _, newLink := range candidates {
		parts := strings.Split(newLink, "/")
		if len(parts) < 2 {
			continue
		}
		asin := parts[len(parts)-1]
		embedURL := fmt.Sprintf("https://music.amazon.com/embed/%s", asin)

		if err := validatePageContent(client, embedURL, cleanTitleVal, song.Artists[0]); err == nil {
			song.AmazonMusic = fmt.Sprintf("%s:%s", asin, asin)
			return nil
		}
	}

	return errNoResults
}

func fixYoutubeMusic(client *retryablehttp.Client, song *models.Song) error {
	cleanTitleVal := cleanTitle(song.Title)
	videoIDs, err := searchYoutube(client, cleanTitleVal, song.Artists[0])
	if err != nil {
		return err
	}

	for _, videoID := range videoIDs {
		newLink := "https://music.youtube.com/watch?v=" + videoID
		if err := validateYoutubeViaOEmbed(client, newLink, cleanTitleVal, song.Artists[0]); err == nil {
			song.YoutubeMusic = videoID
			return nil
		}
	}
	return errNoResults
}

// --- Helpers ---

type iTunesResponse struct {
	Results []struct {
		TrackViewUrl string `json:"trackViewUrl"`
	} `json:"results"`
}

func searchAppleMusic(client *retryablehttp.Client, title, artist string) ([]string, error) {
	term := fmt.Sprintf("%s %s", title, artist)
	u, _ := url.Parse(appleSearchAPI)
	q := u.Query()
	q.Set("term", term)
	q.Set("country", "US")
	q.Set("media", "music")
	q.Set("entity", "song")
	q.Set("limit", "5")
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var result iTunesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var links []string
	for _, item := range result.Results {
		if item.TrackViewUrl != "" {
			links = append(links, item.TrackViewUrl)
		}
	}
	if len(links) > 0 {
		return links, nil
	}
	return nil, errNoResults
}

func extractAppleIDs(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	trackID := u.Query().Get("i")
	albumID := ""

	re := regexp.MustCompile(`/(\d+)\?`)
	matches := re.FindStringSubmatch(link)
	if len(matches) > 1 {
		albumID = matches[1]
	}

	if albumID != "" && trackID != "" {
		return fmt.Sprintf("%s:%s", albumID, trackID)
	}
	return ""
}

func searchAmazonMusic(client *retryablehttp.Client, title, artist string) ([]string, error) {
	term := fmt.Sprintf("%s %s", title, artist)
	u, _ := url.Parse(amazonSearch)
	q := u.Query()
	q.Set("k", term)
	q.Set("i", "digital-music")
	u.RawQuery = q.Encode()

	req, _ := retryablehttp.NewRequest("GET", u.String(), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	doc, err := xhtml.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var links []string
	var f func(*xhtml.Node)
	f = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "div" {
			isSearchResult := false
			asin := ""
			for _, a := range n.Attr {
				if a.Key == "data-component-type" && a.Val == "s-search-result" {
					isSearchResult = true
				}
				if a.Key == "data-asin" {
					asin = a.Val
				}
			}
			if isSearchResult && asin != "" {
				links = append(links, fmt.Sprintf("https://music.amazon.com/tracks/%s", asin))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	if len(links) > 0 {
		return links, nil
	}
	return nil, errNoResults
}

func searchYoutube(client *retryablehttp.Client, title, artist string) ([]string, error) {
	term := fmt.Sprintf("%s %s audio", title, artist)
	u, _ := url.Parse(youtubeSearch)
	q := u.Query()
	q.Set("search_query", term)
	u.RawQuery = q.Encode()

	req, _ := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	body := string(bodyBytes)

	re := regexp.MustCompile(`"videoId":"([a-zA-Z0-9_-]{11})"`)
	matches := re.FindAllStringSubmatch(body, 10)

	var ids []string
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			id := match[1]
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}

	if len(ids) > 0 {
		return ids, nil
	}
	return nil, errNoResults
}

func validateYoutubeViaOEmbed(client *retryablehttp.Client, url, title, artist string) error {
	oembedURL := fmt.Sprintf("https://www.youtube.com/oembed?url=%s&format=json", url)
	resp, err := client.Get(oembedURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	var result struct {
		Title      string `json:"title"`
		AuthorName string `json:"author_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	normTitle := normalize(result.Title)
	normAuthor := normalize(result.AuthorName)
	expectedTitle := normalize(title)
	expectedArtist := normalize(artist)

	if !strings.Contains(normTitle, expectedTitle) {
		return fmt.Errorf("title mismatch")
	}

	artistsToCheck := strings.Split(expectedArtist, "/")
	foundArtist := false
	for _, a := range artistsToCheck {
		cleanA := strings.TrimSpace(a)
		if strings.Contains(normAuthor, cleanA) || strings.Contains(normTitle, cleanA) {
			foundArtist = true
			break
		}
	}

	if !foundArtist {
		return fmt.Errorf("artist mismatch")
	}
	return nil
}
