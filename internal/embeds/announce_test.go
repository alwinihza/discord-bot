package embeds

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alwinihza/discord-bot/internal/setlist"
	"github.com/alwinihza/discord-bot/models"
)

func TestNewStreamEmbedLinksEverySongAndFitsDiscord(t *testing.T) {
	video := models.VideoInfo{ID: "KANYG45JUlo", Title: "nyanyinyaa", PublishedAt: "2026-10-23 08:29:35+00",
		Thumbnail: "https://i.ytimg.com/vi/KANYG45JUlo/hqdefault.jpg"}
	e := CreateNewStreamEmbed(video, []setlist.Entry{
		{Timestamp: "00:13:22", Title: "Halu", Artist: "Feby Putri"},
		{Timestamp: "01:02:03", Title: "Samurai Heart [Some Like It Hot!!]", Artist: "SPYAIR", Version: "short"},
	})
	for _, want := range []string{
		"`00:13:22` [Halu - Feby Putri](https://youtu.be/KANYG45JUlo?t=802)",
		"[Samurai Heart (Some Like It Hot!!) (short) - SPYAIR](https://youtu.be/KANYG45JUlo?t=3723)",
	} {
		if !strings.Contains(e.Description, want) {
			t.Errorf("description %q lacks %q", e.Description, want)
		}
	}
	if e.URL != "https://youtu.be/KANYG45JUlo" || e.Footer.Text != "2 songs · new karaoke stream · 2026-10-23" {
		t.Errorf("url %q footer %q", e.URL, e.Footer.Text)
	}

	var many []setlist.Entry
	for n := range 200 {
		many = append(many, setlist.Entry{Timestamp: fmt.Sprintf("%02d:%02d:00", n/60, n%60), Title: strings.Repeat("long title ", 8), Artist: "Someone"})
	}
	e = CreateNewStreamEmbed(video, many)
	if len(e.Description) > 4096 || !strings.Contains(e.Description, "more") {
		t.Errorf("200 songs: %d chars, ends %q", len(e.Description), e.Description[len(e.Description)-30:])
	}
}
