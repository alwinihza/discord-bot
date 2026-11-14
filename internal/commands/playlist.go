package commands

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"

	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/bwmarrin/discordgo"
)

const (
	playlistConfirmPrefix = "playlistmod:confirm:"
	playlistCancelID      = "playlistmod:cancel"
	// Every site call runs after the interaction is acknowledged, so it may outlast Discord's 3s window.
	siteTimeout = 10 * time.Second
)

var noMentions = &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}

func isMod(i *discordgo.InteractionCreate) bool {
	return i.Member != nil && i.Member.Permissions&discordgo.PermissionManageMessages != 0
}

func (h *CommandHandler) PlaylistMod(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !isMod(i) {
		respondPlaylist(s, i, "❌ Only mods can use this command", nil)
		return
	}
	if h.site == nil {
		respondPlaylist(s, i, "❌ Playlist moderation is not set up on this bot", nil)
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
		respondPlaylist(s, i, "❌ That is not a playlist link or id", nil)
		return
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral, AllowedMentions: noMentions},
	}); err != nil {
		log.Printf("Error acknowledging playlist-mod: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), siteTimeout)
	defer cancel()
	summary, err := h.site.Info(ctx, id)
	if err != nil {
		editPlaylist(s, i, "❌ "+h.siteError(err), nil)
		return
	}

	switch sub.Name {
	case "info":
		editPlaylist(s, i, describePlaylist(summary), nil)
	case "delete":
		if summary.Deleted {
			editPlaylist(s, i, describePlaylist(summary)+"\nIt is already deleted.", nil)
			return
		}
		editPlaylist(s, i, "Delete this playlist? Its link stops working.\n"+describePlaylist(summary), []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Delete", Style: discordgo.DangerButton, CustomID: playlistConfirmPrefix + id},
				discordgo.Button{Label: "Cancel", Style: discordgo.SecondaryButton, CustomID: playlistCancelID},
			}},
		})
	}
}

func (h *CommandHandler) handlePlaylistButton(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	if customID == playlistCancelID {
		cancelPlaylistDelete(s, i)
		return
	}

	id := strings.TrimPrefix(customID, playlistConfirmPrefix)
	if h.site == nil || !isMod(i) {
		respondPlaylist(s, i, "❌ Only mods can delete playlists", nil)
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
		Data: &discordgo.InteractionResponseData{AllowedMentions: noMentions},
	}); err != nil {
		log.Printf("Error acknowledging playlist delete: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), siteTimeout)
	defer cancel()
	summary, err := h.site.Delete(ctx, id)
	content := "Deleted.\n" + describePlaylist(summary)
	if err != nil {
		content = "❌ " + h.siteError(err)
	}
	editPlaylist(s, i, content, []discordgo.MessageComponent{})
}

func cancelPlaylistDelete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:         "Cancelled. The playlist is unchanged.",
			Components:      []discordgo.MessageComponent{},
			AllowedMentions: noMentions,
		},
	})
	if err != nil {
		log.Printf("Error cancelling playlist delete: %v", err)
	}
}

func respondPlaylist(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Components: components, Flags: discordgo.MessageFlagsEphemeral, AllowedMentions: noMentions},
	})
	if err != nil {
		log.Printf("Error sending playlist reply: %v", err)
	}
}

func editPlaylist(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	if components == nil {
		components = []discordgo.MessageComponent{}
	}
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content, Components: &components, AllowedMentions: noMentions}); err != nil {
		log.Printf("Error editing playlist reply: %v", err)
	}
}

// escapeMarkdown backslash-escapes every ASCII punctuation mark, which Discord renders literally, and flattens newlines.
func escapeMarkdown(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r':
			b.WriteByte(' ')
		case r < unicode.MaxASCII && unicode.IsPunct(r) || r == '`' || r == '|' || r == '~' || r == '<' || r == '>' || r == '+' || r == '=' || r == '$' || r == '^':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func describePlaylist(p site.Summary) string {
	state := ""
	if p.Deleted {
		state = " (deleted)"
	}
	return fmt.Sprintf("**%s**%s\n%d songs, created %s, updated %s", escapeMarkdown(p.Name), state, p.EntryCount, p.CreatedAt, p.UpdatedAt)
}

func (h *CommandHandler) siteError(err error) string {
	var user escapedError
	var rule *site.RuleError
	switch {
	case errors.As(err, &user):
		return string(user)
	case errors.As(err, &rule):
		return escapeMarkdown(rule.Message)
	case errors.Is(err, site.ErrInvalidID):
		return "That is not a playlist link or id"
	case errors.Is(err, site.ErrNotFound):
		return "No playlist with that id"
	case errors.Is(err, site.ErrNotOwner):
		return "That playlist belongs to someone else"
	case errors.Is(err, site.ErrDeleted):
		return "That playlist was deleted"
	case errors.Is(err, site.ErrConflict):
		return "The playlist changed while you were editing it, please try again"
	case errors.Is(err, site.ErrRateLimited):
		return "Slow down: the site limits how often playlists can be created and edited. Try again later"
	case errors.Is(err, site.ErrDailyCap):
		return "The site reached its limit for new playlists today, try again tomorrow"
	case errors.Is(err, site.ErrNoOwnerAPI):
		log.Printf("Site has no owner routes yet")
		return "The site does not support playlists from Discord yet, try again after its next update"
	case errors.Is(err, site.ErrForbidden):
		log.Printf("Site rejected the admin secret")
		return "The site rejected the bot's admin secret, ask the operator to check SITE_ADMIN_SECRET"
	default:
		log.Printf("Site call failed: %v", err)
		return "Could not reach the site, please try again"
	}
}
