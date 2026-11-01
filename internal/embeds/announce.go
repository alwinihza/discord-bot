package embeds

import (
	"fmt"
	"strings"

	"github.com/alwinihza/discord-bot/internal/setlist"
	"github.com/alwinihza/discord-bot/models"
	"github.com/alwinihza/discord-bot/shared"
	"github.com/bwmarrin/discordgo"
)

const maxDescription = 4096

// CreateNewStreamEmbed announces a stream with its songs, each linked to
// the moment it starts.
func CreateNewStreamEmbed(video models.VideoInfo, entries []setlist.Entry) *discordgo.MessageEmbed {
	url := "https://youtu.be/" + video.ID
	var b strings.Builder
	for n, e := range entries {
		label := models.SearchResult{Title: e.Title, Artist: e.Artist, Version: e.Version}.Label()
		line := fmt.Sprintf("`%s` [%s](%s?t=%d)\n", e.Timestamp, linkText.Replace(Truncate(label, 80)), url, shared.TimestampToSeconds(e.Timestamp))
		more := fmt.Sprintf("…and %d more", len(entries)-n)
		if b.Len()+len(line)+len(more) > maxDescription {
			b.WriteString(more)
			break
		}
		b.WriteString(line)
	}
	embed := &discordgo.MessageEmbed{
		Title:       Truncate("🎤 "+video.Title, 256),
		URL:         url,
		Description: b.String(),
		Color:       embedColor,
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("%d songs · new karaoke stream", len(entries))},
	}
	if video.Thumbnail != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: video.Thumbnail}
	}
	if len(video.PublishedAt) >= 10 {
		embed.Footer.Text += " · " + video.PublishedAt[:10]
	}
	return embed
}
