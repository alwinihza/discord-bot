package youtube

import (
	"testing"

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
