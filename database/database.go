package database

import (
	"context"
	"fmt"

	"github.com/alwinihza/discord-bot/models"
	"github.com/alwinihza/discord-bot/shared"
	"github.com/jackc/pgx/v5"
)

type DB struct {
	conn *pgx.Conn
}

func NewDB(connectionString string) (*DB, error) {
	conn, err := pgx.Connect(context.Background(), connectionString)
	if err != nil {
		return nil, fmt.Errorf("database connection failed: %w", err)
	}

	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return &DB{conn: conn}, nil
}

func (db *DB) Close(ctx context.Context) error {
	return db.conn.Close(ctx)
}

// Add your database query methods here...
func (db *DB) FetchVideos(page, itemsPerPage int64) ([]models.VideoSong, int64, error) {
	// Get total count
	var totalCount int64
	err := db.conn.QueryRow(context.Background(), "SELECT COUNT(*) FROM karaoke_video").Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get video count: %w", err)
	}

	// Calculate offset
	offset := (page - 1) * itemsPerPage

	// Fetch paginated results
	rows, err := db.conn.Query(
		context.Background(),
		"SELECT video_url, video_title FROM karaoke_video ORDER BY video_title LIMIT $1 OFFSET $2",
		itemsPerPage,
		offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch videos: %w", err)
	}
	defer rows.Close()

	ret := make([]models.VideoSong, 0, itemsPerPage)
	for rows.Next() {
		var videoTitle, videoUrl string
		if err := rows.Scan(&videoUrl, &videoTitle); err != nil {
			return nil, 0, fmt.Errorf("failed to scan video row: %w", err)
		}
		ret = append(ret, models.VideoSong{
			Title: videoTitle,
			URL:   fmt.Sprintf("https://youtu.be/%s", videoUrl),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return ret, totalCount, nil
}

func (db *DB) FetchSearchResult(searchValue string, page, itemsPerPage int64) ([]models.SearchResult, int64, error) {

	var totalCount int64

	offset := (page - 1) * itemsPerPage
	err := db.conn.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM karaoke_song s JOIN karaoke_video v ON v.id=s.video_id WHERE s.song_title ILIKE $1",
		"%"+searchValue+"%").Scan(&totalCount)

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get video count: %w", err)
	}

	rows, err := db.conn.Query(context.Background(),
		"SELECT song_title,timestamp,video_title,video_url FROM karaoke_song s JOIN karaoke_video v ON v.id=s.video_id WHERE s.song_title ILIKE $1 LIMIT $2 OFFSET $3",
		"%"+searchValue+"%", itemsPerPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to search the keyword %s : %w", searchValue, err)
	}

	ret := make([]models.SearchResult, 0, itemsPerPage)
	for rows.Next() {
		var videoTitle, videoUrl, songTitle, timestamp string
		rows.Scan(&songTitle, &timestamp, &videoTitle, &videoUrl)
		temp := models.SearchResult{
			VideoTitle: videoTitle,
			Title:      songTitle,
			URL:        fmt.Sprintf("https://youtu.be/%s?t=%d", videoUrl, shared.TimestampToSeconds(timestamp)),
			Timestamp:  timestamp,
		}
		ret = append(ret, temp)
	}
	return ret, totalCount, nil
}

func (db *DB) FetchRandomSong() (*models.SearchResult, error) {
	row := db.conn.QueryRow(context.Background(), `
		SELECT song_title, timestamp, video_title, video_url 
		FROM karaoke_song s 
		JOIN karaoke_video v ON v.id=s.video_id 
		ORDER BY RANDOM() 
		LIMIT 1`)

	var videoTitle, videoUrl, songTitle, timestamp string
	if err := row.Scan(&songTitle, &timestamp, &videoTitle, &videoUrl); err != nil {
		return nil, fmt.Errorf("failed to fetch random song: %w", err)
	}

	return &models.SearchResult{
		VideoTitle: videoTitle,
		Title:      songTitle,
		URL:        fmt.Sprintf("https://youtu.be/%s?t=%d", videoUrl, shared.TimestampToSeconds(timestamp)),
		Timestamp:  timestamp,
	}, nil
}
