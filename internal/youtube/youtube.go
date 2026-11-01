// Package youtube reads a video's title and publish date from the YouTube
// Data API.
package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/alwinihza/discord-bot/models"
)

const apiURL = "https://www.googleapis.com/youtube/v3/videos"

// ErrNotFound means the video is deleted, private or never existed.
var ErrNotFound = errors.New("video not found")

type Client struct {
	key string
	url string
}

func NewClient(key string) *Client {
	return &Client{key: key, url: apiURL}
}

func (c *Client) Fetch(ctx context.Context, id string) (models.VideoInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"?part=snippet&id="+id, nil)
	if err != nil {
		return models.VideoInfo{}, err
	}
	// In a header, not the URL, because Go's HTTP errors quote the URL into logs.
	req.Header.Set("X-Goog-Api-Key", c.key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return models.VideoInfo{}, fmt.Errorf("call YouTube API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return models.VideoInfo{}, fmt.Errorf("call YouTube API: %s", resp.Status)
	}

	var body struct {
		Items []struct {
			Snippet struct {
				Title       string    `json:"title"`
				PublishedAt time.Time `json:"publishedAt"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return models.VideoInfo{}, fmt.Errorf("read YouTube API response: %w", err)
	}
	if len(body.Items) == 0 {
		return models.VideoInfo{}, fmt.Errorf("video %s: %w", id, ErrNotFound)
	}
	snippet := body.Items[0].Snippet
	return models.VideoInfo{
		ID:          id,
		Title:       snippet.Title,
		PublishedAt: snippet.PublishedAt.UTC().Format("2006-01-02 15:04:05") + "+00",
		Thumbnail:   "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg",
	}, nil
}
