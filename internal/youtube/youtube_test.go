package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alwinihza/discord-bot/models"
)

// serve answers like the Data API and fails the test if the key is missing
// from the header or appears in the URL.
func serve(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-Api-Key") != "secret-key" {
			t.Errorf("key header = %q", r.Header.Get("X-Goog-Api-Key"))
		}
		if strings.Contains(r.URL.String(), "secret-key") {
			t.Errorf("key leaked into URL %q", r.URL)
		}
		if got := r.URL.Query().Get("id"); got != "KANYG45JUlo" {
			t.Errorf("id = %q", got)
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return &Client{key: "secret-key", url: srv.URL}
}

func TestFetch(t *testing.T) {
	c := serve(t, http.StatusOK, `{"items":[{"snippet":{"title":"【KARAOKE】Fio & friends ♡","publishedAt":"2026-10-23T08:29:35Z"}}]}`)
	got, err := c.Fetch(context.Background(), "KANYG45JUlo")
	want := models.VideoInfo{
		ID: "KANYG45JUlo", Title: "【KARAOKE】Fio & friends ♡", PublishedAt: "2026-10-23 08:29:35+00",
		Thumbnail: "https://i.ytimg.com/vi/KANYG45JUlo/hqdefault.jpg",
	}
	if err != nil || got != want {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestFetchMissingVideo(t *testing.T) {
	c := serve(t, http.StatusOK, `{"items":[]}`)
	if _, err := c.Fetch(context.Background(), "KANYG45JUlo"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestFetchAPIErrorIsNotMissingVideo(t *testing.T) {
	c := serve(t, http.StatusForbidden, `{"error":{"code":403}}`)
	_, err := c.Fetch(context.Background(), "KANYG45JUlo")
	if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want a 403 error", err)
	}
}
