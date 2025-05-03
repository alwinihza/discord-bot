// internal/embeds/embeds.go
package embeds

import (
	"fmt"

	"github.com/alwinihza/discord-bot/models"
	"github.com/bwmarrin/discordgo"
)

func CreateVideoEmbed(videos []models.VideoSong, currentPage, totalPages int64) *discordgo.MessageEmbed {
	fields := make([]*discordgo.MessageEmbedField, len(videos))
	for i, video := range videos {
		fields[i] = &discordgo.MessageEmbedField{
			Name:   video.Title,
			Value:  fmt.Sprintf("[Watch Video](%s)", video.URL),
			Inline: false,
		}
	}

	return &discordgo.MessageEmbed{
		Title:       "Karaoke Video List",
		Description: fmt.Sprintf("Page %d/%d", currentPage, totalPages),
		Color:       0x00BFFF,
		Fields:      fields,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Use the buttons below to navigate through the list!",
		},
	}
}

func CreateSongEmbed(videos []models.SearchResult, currentPage, totalPages int64) *discordgo.MessageEmbed {
	fields := make([]*discordgo.MessageEmbedField, len(videos))
	for i, video := range videos {
		fields[i] = &discordgo.MessageEmbedField{
			Name:   video.VideoTitle,
			Value:  fmt.Sprintf("[%s - %s](%s)", video.Timestamp, video.Title, video.URL),
			Inline: false,
		}
	}

	return &discordgo.MessageEmbed{
		Title:       "Search Result",
		Description: fmt.Sprintf("Page %d/%d", currentPage, totalPages),
		Color:       0x00BFFF,
		Fields:      fields,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Use the buttons below to navigate through the list!",
		},
	}
}

func CreatePaginationButtons(currentPage, totalPages int64, key string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Emoji:    discordgo.ComponentEmoji{Name: "⬅️"},
					Style:    discordgo.PrimaryButton,
					CustomID: key + "prev_page",
					Disabled: currentPage <= 1,
				},
				discordgo.Button{
					Emoji:    discordgo.ComponentEmoji{Name: "➡️"},
					Style:    discordgo.PrimaryButton,
					CustomID: key + "next_page",
					Disabled: currentPage >= totalPages,
				},
				discordgo.Button{
					Emoji:    discordgo.ComponentEmoji{Name: "❌"},
					Style:    discordgo.SecondaryButton,
					CustomID: "close",
					Disabled: false,
				},
			},
		},
	}
}
