package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/alwinihza/discord-bot/models"
	"github.com/alwinihza/discord-bot/shared"
)

var ErrSongNotFound = errors.New("song not found")

// A song is every row sharing a title and artist, so all versions count together.
const songKey = "lower(s.song_title), lower(coalesce(s.artist, ''))"

// SongChoices returns the songs matching query, most sung first.
func (db *DB) SongChoices(ctx context.Context, query string, limit int) ([]models.SongChoice, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT min(s.id), s.song_title, coalesce(s.artist, ''), count(*) AS times
		FROM karaoke_song s
		WHERE `+songMatch+`
		GROUP BY `+songKey+`
		ORDER BY times DESC, s.song_title, s.artist
		LIMIT ?2`, likePattern(query), limit)
	if err != nil {
		return nil, fmt.Errorf("list song choices: %w", err)
	}
	defer rows.Close()

	var choices []models.SongChoice
	for rows.Next() {
		var c models.SongChoice
		if err := rows.Scan(&c.ID, &c.Title, &c.Artist, &c.TimesSung); err != nil {
			return nil, fmt.Errorf("read song choice: %w", err)
		}
		choices = append(choices, c)
	}
	return choices, rows.Err()
}

// SongStats resolves query as a song ID (from autocomplete) or, failing
// that, as search text, picking the most-sung match.
func (db *DB) SongStats(ctx context.Context, query string) (models.SongStats, error) {
	var stats models.SongStats
	err := db.db.QueryRowContext(ctx,
		"SELECT song_title, coalesce(artist, '') FROM karaoke_song WHERE id = ?", query).
		Scan(&stats.Title, &stats.Artist)
	if errors.Is(err, sql.ErrNoRows) {
		choices, err := db.SongChoices(ctx, query, 4)
		if err != nil {
			return stats, err
		}
		if len(choices) == 0 {
			return stats, ErrSongNotFound
		}
		stats.Title, stats.Artist, stats.OtherMatches = choices[0].Title, choices[0].Artist, choices[1:]
	} else if err != nil {
		return stats, fmt.Errorf("look up song id: %w", err)
	}

	err = db.db.QueryRowContext(ctx, `
		SELECT rank, total FROM (
			SELECT t, a, RANK() OVER (ORDER BY times DESC) AS rank, count(*) OVER () AS total
			FROM (SELECT lower(s.song_title) AS t, lower(coalesce(s.artist, '')) AS a, count(*) AS times
			      FROM karaoke_song s GROUP BY `+songKey+`)
		) WHERE t = lower(?1) AND a = lower(?2)`, stats.Title, stats.Artist).Scan(&stats.Rank, &stats.TotalSongs)
	if err != nil {
		return stats, fmt.Errorf("rank song: %w", err)
	}

	rows, err := db.db.QueryContext(ctx, `
		SELECT substr(v.published_at, 1, 10), v.video_title, v.video_url, s.timestamp, coalesce(s.version, ''), coalesce(s.note, '')
		FROM karaoke_song s
		JOIN karaoke_video v ON v.id = s.video_id
		WHERE lower(s.song_title) = lower(?1) AND lower(coalesce(s.artist, '')) = lower(?2)
		ORDER BY v.published_at, s.timestamp`, stats.Title, stats.Artist)
	if err != nil {
		return stats, fmt.Errorf("list performances: %w", err)
	}
	defer rows.Close()

	stats.Versions = map[string]int{}
	streams := map[string]bool{}
	for rows.Next() {
		var p models.Performance
		var videoID, timestamp string
		if err := rows.Scan(&p.Date, &p.VideoTitle, &videoID, &timestamp, &p.Version, &p.Note); err != nil {
			return stats, fmt.Errorf("read performance: %w", err)
		}
		p.URL = fmt.Sprintf("https://youtu.be/%s?t=%d", videoID, shared.TimestampToSeconds(timestamp))
		stats.Performances = append(stats.Performances, p)
		stats.Versions[p.Version]++
		if p.Note != "" && !slices.Contains(stats.Notes, p.Note) {
			stats.Notes = append(stats.Notes, p.Note)
		}
		streams[videoID] = true
	}
	stats.StreamCount = len(streams)
	return stats, rows.Err()
}
