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
	if len(st.Performances) != 9 || st.Rank != 1 || st.TotalSongs != 457 {
		t.Fatalf("got %d performances, rank %d of %d; want 9, 1 of 457", len(st.Performances), st.Rank, st.TotalSongs)
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
	if sum != 9 {
		t.Fatalf("versions add up to %d, want 9", sum)
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
	if len(choices) != 25 || choices[0].Title != "Tabako" {
		t.Fatalf("got %d choices starting with %+v", len(choices), choices[0])
	}
	for _, c := range choices {
		if utf8.RuneCountInString(c.ID) > 100 {
			t.Fatalf("choice value %q exceeds Discord's 100-char limit", c.ID)
		}
	}
}
