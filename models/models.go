package models

import "time"

type VideoSong struct {
	Title string
	URL   string
}

type SearchResult struct {
	VideoTitle string
	Title      string
	Artist     string
	Version    string // playing style tags shown in labels: "short", "acapella"
	Note       string // context kept out of labels: cover source, series
	URL        string
	Timestamp  string
}

// Label renders the song as "Title (version) - Artist", omitting empty parts.
func (s SearchResult) Label() string {
	label := s.Title
	if s.Version != "" {
		label += " (" + s.Version + ")"
	}
	if s.Artist != "" {
		label += " - " + s.Artist
	}
	return label
}

// PaginationKind is also the custom-ID prefix of that view's buttons.
type PaginationKind string

const (
	KindList   PaginationKind = "list"
	KindSearch PaginationKind = "search"
)

type PaginationState struct {
	Kind        PaginationKind
	CurrentPage int64
	TotalPages  int64
	SearchValue string
	CreatedAt   time.Time
}

type SongChoice struct {
	ID        string
	Title     string
	Artist    string
	TimesSung int
}

func (c SongChoice) Label() string {
	return SearchResult{Title: c.Title, Artist: c.Artist}.Label()
}

type Performance struct {
	Date       string
	VideoTitle string
	Version    string
	Note       string
	URL        string
}

// SongStats covers every performance of one song, all versions together.
type SongStats struct {
	Title        string
	Artist       string
	Rank         int
	TotalSongs   int
	StreamCount  int
	Versions     map[string]int
	Notes        []string
	Performances []Performance // oldest first
	OtherMatches []SongChoice
}
