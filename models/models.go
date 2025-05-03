package models

import (
	"time"

	"github.com/bwmarrin/discordgo"
)

type VideoSong struct {
	Title string
	URL   string
}

type SearchResult struct {
	VideoTitle string
	Title      string
	URL        string
	Timestamp  string
}

type PaginationState struct {
	CurrentPage  int64
	TotalPages   int64
	TotalItems   int64
	ItemsPerPage int64
	SearchValue  string
	Interaction  *discordgo.Interaction
	CreatedAt    time.Time
}
