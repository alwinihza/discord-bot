// Package youtube reads a video's title and publish date from its watch
// page, which needs no API key.
package youtube

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/alwinihza/discord-bot/models"
)

// ErrBlocked means YouTube answered with its bot check instead of the video.
// It comes and goes, so it is worth retrying.
var ErrBlocked = errors.New("youtube bot check")

var errNoVideo = errors.New("no public title or date")

var (
	watchURL    = "https://www.youtube.com/watch?v="
	retryDelays = []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}

	titlePattern = regexp.MustCompile(`<meta name="title" content="([^"]*)"`)
	datePattern  = regexp.MustCompile(`"publishDate":"([^"]+)"`)
)

// Fetch retries everything except a video that plainly has no public page.
func Fetch(ctx context.Context, id string) (models.VideoInfo, error) {
	info, err := fetchOnce(ctx, id)
	for _, delay := range retryDelays {
		if err == nil || errors.Is(err, errNoVideo) {
			break
		}
		select {
		case <-ctx.Done():
			return info, err
		case <-time.After(delay):
		}
		info, err = fetchOnce(ctx, id)
	}
	return info, err
}

func fetchOnce(ctx context.Context, id string) (models.VideoInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, watchURL+id, nil)
	if err != nil {
		return models.VideoInfo{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept-Language", "en")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return models.VideoInfo{}, fmt.Errorf("fetch watch page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return models.VideoInfo{}, fmt.Errorf("fetch watch page: %s", resp.Status)
	}
	page, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return models.VideoInfo{}, fmt.Errorf("read watch page: %w", err)
	}
	return parseWatchPage(id, page)
}

func parseWatchPage(id string, page []byte) (models.VideoInfo, error) {
	if bytes.Contains(page, []byte("confirm you’re not a bot")) {
		return models.VideoInfo{}, ErrBlocked
	}
	title := titlePattern.FindSubmatch(page)
	date := datePattern.FindSubmatch(page)
	if title == nil || len(title[1]) == 0 || date == nil {
		return models.VideoInfo{}, fmt.Errorf("video %s: %w", id, errNoVideo)
	}
	published, err := time.Parse(time.RFC3339, string(date[1]))
	if err != nil {
		return models.VideoInfo{}, fmt.Errorf("parse publish date: %w", err)
	}
	return models.VideoInfo{
		ID:          id,
		Title:       html.UnescapeString(string(title[1])),
		PublishedAt: published.UTC().Format("2006-01-02 15:04:05") + "+00",
		Thumbnail:   "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg",
	}, nil
}
