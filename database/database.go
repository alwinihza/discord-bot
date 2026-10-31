package database

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	"github.com/alwinihza/discord-bot/models"
	"github.com/alwinihza/discord-bot/shared"
	_ "modernc.org/sqlite"
)

var (
	//go:embed schema.sql
	schemaSQL string
	//go:embed seed.sql
	seedSQL string
)

type DB struct {
	db *sql.DB
}

// Open opens the SQLite file at path, creating and seeding it on first use.
func Open(ctx context.Context, path string) (*DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db := &DB{db: sqlDB}
	if err := db.initialize(ctx); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) initialize(ctx context.Context) error {
	var exists bool
	err := db.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'karaoke_video')").Scan(&exists)
	if err != nil {
		return fmt.Errorf("inspect database: %w", err)
	}
	if exists {
		return nil
	}

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, seedSQL); err != nil {
		return fmt.Errorf("seed data: %w", err)
	}
	return tx.Commit()
}

func (db *DB) Close() error {
	return db.db.Close()
}

func (db *DB) FetchVideos(ctx context.Context, page, itemsPerPage int64) ([]models.VideoSong, int64, error) {
	var totalCount int64
	if err := db.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM karaoke_video").Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("count videos: %w", err)
	}

	rows, err := db.db.QueryContext(ctx,
		"SELECT video_url, video_title FROM karaoke_video ORDER BY video_title, id LIMIT ? OFFSET ?",
		itemsPerPage, (page-1)*itemsPerPage)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch videos: %w", err)
	}
	defer rows.Close()

	var videos []models.VideoSong
	for rows.Next() {
		var videoID, title string
		if err := rows.Scan(&videoID, &title); err != nil {
			return nil, 0, fmt.Errorf("read video: %w", err)
		}
		videos = append(videos, models.VideoSong{Title: title, URL: "https://youtu.be/" + videoID})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read videos: %w", err)
	}
	return videos, totalCount, nil
}

const songQuery = `
	SELECT s.song_title, COALESCE(s.artist, ''), COALESCE(s.version, ''), s.timestamp, v.video_title, v.video_url
	FROM karaoke_song s
	JOIN karaoke_video v ON v.id = s.video_id`

func scanSongs(rows *sql.Rows) ([]models.SearchResult, error) {
	defer rows.Close()
	var songs []models.SearchResult
	for rows.Next() {
		var songTitle, artist, version, timestamp, videoTitle, videoID string
		if err := rows.Scan(&songTitle, &artist, &version, &timestamp, &videoTitle, &videoID); err != nil {
			return nil, err
		}
		songs = append(songs, models.SearchResult{
			VideoTitle: videoTitle,
			Title:      songTitle,
			Artist:     artist,
			Version:    version,
			URL:        fmt.Sprintf("https://youtu.be/%s?t=%d", videoID, shared.TimestampToSeconds(timestamp)),
			Timestamp:  timestamp,
		})
	}
	return songs, rows.Err()
}

// likePattern matches s anywhere, with LIKE wildcards in s taken literally.
func likePattern(s string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s) + "%"
}

const songMatch = `(s.song_title LIKE ?1 ESCAPE '\' OR s.artist LIKE ?1 ESCAPE '\'
	OR s.version LIKE ?1 ESCAPE '\' OR s.original_title LIKE ?1 ESCAPE '\')`

func (db *DB) FetchSearchResult(ctx context.Context, searchValue string, page, itemsPerPage int64) ([]models.SearchResult, int64, error) {
	pattern := likePattern(searchValue)

	var totalCount int64
	err := db.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM karaoke_song s JOIN karaoke_video v ON v.id = s.video_id
		 WHERE `+songMatch, pattern).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("count search results: %w", err)
	}

	rows, err := db.db.QueryContext(ctx,
		songQuery+" WHERE "+songMatch+" ORDER BY s.song_title, s.artist, s.id LIMIT ?2 OFFSET ?3",
		pattern, itemsPerPage, (page-1)*itemsPerPage)
	if err != nil {
		return nil, 0, fmt.Errorf("search %q: %w", searchValue, err)
	}
	songs, err := scanSongs(rows)
	if err != nil {
		return nil, 0, fmt.Errorf("read search results: %w", err)
	}
	return songs, totalCount, nil
}

func (db *DB) FetchRandomSong(ctx context.Context) (models.SearchResult, error) {
	rows, err := db.db.QueryContext(ctx, songQuery+" ORDER BY RANDOM() LIMIT 1")
	if err != nil {
		return models.SearchResult{}, fmt.Errorf("fetch random song: %w", err)
	}
	songs, err := scanSongs(rows)
	if err != nil {
		return models.SearchResult{}, fmt.Errorf("read random song: %w", err)
	}
	if len(songs) == 0 {
		return models.SearchResult{}, fmt.Errorf("no songs in database")
	}
	return songs[0], nil
}
