package site

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("playlist not found")
	ErrForbidden = errors.New("site rejected the admin secret")
	ErrInvalidID = errors.New("invalid playlist id")
)

type Summary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	EntryCount int    `json:"entryCount"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	Deleted    bool   `json:"deleted"`
}

type Client struct {
	base   string
	secret string
	http   *http.Client
}

func NewClient(baseURL, secret string) *Client {
	return &Client{
		base:   strings.TrimRight(baseURL, "/"),
		secret: secret,
		http: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *Client) Info(ctx context.Context, id string) (Summary, error) {
	if !bareID.MatchString(id) {
		return Summary{}, ErrInvalidID
	}
	return c.do(ctx, http.MethodGet, "/admin/playlists/"+id)
}

func (c *Client) Delete(ctx context.Context, id string) (Summary, error) {
	if !bareID.MatchString(id) {
		return Summary{}, ErrInvalidID
	}
	return c.do(ctx, http.MethodDelete, "/admin/playlists/"+id)
}

func (c *Client) do(ctx context.Context, method, path string) (Summary, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, nil)
	if err != nil {
		return Summary{}, errors.New("build site request")
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	resp, err := c.http.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return Summary{}, fmt.Errorf("call site: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Summary{}, ErrNotFound
	case http.StatusForbidden:
		return Summary{}, ErrForbidden
	default:
		return Summary{}, fmt.Errorf("call site: %s", resp.Status)
	}
	var s Summary
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&s); err != nil {
		return Summary{}, fmt.Errorf("read site response: %w", err)
	}
	return s, nil
}

var bareID = regexp.MustCompile(`^[0-9A-Za-z]{8}$`)

func PlaylistID(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if bareID.MatchString(input) {
		return input, true
	}
	u, err := url.Parse(input)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	id, ok := strings.CutPrefix(strings.TrimRight(u.Path, "/"), "/p/")
	if !ok || !bareID.MatchString(id) {
		return "", false
	}
	return id, true
}
