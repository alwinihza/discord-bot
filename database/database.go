package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
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
	//go:embed migrations/*.sql
	migrationFiles embed.FS
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
	err = db.initialize(ctx)
	if err == nil {
		err = db.migrate(ctx)
	}
	if err != nil {
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

// migrate applies migrations/NNN_*.sql in order, each exactly once, tracking
// progress in SQLite's user_version.
func (db *DB) migrate(ctx context.Context) error {
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	var applied int
	if err := db.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&applied); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	for v := applied; v < len(names); v++ {
		body, err := migrationFiles.ReadFile(names[v])
		if err != nil {
			return err
		}
		tx, err := db.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, string(body)+fmt.Sprintf("\nPRAGMA user_version = %d;", v+1))
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		if err != nil {
			return fmt.Errorf("apply %s: %w", names[v], err)
		}
	}
	return nil
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
		"SELECT video_url, video_title, kind FROM karaoke_video ORDER BY video_title, id LIMIT ? OFFSET ?",
		itemsPerPage, (page-1)*itemsPerPage)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch videos: %w", err)
	}
	defer rows.Close()

	var videos []models.VideoSong
	for rows.Next() {
		var videoID, title, kind string
		if err := rows.Scan(&videoID, &title, &kind); err != nil {
			return nil, 0, fmt.Errorf("read video: %w", err)
		}
		videos = append(videos, models.VideoSong{Title: title, URL: "https://youtu.be/" + videoID, Kind: kind})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read videos: %w", err)
	}
	return videos, totalCount, nil
}

const songQuery = `
	SELECT s.song_title, COALESCE(s.artist, ''), COALESCE(s.version, ''), COALESCE(s.note, ''), s.timestamp, v.video_title, v.video_url
	FROM karaoke_song s
	JOIN karaoke_video v ON v.id = s.video_id`

func scanSongs(rows *sql.Rows) ([]models.SearchResult, error) {
	defer rows.Close()
	var songs []models.SearchResult
	for rows.Next() {
		var songTitle, artist, version, note, timestamp, videoTitle, videoID string
		if err := rows.Scan(&songTitle, &artist, &version, &note, &timestamp, &videoTitle, &videoID); err != nil {
			return nil, err
		}
		songs = append(songs, models.SearchResult{
			VideoTitle: videoTitle,
			Title:      songTitle,
			Artist:     artist,
			Version:    version,
			Note:       note,
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
	OR s.version LIKE ?1 ESCAPE '\' OR s.note LIKE ?1 ESCAPE '\' OR s.original_title LIKE ?1 ESCAPE '\')`

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
