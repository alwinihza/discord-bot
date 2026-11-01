package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alwinihza/discord-bot/models"
)

func TestParseWatchPage(t *testing.T) {
	page := `<meta name="title" content="【KARAOKE】Fio &amp; friends"><script>{"publishDate":"2026-10-23T01:29:35-07:00"}`
	got, err := parseWatchPage("KANYG45JUlo", []byte(page))
	want := models.VideoInfo{
		ID: "KANYG45JUlo", Title: "【KARAOKE】Fio & friends", PublishedAt: "2026-10-23 08:29:35+00",
		Thumbnail: "https://i.ytimg.com/vi/KANYG45JUlo/hqdefault.jpg",
	}
	if err != nil || got != want {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := parseWatchPage("xxxxxxxxxxx", []byte(`<meta name="title" content="">`)); err == nil {
		t.Error("missing video: want an error")
	}
}

const goodPage = `<meta name="title" content="nyanyinyaa">{"publishDate":"2026-10-23T01:29:35-07:00"}`
const blockedPage = `<meta name="title" content="">Sign in to confirm you’re not a bot`

// serve answers each request with the next page, repeating the last one.
func serve(t *testing.T, pages ...string) *int {
	t.Helper()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, pages[min(requests, len(pages)-1)])
		requests++
	}))
	t.Cleanup(srv.Close)
	oldURL, oldDelays := watchURL, retryDelays
	watchURL, retryDelays = srv.URL+"/", []time.Duration{0, 0, 0}
	t.Cleanup(func() { watchURL, retryDelays = oldURL, oldDelays })
	return &requests
}

func TestFetchRetriesThroughABriefBlock(t *testing.T) {
	requests := serve(t, blockedPage, blockedPage, goodPage)
	got, err := Fetch(context.Background(), "KANYG45JUlo")
	if err != nil || got.Title != "nyanyinyaa" || *requests != 3 {
		t.Fatalf("got %+v, %v after %d requests", got, err, *requests)
	}
}

func TestFetchReportsALastingBlock(t *testing.T) {
	requests := serve(t, blockedPage)
	if _, err := Fetch(context.Background(), "KANYG45JUlo"); !errors.Is(err, ErrBlocked) || *requests != 4 {
		t.Fatalf("err %v after %d requests, want ErrBlocked after 4", err, *requests)
	}
}

func TestFetchDoesNotRetryAMissingVideo(t *testing.T) {
	requests := serve(t, `<meta name="title" content="">`)
	if _, err := Fetch(context.Background(), "xxxxxxxxxxx"); err == nil || errors.Is(err, ErrBlocked) || *requests != 1 {
		t.Fatalf("err %v after %d requests, want a plain error after 1", err, *requests)
	}
}
