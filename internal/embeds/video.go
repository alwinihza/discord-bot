package embeds

import (
	"fmt"

	"github.com/alwinihza/discord-bot/models"
	"github.com/bwmarrin/discordgo"
)

const (
	embedColor     = 0x00BFFF
	footerText     = "Use the buttons below to navigate through the list!"
	maxTitleLength = 256
)

// Truncate shortens s to at most n runes, ending with "…" when cut.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func pagedEmbed(title string, fields []*discordgo.MessageEmbedField, currentPage, totalPages int64) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       Truncate(title, maxTitleLength),
		Description: fmt.Sprintf("Page %d/%d", currentPage, totalPages),
		Color:       embedColor,
		Fields:      fields,
		Footer:      &discordgo.MessageEmbedFooter{Text: footerText},
	}
}

func CreateVideoEmbed(videos []models.VideoSong, currentPage, totalPages int64) *discordgo.MessageEmbed {
	fields := make([]*discordgo.MessageEmbedField, len(videos))
	for i, video := range videos {
		fields[i] = &discordgo.MessageEmbedField{
			Name:  Truncate(video.Title, 256),
			Value: fmt.Sprintf("[Watch Video](%s)", video.URL),
		}
	}
	return pagedEmbed("Karaoke Video List", fields, currentPage, totalPages)
}

func CreateSongEmbed(songs []models.SearchResult, query string, currentPage, totalPages int64) *discordgo.MessageEmbed {
	fields := make([]*discordgo.MessageEmbedField, len(songs))
	for i, song := range songs {
		fields[i] = &discordgo.MessageEmbedField{
			Name:  Truncate(song.VideoTitle, 256),
			Value: fmt.Sprintf("[%s - %s](%s)", song.Timestamp, Truncate(song.Label(), 900), song.URL),
		}
	}
	return pagedEmbed(fmt.Sprintf("Search results for %q", query), fields, currentPage, totalPages)
}

func CreatePaginationButtons(currentPage, totalPages int64, key string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Emoji:    &discordgo.ComponentEmoji{Name: "⬅️"},
					Style:    discordgo.PrimaryButton,
					CustomID: key + "prev_page",
					Disabled: currentPage <= 1,
				},
				discordgo.Button{
					Emoji:    &discordgo.ComponentEmoji{Name: "➡️"},
					Style:    discordgo.PrimaryButton,
					CustomID: key + "next_page",
					Disabled: currentPage >= totalPages,
				},
				discordgo.Button{
					Emoji:    &discordgo.ComponentEmoji{Name: "❌"},
					Style:    discordgo.SecondaryButton,
					CustomID: "close",
				},
			},
		},
	}
}
