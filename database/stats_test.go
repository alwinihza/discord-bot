package database

import (
	"context"
	"errors"
	"sort"
	"testing"
	"unicode/utf8"
)

func TestSongStatsForMostSungSong(t *testing.T) {
	db := openTestDB(t)
	st, err := db.SongStats(context.Background(), "tabako")
	if err != nil {
		t.Fatal(err)
	}
	if st.Title != "Tabako" || st.Artist != "Koresawa" {
		t.Fatalf("resolved to %q by %q", st.Title, st.Artist)
	}
	if len(st.Performances) != 10 || st.Rank != 1 || st.TotalSongs != 473 {
		t.Fatalf("got %d performances, rank %d of %d; want 10, 1 of 473", len(st.Performances), st.Rank, st.TotalSongs)
	}
	if !sort.SliceIsSorted(st.Performances, func(i, j int) bool { return st.Performances[i].Date < st.Performances[j].Date }) {
		t.Fatal("performances are not oldest first")
	}
	if len(st.Performances[0].Date) != 10 {
		t.Fatalf("date %q is not YYYY-MM-DD", st.Performances[0].Date)
	}
	sum := 0
	for _, n := range st.Versions {
		sum += n
	}
	if sum != 10 {
		t.Fatalf("versions add up to %d, want 10", sum)
	}
}

func TestSongStatsByIDMatchesByText(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	choices, err := db.SongChoices(ctx, "celengan rindu", 1)
	if err != nil || len(choices) != 1 {
		t.Fatalf("choices %v, err %v", choices, err)
	}
	byID, err := db.SongStats(ctx, choices[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	byText, err := db.SongStats(ctx, "celengan rindu")
	if err != nil {
		t.Fatal(err)
	}
	if byID.Title != byText.Title || len(byID.Performances) != len(byText.Performances) || len(byID.Performances) != choices[0].TimesSung {
		t.Fatalf("by id %q x%d, by text %q x%d, choice x%d", byID.Title, len(byID.Performances), byText.Title, len(byText.Performances), choices[0].TimesSung)
	}
	if len(byID.OtherMatches) != 0 {
		t.Fatal("an exact id lookup should not list other matches")
	}
}

func TestSongStatsListsOtherMatches(t *testing.T) {
	db := openTestDB(t)
	st, err := db.SongStats(context.Background(), "rain")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.OtherMatches) == 0 {
		t.Fatal(`"rain" matches several songs but no others were listed`)
	}
}

func TestSongStatsUnknownSong(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.SongStats(context.Background(), "zzz no such song zzz"); !errors.Is(err, ErrSongNotFound) {
		t.Fatalf("got %v, want ErrSongNotFound", err)
	}
}

func TestSongChoicesForEmptyInput(t *testing.T) {
	db := openTestDB(t)
	choices, err := db.SongChoices(context.Background(), "", 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) != 25 || choices[0].TimesSung != 10 {
		t.Fatalf("got %d choices starting with %+v", len(choices), choices[0])
	}
	for _, c := range choices {
		if utf8.RuneCountInString(c.ID) > 100 {
			t.Fatalf("choice value %q exceeds Discord's 100-char limit", c.ID)
		}
	}
}

func TestCoversAndOriginalsAreListedAndCounted(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	kinds := map[string]int{}
	for page := int64(1); page <= 7; page++ {
		videos, _, err := db.FetchVideos(ctx, page, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range videos {
			kinds[v.Kind]++
		}
	}
	if kinds["karaoke"] != 40 || kinds["cover"] != 20 || kinds["original"] != 3 {
		t.Fatalf("video kinds %v", kinds)
	}

	st, err := db.SongStats(ctx, "fated rendezvous")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.OtherMatches) != 0 {
		t.Fatalf("Fated Rendezvous is still split across names: %+v", st.OtherMatches)
	}
}
