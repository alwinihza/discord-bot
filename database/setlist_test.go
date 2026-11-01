package database

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/alwinihza/discord-bot/internal/setlist"
	"github.com/alwinihza/discord-bot/models"
)

// Every stored setlist must survive a trip through the /setlist form
// unchanged, or a mod who opens and resubmits one would corrupt it.
func TestEveryStoredSetlistRoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	rows, err := db.db.QueryContext(ctx, "SELECT video_url FROM karaoke_video")
	if err != nil {
		t.Fatal(err)
	}
	countOriginals := func() (n int) {
		db.db.QueryRowContext(ctx, "SELECT count(*) FROM karaoke_song WHERE original_title IS NOT NULL").Scan(&n)
		return n
	}
	originals := countOriginals()
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()

	for _, id := range ids {
		_, entries, found, err := db.Setlist(ctx, id)
		if err != nil || !found {
			t.Fatalf("Setlist(%s) = %v, %v", id, found, err)
		}
		text := setlist.Format(entries)
		if len(text) > 4000 {
			t.Errorf("%s: setlist is %d chars, over the form limit", id, len(text))
		}
		parsed, err := setlist.Parse(text)
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		if !reflect.DeepEqual(parsed, entries) {
			t.Errorf("%s: round trip changed the setlist", id)
		}
		if _, err := db.SaveSetlist(ctx, models.VideoInfo{ID: id}, parsed); err != nil {
			t.Fatal(err)
		}
		_, again, _, _ := db.Setlist(ctx, id)
		if !reflect.DeepEqual(again, entries) {
			t.Errorf("%s: resaving changed the setlist", id)
		}
	}
	_, songs, _ := db.FetchSearchResult(ctx, "", 1, 1)
	if songs != 908 {
		t.Errorf("resaving everything left %d songs, want 908", songs)
	}
	if n := countOriginals(); n != originals || n == 0 {
		t.Errorf("resaving kept %d original titles, want %d", n, originals)
	}
}

func TestSaveSetlistAddsAndReplaces(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	video := models.VideoInfo{ID: "KANYG45JUlo", Title: "nyanyinyaa", PublishedAt: "2026-10-23 08:29:35+00"}
	first, _ := setlist.Parse("00:10:00 | Halu | Feby Putri\n00:20:00 | Totally Made Up | Nobody")
	res, err := db.SaveSetlist(ctx, video, first)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "nyanyinyaa" || len(res.Previous) != 0 {
		t.Errorf("first save = %+v", res)
	}
	if !reflect.DeepEqual(res.NewSongs, []string{"Totally Made Up - Nobody"}) {
		t.Errorf("NewSongs = %q, want only the unknown song", res.NewSongs)
	}

	second, _ := setlist.Parse("00:10:00 | Halu | Feby Putri | acoustic")
	res, err = db.SaveSetlist(ctx, models.VideoInfo{ID: "KANYG45JUlo", Title: "ignored"}, second)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "nyanyinyaa" || len(res.Previous) != 2 {
		t.Errorf("second save = %+v", res)
	}
	_, entries, _, _ := db.Setlist(ctx, "KANYG45JUlo")
	if !reflect.DeepEqual(entries, second) {
		t.Errorf("stored %+v, want %+v", entries, second)
	}

	results, _, _ := db.FetchSearchResult(ctx, "Totally Made Up", 1, 10)
	if len(results) != 0 {
		t.Errorf("removed song still searchable: %+v", results)
	}
	stats, err := db.SongStats(ctx, "Halu")
	if err != nil {
		t.Fatal(err)
	}
	last := stats.Performances[len(stats.Performances)-1]
	if last.Date != "2026-10-23" || !strings.Contains(last.URL, "KANYG45JUlo?t=600") {
		t.Errorf("new performance = %+v", last)
	}
}
