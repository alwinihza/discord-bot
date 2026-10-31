package embeds

import (
	"context"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/alwinihza/discord-bot/database"
)

// Every song's stats embed must fit Discord's embed limits, or the reply fails.
func TestStatsEmbedFitsDiscordLimitsForEverySong(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	songs, err := db.SongChoices(ctx, "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, song := range songs {
		st, err := db.SongStats(ctx, song.ID)
		if err != nil {
			t.Fatal(err)
		}
		e := CreateStatsEmbed(st)
		total := utf8.RuneCountInString(e.Title)
		for _, f := range e.Fields {
			if utf8.RuneCountInString(f.Value) > 1024 || f.Value == "" {
				t.Fatalf("%s: field %q has %d chars", song.Label(), f.Name, utf8.RuneCountInString(f.Value))
			}
			total += utf8.RuneCountInString(f.Name) + utf8.RuneCountInString(f.Value)
		}
		if total > 6000 {
			t.Fatalf("%s: embed has %d chars, Discord allows 6000", song.Label(), total)
		}
	}
	if len(songs) != 457 {
		t.Fatalf("checked %d songs, want 457", len(songs))
	}
}
