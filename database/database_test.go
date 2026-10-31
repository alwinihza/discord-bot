package database

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "karaoke.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestReopenKeepsDataWithoutReseeding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "karaoke.db")
	ctx := context.Background()
	for range 2 {
		db, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		_, videos, err := db.FetchVideos(ctx, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		_, songs, err := db.FetchSearchResult(ctx, "", 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		db.Close()
		if videos != 38 || songs != 840 {
			t.Fatalf("got %d videos and %d songs, want 38 and 840", videos, songs)
		}
	}
}

func TestVideoPagesCoverEveryVideoOnce(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, total, err := db.FetchVideos(ctx, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for page := int64(1); (page-1)*10 < total; page++ {
		videos, _, err := db.FetchVideos(ctx, page, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range videos {
			if seen[v.URL] {
				t.Fatalf("video %q appears on more than one page", v.URL)
			}
			seen[v.URL] = true
		}
	}
	if int64(len(seen)) != total {
		t.Fatalf("pages covered %d videos, count says %d", len(seen), total)
	}
}

func TestSearchPagesCoverEveryMatchOnce(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, total, err := db.FetchSearchResult(ctx, "a", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total <= 10 {
		t.Fatalf("want a multi-page result for %q, got %d rows", "a", total)
	}
	seen := map[string]bool{}
	for page := int64(1); (page-1)*10 < total; page++ {
		songs, _, err := db.FetchSearchResult(ctx, "a", page, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range songs {
			key := s.URL + "|" + s.Title
			if seen[key] {
				t.Fatalf("song %q appears on more than one page", key)
			}
			seen[key] = true
		}
	}
	if int64(len(seen)) != total {
		t.Fatalf("pages covered %d songs, count says %d", len(seen), total)
	}
}

func TestSearchTreatsWildcardsLiterally(t *testing.T) {
	db := openTestDB(t)
	_, total, err := db.FetchSearchResult(context.Background(), "%", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf(`search for "%%" matched %d songs, want 0`, total)
	}
}

func TestConcurrentQueries(t *testing.T) {
	db := openTestDB(t)
	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for range 50 {
		wg.Go(func() {
			if _, err := db.FetchRandomSong(context.Background()); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestSearchMatchesArtist(t *testing.T) {
	db := openTestDB(t)
	songs, total, err := db.FetchSearchResult(context.Background(), "rie takahashi", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 {
		t.Fatal("searching an artist name found no songs")
	}
	for _, s := range songs {
		if s.Title != "Stay Alive" || s.Artist != "Emilia (CV: Rie Takahashi)" {
			t.Errorf("unexpected match %q", s.Label())
		}
	}
}

func TestSearchFindsEverythingTheOriginalTitlesMatched(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, q := range []string{"GUMI", "Hatsune Miku", "Hanatan", "DAZBEE", "Motohiro Hata", "Kokoronashi", "short"} {
		var before int64
		err := db.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM karaoke_song WHERE original_title LIKE ? ESCAPE '\'`, likePattern(q)).Scan(&before)
		if err != nil {
			t.Fatal(err)
		}
		_, now, err := db.FetchSearchResult(ctx, q, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		if before == 0 || now < before {
			t.Errorf("search %q: %d results, original titles matched %d", q, now, before)
		}
	}
}
