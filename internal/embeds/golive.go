package embeds

import (
	"fmt"
	"strings"
	"time"

	"github.com/alwinihza/discord-bot/models"
	"github.com/bwmarrin/discordgo"
)

const (
	watchURL = "https://www.youtube.com/watch?v="
	// The footers mark go-live messages, so the bot can find its own posts
	// in the channel after a restart. Changing them orphans open messages.
	liveFooter  = "Live on YouTube"
	endedFooter = "VOD on YouTube"
	liveColor   = 0xFF0000
)

// GoLiveKind says which go-live embed a message carries.
type GoLiveKind int

const (
	NotGoLive GoLiveKind = iota
	Live
	Ended
)

// CreateLiveEmbed announces a stream that is live now.
func CreateLiveEmbed(video models.VideoInfo) *discordgo.MessageEmbed {
	return goLiveEmbed(video, "🔴 "+video.Title, "🔴 **Live now**", liveColor, liveFooter)
}

// CreateEndedEmbed replaces the live embed once the stream is over. The
// link now leads to the VOD.
func CreateEndedEmbed(video models.VideoInfo, length time.Duration) *discordgo.MessageEmbed {
	return goLiveEmbed(video, video.Title, "Stream ended · "+formatLength(length), embedColor, endedFooter)
}

func goLiveEmbed(video models.VideoInfo, title, description string, color int, footer string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       Truncate(title, maxTitleLength),
		URL:         watchURL + video.ID,
		Description: description,
		Color:       color,
		Footer:      &discordgo.MessageEmbedFooter{Text: footer},
	}
	if video.Thumbnail != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: video.Thumbnail}
	}
	return embed
}

// ParseGoLive reports which go-live embed m carries and the video it is
// about, with the title as shown (without the live marker). It does not check
// the author.
func ParseGoLive(m *discordgo.Message) (GoLiveKind, models.VideoInfo) {
	for _, e := range m.Embeds {
		if e.Footer == nil {
			continue
		}
		var kind GoLiveKind
		switch e.Footer.Text {
		case liveFooter:
			kind = Live
		case endedFooter:
			kind = Ended
		default:
			continue
		}
		id, ok := strings.CutPrefix(e.URL, watchURL)
		if !ok || id == "" {
			continue
		}
		video := models.VideoInfo{ID: id, Title: strings.TrimPrefix(e.Title, "🔴 ")}
		if e.Image != nil {
			video.Thumbnail = e.Image.URL
		}
		return kind, video
	}
	return NotGoLive, models.VideoInfo{}
}

func formatLength(d time.Duration) string {
	minutes := int(d.Round(time.Minute) / time.Minute)
	if minutes < 60 {
		return fmt.Sprintf("%dm", max(minutes, 0))
	}
	return fmt.Sprintf("%dh %dm", minutes/60, minutes%60)
}
