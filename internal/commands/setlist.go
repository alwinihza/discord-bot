package commands

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alwinihza/discord-bot/internal/embeds"
	"github.com/alwinihza/discord-bot/internal/setlist"
	"github.com/alwinihza/discord-bot/internal/youtube"
	"github.com/alwinihza/discord-bot/models"
	"github.com/alwinihza/discord-bot/shared"
	"github.com/bwmarrin/discordgo"
)

const (
	setlistModalPrefix = "setlist:"
	maxSetlistText     = 4000 // Discord's text input limit
	// A modal submit is deferred first, so saving may take longer than 3s.
	saveTimeout = 15 * time.Second
)

// Setlist opens a form holding the stream's current setlist for a mod to edit.
func (h *CommandHandler) Setlist(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var raw string
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "video" {
			raw = opt.StringValue()
		}
	}
	videoID, ok := setlist.VideoID(raw)
	if !ok {
		shared.RespondWithError(s, i, fmt.Sprintf("%q is not a YouTube link or video ID", embeds.Truncate(raw, 100)))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	title, entries, found, err := h.db.Setlist(ctx, videoID)
	if err != nil {
		log.Printf("Error loading setlist of %s: %v", videoID, err)
		shared.RespondWithError(s, i, "Failed to load the setlist, please try again")
		return
	}
	text := setlist.Format(entries)
	if len(text) > maxSetlistText {
		shared.RespondWithError(s, i, "This setlist is too long to edit in Discord")
		return
	}
	if !found {
		title = "New stream " + videoID
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: setlistModalPrefix + videoID,
			Title:    embeds.Truncate(title, 45),
			Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    "songs",
					Label:       "One song per line: time | title | artist",
					Style:       discordgo.TextInputParagraph,
					Value:       text,
					Placeholder: "00:13:22 | Halu | Feby Putri\n00:20:42 | Runtuh | Feby Putri | acoustic",
					Required:    true,
					MaxLength:   maxSetlistText,
				},
			}}},
		},
	})
	if err != nil {
		log.Printf("Error opening setlist form: %v", err)
	}
}

// SetlistSubmit saves the edited setlist and reports back to the mod only.
func (h *CommandHandler) SetlistSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	videoID := strings.TrimPrefix(data.CustomID, setlistModalPrefix)
	text, ok := modalText(data)
	if !ok {
		shared.RespondWithError(s, i, "The setlist form came back empty, please try again")
		return
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})
	if err != nil {
		log.Printf("Error acknowledging setlist: %v", err)
		return
	}
	// The form closes on submit, so every failure hands the text back.
	fail := func(msg string) {
		_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: embeds.Truncate("❌ Nothing was saved.\n"+msg, 2000),
			Files:   []*discordgo.File{{Name: "setlist.txt", ContentType: "text/plain", Reader: strings.NewReader(text)}},
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		if err != nil {
			log.Printf("Error reporting setlist failure: %v", err)
		}
	}

	entries, err := setlist.Parse(text)
	if err != nil {
		fail(fmt.Sprintf("Fix these lines and run `/setlist` again:\n%v", err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	_, _, found, err := h.db.Setlist(ctx, videoID)
	if err != nil {
		log.Printf("Error loading setlist of %s: %v", videoID, err)
		fail("Failed to read the database, please try again.")
		return
	}
	video := models.VideoInfo{ID: videoID}
	if !found {
		video, err = h.youtube.Fetch(ctx, videoID)
		if err != nil {
			log.Printf("Error fetching video %s: %v", videoID, err)
			if errors.Is(err, youtube.ErrNotFound) {
				fail("YouTube has no public video with this ID. Check the link.")
			} else {
				fail("Could not read this video's title and date from YouTube, please try again.")
			}
			return
		}
	}

	res, err := h.db.SaveSetlist(ctx, video, entries)
	if err != nil {
		log.Printf("Error saving setlist of %s: %v", videoID, err)
		fail("Failed to save, please try again.")
		return
	}
	log.Printf("%s saved %d songs for %s, replacing:\n%s",
		i.Member.User.Username, len(entries), videoID, setlist.Format(res.Previous))

	msg := fmt.Sprintf("✅ Saved %d songs for **%s** (was %d).", len(entries), res.Title, len(res.Previous))
	if res.Created && h.announceChannel != "" {
		_, err := s.ChannelMessageSendEmbed(h.announceChannel, embeds.CreateNewStreamEmbed(video, entries))
		if err != nil {
			log.Printf("Error announcing %s: %v", videoID, err)
			msg += "\n⚠️ Could not post the announcement, check the bot can send messages and embeds there."
		} else {
			msg += fmt.Sprintf("\n📣 Announced in <#%s>.", h.announceChannel)
		}
	}
	if len(res.NewSongs) > 0 {
		msg += "\nFirst time in the bot, check the spelling:\n- " + strings.Join(res.NewSongs, "\n- ")
	}
	_, err = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: embeds.Truncate(msg, 2000),
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		log.Printf("Error reporting saved setlist: %v", err)
	}
}

func modalText(data discordgo.ModalSubmitInteractionData) (string, bool) {
	for _, c := range data.Components {
		if row, ok := c.(*discordgo.ActionsRow); ok {
			for _, c := range row.Components {
				if input, ok := c.(*discordgo.TextInput); ok && input.CustomID == "songs" {
					return input.Value, true
				}
			}
		}
	}
	return "", false
}

func (h *CommandHandler) HandleModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if strings.HasPrefix(i.ModalSubmitData().CustomID, setlistModalPrefix) {
		h.SetlistSubmit(s, i)
	}
}
