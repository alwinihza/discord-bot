// Package youtube reads a video's title and publish date, and finds a
// channel's live broadcast, from the YouTube Data API.
package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/alwinihza/discord-bot/models"
)

const (
	apiURL    = "https://www.googleapis.com/youtube/v3/videos"
	searchURL = "https://www.googleapis.com/youtube/v3/search"
)

// ErrNotFound means the video is deleted, private or never existed.
var ErrNotFound = errors.New("video not found")

type Client struct {
	key       string
	url       string
	searchURL string
}

func NewClient(key string) *Client {
	return &Client{key: key, url: apiURL, searchURL: searchURL}
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

// ActiveLiveVideo returns the id of the channel's broadcast that is live now.
// found is false when the channel is not live.
func (c *Client) ActiveLiveVideo(ctx context.Context, channelID string) (videoID string, found bool, err error) {
	q := url.Values{"part": {"id"}, "channelId": {channelID}, "eventType": {"live"}, "type": {"video"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.searchURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("X-Goog-Api-Key", c.key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("call YouTube search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("call YouTube search: %s", resp.Status)
	}

	var body struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", false, fmt.Errorf("read YouTube search response: %w", err)
	}
	for _, item := range body.Items {
		if item.ID.VideoID != "" {
			return item.ID.VideoID, true, nil
		}
	}
	return "", false, nil
}
