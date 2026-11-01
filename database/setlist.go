package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/alwinihza/discord-bot/internal/setlist"
	"github.com/alwinihza/discord-bot/models"
)

// Setlist returns a stream's title and songs in order; found is false when
// the video is not in the database yet.
func (db *DB) Setlist(ctx context.Context, videoID string) (title string, entries []setlist.Entry, found bool, err error) {
	err = db.db.QueryRowContext(ctx, "SELECT video_title FROM karaoke_video WHERE video_url = ?", videoID).Scan(&title)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, fmt.Errorf("look up video: %w", err)
	}
	songs, err := db.setlistRows(ctx, db.db, videoID)
	if err != nil {
		return "", nil, false, err
	}
	for _, s := range songs {
		entries = append(entries, s.Entry)
	}
	return title, entries, true, nil
}

type storedSong struct {
	setlist.Entry
	id, originalTitle string
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (db *DB) setlistRows(ctx context.Context, q querier, videoID string) ([]storedSong, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT s.id, s.timestamp, s.song_title, coalesce(s.artist, ''), coalesce(s.version, ''),
		       coalesce(s.note, ''), coalesce(s.original_title, '')
		FROM karaoke_song s JOIN karaoke_video v ON v.id = s.video_id
		WHERE v.video_url = ?`, videoID)
	if err != nil {
		return nil, fmt.Errorf("list setlist: %w", err)
	}
	defer rows.Close()
	var songs []storedSong
	for rows.Next() {
		var s storedSong
		if err := rows.Scan(&s.id, &s.Timestamp, &s.Title, &s.Artist, &s.Version, &s.Note, &s.originalTitle); err != nil {
			return nil, fmt.Errorf("read setlist: %w", err)
		}
		// Older rows hold "1:04:48"; normalize so they sort and compare like new ones.
		if ts, err := setlist.NormalizeTimestamp(s.Timestamp); err == nil {
			s.Timestamp = ts
		}
		songs = append(songs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read setlist: %w", err)
	}
	slices.SortStableFunc(songs, func(a, b storedSong) int { return strings.Compare(a.Timestamp, b.Timestamp) })
	return songs, nil
}

// songSlot identifies a performance across edits of the same setlist.
func songSlot(e setlist.Entry) string {
	return e.Timestamp + "\x00" + strings.ToLower(e.Title)
}

type SaveResult struct {
	Title    string
	Created  bool // the video was not in the database before
	Previous []setlist.Entry
	// NewSongs are title/artist pairs no other stream has, often typos.
	NewSongs []string
}

// SaveSetlist replaces every song of the video with entries. A video not yet
// in the database is created from video; an existing one keeps its details.
// Songs that keep their timestamp and title keep their ID and original title.
func (db *DB) SaveSetlist(ctx context.Context, video models.VideoInfo, entries []setlist.Entry) (SaveResult, error) {
	var res SaveResult
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return res, fmt.Errorf("begin save: %w", err)
	}
	defer tx.Rollback()

	added, err := tx.ExecContext(ctx, `
		INSERT INTO karaoke_video (id, video_title, video_url, published_at, thumbnail)
		SELECT ?, ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM karaoke_video WHERE video_url = ?3)`,
		newID(), video.Title, video.ID, video.PublishedAt, video.Thumbnail)
	if err != nil {
		return res, fmt.Errorf("add video: %w", err)
	}
	if n, err := added.RowsAffected(); err == nil {
		res.Created = n == 1
	}
	var rowID string
	err = tx.QueryRowContext(ctx, "SELECT id, video_title FROM karaoke_video WHERE video_url = ?", video.ID).Scan(&rowID, &res.Title)
	if err != nil {
		return res, fmt.Errorf("look up video: %w", err)
	}

	old, err := db.setlistRows(ctx, tx, video.ID)
	if err != nil {
		return res, err
	}
	kept := map[string]storedSong{}
	for _, s := range old {
		res.Previous = append(res.Previous, s.Entry)
		kept[songSlot(s.Entry)] = s
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM karaoke_song WHERE video_id = ?", rowID); err != nil {
		return res, fmt.Errorf("clear setlist: %w", err)
	}

	for _, e := range entries {
		id, original := newID(), ""
		if s, ok := kept[songSlot(e)]; ok {
			id, original = s.id, s.originalTitle
			delete(kept, songSlot(e))
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO karaoke_song (id, song_title, artist, version, note, original_title, timestamp, video_id)
			VALUES (?, ?, nullif(?, ''), nullif(?, ''), nullif(?, ''), nullif(?, ''), ?, ?)`,
			id, e.Title, e.Artist, e.Version, e.Note, original, e.Timestamp, rowID)
		if err != nil {
			return res, fmt.Errorf("add %q: %w", e.Title, err)
		}
	}

	seen := map[string]bool{}
	for _, e := range entries {
		label := models.SearchResult{Title: e.Title, Artist: e.Artist}.Label()
		if seen[strings.ToLower(label)] {
			continue
		}
		seen[strings.ToLower(label)] = true
		var elsewhere bool
		err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM karaoke_song s WHERE s.video_id <> ?1
				AND lower(s.song_title) = lower(?2) AND lower(coalesce(s.artist, '')) = lower(?3))`,
			rowID, e.Title, e.Artist).Scan(&elsewhere)
		if err != nil {
			return res, fmt.Errorf("check %q: %w", e.Title, err)
		}
		if !elsewhere {
			res.NewSongs = append(res.NewSongs, label)
		}
	}
	return res, tx.Commit()
}

// newID returns a random UUIDv4, matching the IDs the seed data uses.
func newID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
