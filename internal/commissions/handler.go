package commissions

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/bwmarrin/discordgo"
)

const (
	noteField   = "note"
	maxNote     = 1000
	notManager  = "❌ Only commission managers can change a commission's status."
	staleButton = "❌ That button is out of date."
)

type Handler struct {
	site     *site.Client
	managers map[string]bool
}

func NewHandler(sc *site.Client, managers map[string]bool) *Handler {
	return &Handler{site: sc, managers: managers}
}

func actingUser(i *discordgo.InteractionCreate) string {
	switch {
	case i.Member != nil && i.Member.User != nil:
		return i.Member.User.ID
	case i.User != nil:
		return i.User.ID
	}
	return ""
}

func (h *Handler) IsManager(user string) bool { return user != "" && h.managers[user] }

func ephemeral(s *discordgo.Session, i *discordgo.InteractionCreate, text string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: text, Flags: discordgo.MessageFlagsEphemeral, AllowedMentions: noMentions},
	})
	if err != nil {
		log.Printf("Commissions: reply: %v", err)
	}
}

func (h *Handler) HandleButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	action, ok := ParseButtonID(i.MessageComponentData().CustomID)
	if !ok {
		ephemeral(s, i, staleButton)
		return
	}
	if !h.IsManager(actingUser(i)) {
		ephemeral(s, i, notManager)
		return
	}
	if needsNote(action.Status) {
		h.openNoteModal(s, i, action)
		return
	}
	h.apply(s, i, action, "")
}

func (h *Handler) HandleModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	action, ok := ParseModalID(data.CustomID)
	if !ok {
		ephemeral(s, i, staleButton)
		return
	}
	if !h.IsManager(actingUser(i)) {
		ephemeral(s, i, notManager)
		return
	}
	h.apply(s, i, action, modalNote(data))
}

func (h *Handler) openNoteModal(s *discordgo.Session, i *discordgo.InteractionCreate, a Action) {
	label := "Note (optional)"
	switch a.Status {
	case "quoted":
		label = "Price or quote note (optional)"
	case "declined":
		label = "Reason (optional)"
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: ModalID(a),
			Title:    truncate(StatusLabel(a.Status), 45),
			Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{CustomID: noteField, Label: label, Style: discordgo.TextInputParagraph, Required: false, MaxLength: maxNote},
			}}},
		},
	})
	if err != nil {
		log.Printf("Commissions: open note modal: %v", err)
	}
}

func modalNote(data discordgo.ModalSubmitInteractionData) string {
	for _, row := range data.Components {
		r, ok := row.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, c := range r.Components {
			if in, ok := c.(*discordgo.TextInput); ok && in.CustomID == noteField {
				return truncate(strings.TrimSpace(in.Value), maxNote)
			}
		}
	}
	return ""
}

func (h *Handler) apply(s *discordgo.Session, i *discordgo.InteractionCreate, a Action, note string) {
	user := actingUser(i)
	if i.Message == nil {
		ephemeral(s, i, staleButton)
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); err != nil {
		log.Printf("Commissions: acknowledge: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), siteTimeout)
	defer cancel()
	updated, err := h.site.SendEvent(ctx, user, a.ID, a.Status, note)
	if err != nil {
		h.followupError(s, i, err)
		return
	}

	embeds := []*discordgo.MessageEmbed{Embed(updated)}
	components := Components(updated)
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds, Components: &components}); err != nil {
		log.Printf("Commissions: edit %s: %v", a.ID, err)
	}
	// The thread of a message has the message's own id.
	if _, err := s.ChannelMessageSendComplex(i.Message.ID, &discordgo.MessageSend{
		Content: ThreadLine(updated.Status, user, note), AllowedMentions: noMentions,
	}); err != nil {
		log.Printf("Commissions: thread line for %s: %v", a.ID, err)
	}
}

func ThreadLine(status, user, note string) string {
	line := fmt.Sprintf("%s by <@%s>", StatusLabel(status), user)
	if note != "" {
		line += ": " + note
	}
	return truncate(line, 2000)
}

func (h *Handler) followupError(s *discordgo.Session, i *discordgo.InteractionCreate, err error) {
	var rule *site.RuleError
	text := "❌ Could not reach the site, try again in a moment."
	switch {
	case errors.As(err, &rule):
		text = "❌ The site refused that change: " + rule.Message
	case errors.Is(err, site.ErrCommissionNotFound):
		text = "❌ The site no longer knows this commission."
	default:
		log.Printf("Commissions: event: %v", err)
	}
	if _, ferr := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: truncate(text, 2000), Flags: discordgo.MessageFlagsEphemeral, AllowedMentions: noMentions,
	}); ferr != nil {
		log.Printf("Commissions: followup: %v", ferr)
	}
}
