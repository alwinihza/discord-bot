package commands

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/alwinihza/discord-bot/database"
	"github.com/alwinihza/discord-bot/internal/embeds"
	"github.com/alwinihza/discord-bot/shared"
	"github.com/bwmarrin/discordgo"
)

const maxChoices = 25 // Discord's autocomplete limit

func songOption(i *discordgo.InteractionCreate) string {
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "song" {
			return opt.StringValue()
		}
	}
	return ""
}

// Stats replies in the channel, visible to everyone; only errors are private.
func (h *CommandHandler) Stats(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	query := songOption(i)
	stats, err := h.db.SongStats(ctx, query)
	if errors.Is(err, database.ErrSongNotFound) {
		shared.RespondWithError(s, i, fmt.Sprintf("No song matches %q", query))
		return
	}
	if err != nil {
		log.Printf("Error fetching stats for %q: %v", query, err)
		shared.RespondWithError(s, i, "Failed to fetch song stats, please try again")
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embeds.CreateStatsEmbed(stats)},
		},
	})
	if err != nil {
		log.Printf("Error sending stats: %v", err)
	}
}

func (h *CommandHandler) songChoices(query string) []*discordgo.ApplicationCommandOptionChoice {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	songs, err := h.db.SongChoices(ctx, query, maxChoices)
	if err != nil {
		log.Printf("Error listing song choices: %v", err)
	}
	choices := make([]*discordgo.ApplicationCommandOptionChoice, len(songs))
	for n, c := range songs {
		choices[n] = &discordgo.ApplicationCommandOptionChoice{
			Name:  embeds.Truncate(fmt.Sprintf("%s (×%d)", c.Label(), c.TimesSung), 100),
			Value: c.ID,
		}
	}
	return choices
}

func (h *CommandHandler) Autocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var choices []*discordgo.ApplicationCommandOptionChoice
	switch i.ApplicationCommandData().Name {
	case "stats":
		choices = h.songChoices(songOption(i))
	case "playlist":
		choices = h.playlistChoices(i)
	default:
		return
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: choices},
	})
	if err != nil {
		log.Printf("Error sending song choices: %v", err)
	}
}
