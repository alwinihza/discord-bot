package site

import (
	"bytes"
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
	ErrNotFound    = errors.New("playlist not found")
	ErrForbidden   = errors.New("site rejected the admin secret")
	ErrInvalidID   = errors.New("invalid playlist id")
	ErrInvalidUser = errors.New("invalid Discord user id")
	ErrNotOwner    = errors.New("playlist belongs to someone else")
	ErrDeleted     = errors.New("playlist was deleted")
	ErrConflict    = errors.New("playlist changed meanwhile")
	ErrRateLimited = errors.New("site rate limit reached")
	ErrDailyCap    = errors.New("site reached its daily playlist cap")
	ErrNoOwnerAPI  = errors.New("site has no owner routes yet")
)

// RuleError carries the site's own message for a request it refused with 400.
type RuleError struct{ Message string }

func (e *RuleError) Error() string { return "site refused the change: " + e.Message }

type Summary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	EntryCount int    `json:"entryCount"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	Deleted    bool   `json:"deleted"`
}

type Entry struct {
	SongID string `json:"songId"`
	Title  string `json:"title"`
}

type Playlist struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Entries []Entry `json:"entries"`
	Version int     `json:"version"`
}

type Client struct {
	base      string
	secret    string
	publicURL string
	http      *http.Client
}

// NewClient talks to the admin listener at baseURL. publicURL is the site address fans open, for ViewLink.
func NewClient(baseURL, secret, publicURL string) *Client {
	return &Client{
		base:      strings.TrimRight(baseURL, "/"),
		secret:    secret,
		publicURL: strings.TrimRight(publicURL, "/"),
		http: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *Client) ViewLink(id string) string { return c.publicURL + "/p/" + id }

func (c *Client) Info(ctx context.Context, id string) (Summary, error) {
	var s Summary
	return s, c.call(ctx, http.MethodGet, "", "/admin/playlists/", []string{id}, nil, &s)
}

func (c *Client) Delete(ctx context.Context, id string) (Summary, error) {
	var s Summary
	return s, c.call(ctx, http.MethodDelete, "", "/admin/playlists/", []string{id}, nil, &s)
}

const ownerPath = "/admin/owner/playlists"

func (c *Client) Mine(ctx context.Context, user string) ([]Summary, error) {
	var out struct {
		Playlists []Summary `json:"playlists"`
	}
	err := c.call(ctx, http.MethodGet, user, ownerPath, nil, nil, &out)
	return out.Playlists, err
}

func (c *Client) Create(ctx context.Context, user, name, songID string) (Playlist, error) {
	var p Playlist
	body := map[string]any{"name": name, "songIds": []string{songID}}
	return p, c.call(ctx, http.MethodPost, user, ownerPath, nil, body, &p)
}

func (c *Client) Get(ctx context.Context, user, id string) (Playlist, error) {
	var p Playlist
	return p, c.call(ctx, http.MethodGet, user, ownerPath+"/", []string{id}, nil, &p)
}

func (c *Client) Rename(ctx context.Context, user, id, name string) (Playlist, error) {
	var p Playlist
	return p, c.call(ctx, http.MethodPut, user, ownerPath+"/", []string{id, "name"}, map[string]string{"name": name}, &p)
}

func (c *Client) AddSong(ctx context.Context, user, id, songID string) (Playlist, error) {
	var p Playlist
	return p, c.call(ctx, http.MethodPost, user, ownerPath+"/", []string{id, "songs"}, map[string]string{"songId": songID}, &p)
}

func (c *Client) RemoveSong(ctx context.Context, user, id, songID string) (Playlist, error) {
	var p Playlist
	return p, c.call(ctx, http.MethodDelete, user, ownerPath+"/", []string{id, "songs", songID}, nil, &p)
}

func (c *Client) Remove(ctx context.Context, user, id string) error {
	return c.call(ctx, http.MethodDelete, user, ownerPath+"/", []string{id}, nil, nil)
}

var (
	bareID    = regexp.MustCompile(`^[0-9A-Za-z]{8}$`)
	discordID = regexp.MustCompile(`^[0-9]{17,20}$`)
)

// call sends one request. Owner routes require user, a Discord snowflake sent as the acting account.
// segments[0] is a playlist id, checked before any request leaves.
func (c *Client) call(ctx context.Context, method, user, prefix string, segments []string, in, out any) error {
	if (user != "" || strings.HasPrefix(prefix, ownerPath)) && !discordID.MatchString(user) {
		return ErrInvalidUser
	}
	path := prefix
	for n, seg := range segments {
		if seg == "" || (n == 0 && !bareID.MatchString(seg)) {
			return ErrInvalidID
		}
		if n > 0 {
			path += "/"
		}
		path += url.PathEscape(seg)
	}
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return errors.New("build site request")
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return errors.New("build site request")
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	if user != "" {
		req.Header.Set("X-Acting-Discord-Id", user)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("call site: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read site response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return statusError(resp, raw)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("read site response: %w", err)
	}
	return nil
}

// The site answers a bad secret with "Forbidden" and a missing route with "Not Found".
// Every other 403 or 404 names a playlist.
func statusError(resp *http.Response, raw []byte) error {
	var fail struct {
		Error string `json:"error"`
	}
	json.Unmarshal(raw, &fail)
	switch resp.StatusCode {
	case http.StatusBadRequest:
		return &RuleError{Message: fail.Error}
	case http.StatusForbidden:
		if fail.Error == "Forbidden" || fail.Error == "" {
			return ErrForbidden
		}
		return ErrNotOwner
	case http.StatusNotFound:
		if fail.Error == "Not Found" {
			return ErrNoOwnerAPI
		}
		return ErrNotFound
	case http.StatusConflict:
		return ErrConflict
	case http.StatusGone:
		return ErrDeleted
	case http.StatusTooManyRequests:
		return ErrRateLimited
	case http.StatusServiceUnavailable:
		return ErrDailyCap
	}
	return fmt.Errorf("call site: %s", resp.Status)
}

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
