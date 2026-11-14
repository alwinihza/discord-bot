package commands

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/alwinihza/discord-bot/shared"
	"github.com/bwmarrin/discordgo"
)

const (
	playlistConfirmPrefix = "playlistmod:confirm:"
	playlistCancelID      = "playlistmod:cancel"
	// The delete runs after the interaction is acknowledged, so it may take longer than 3s.
	deleteTimeout = 10 * time.Second
)

// PlaylistMod handles /playlist-mod info|delete for mods.
func (h *CommandHandler) PlaylistMod(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if h.site == nil {
		shared.RespondWithError(s, i, "Playlist moderation is not set up on this bot")
		return
	}
	sub := i.ApplicationCommandData().Options[0]
	var link string
	for _, opt := range sub.Options {
		if opt.Name == "link" {
			link = opt.StringValue()
		}
	}
	id, ok := site.PlaylistID(link)
	if !ok {
		shared.RespondWithError(s, i, "That is not a playlist link or id")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	summary, err := h.site.Info(ctx, id)
	if err != nil {
		shared.RespondWithError(s, i, h.siteError(err))
		return
	}

	switch sub.Name {
	case "info":
		respondEphemeral(s, i, describePlaylist(summary), nil)
	case "delete":
		if summary.Deleted {
			respondEphemeral(s, i, describePlaylist(summary)+"\nIt is already deleted.", nil)
			return
		}
		respondEphemeral(s, i, "Delete this playlist? Its link stops working.\n"+describePlaylist(summary), []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Delete", Style: discordgo.DangerButton, CustomID: playlistConfirmPrefix + id},
				discordgo.Button{Label: "Cancel", Style: discordgo.SecondaryButton, CustomID: playlistCancelID},
			}},
		})
	}
}

func (h *CommandHandler) handlePlaylistButton(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	if customID == playlistCancelID {
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{Content: "Cancelled. The playlist is unchanged.", Components: []discordgo.MessageComponent{}},
		})
		if err != nil {
			log.Printf("Error cancelling playlist delete: %v", err)
		}
		return
	}

	id := strings.TrimPrefix(customID, playlistConfirmPrefix)
	if h.site == nil || i.Member == nil || i.Member.Permissions&discordgo.PermissionManageMessages == 0 {
		shared.RespondWithError(s, i, "Only mods can delete playlists")
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); err != nil {
		log.Printf("Error acknowledging playlist delete: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), deleteTimeout)
	defer cancel()
	summary, err := h.site.Delete(ctx, id)
	content := "Deleted.\n" + describePlaylist(summary)
	if err != nil {
		content = "❌ " + h.siteError(err)
	}
	empty := []discordgo.MessageComponent{}
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content, Components: &empty}); err != nil {
		log.Printf("Error reporting playlist delete: %v", err)
	}
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Components: components, Flags: discordgo.MessageFlagsEphemeral},
	})
	if err != nil {
		log.Printf("Error sending playlist reply: %v", err)
	}
}

func describePlaylist(p site.Summary) string {
	state := ""
	if p.Deleted {
		state = " (deleted)"
	}
	return fmt.Sprintf("**%s**%s\n%d songs, created %s, updated %s", p.Name, state, p.EntryCount, p.CreatedAt, p.UpdatedAt)
}

func (h *CommandHandler) siteError(err error) string {
	switch {
	case errors.Is(err, site.ErrNotFound):
		return "No playlist with that id"
	case errors.Is(err, site.ErrForbidden):
		log.Printf("Site rejected the admin secret")
		return "The site rejected the bot's admin secret, ask the operator to check SITE_ADMIN_SECRET"
	default:
		log.Printf("Site call failed: %v", err)
		return "Could not reach the site, please try again"
	}
}
