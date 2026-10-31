package commands

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alwinihza/discord-bot/database"
	"github.com/alwinihza/discord-bot/internal/embeds"
	"github.com/alwinihza/discord-bot/models"
	"github.com/alwinihza/discord-bot/pkg/state"
	"github.com/alwinihza/discord-bot/shared"
	"github.com/bwmarrin/discordgo"
)

const (
	itemsPerPage = 10
	// Discord drops an interaction that is not answered within 3 seconds.
	queryTimeout = 2500 * time.Millisecond
)

type CommandHandler struct {
	db *database.DB
	pm *state.PaginationManager
}

func NewCommandHandler(db *database.DB, pm *state.PaginationManager) *CommandHandler {
	return &CommandHandler{db: db, pm: pm}
}

func (h *CommandHandler) GetCommandHandlers() map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate) {
	return map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"list-video": h.ListVideo,
		"random":     h.Random,
		"search":     h.Search,
		"stats":      h.Stats,
	}
}

func (h *CommandHandler) HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch id := i.MessageComponentData().CustomID; id {
	case "close":
		h.handleClose(s, i)
	case "random":
		h.respondRandom(s, i, discordgo.InteractionResponseUpdateMessage)
	default:
		for _, kind := range []models.PaginationKind{models.KindList, models.KindSearch} {
			switch id {
			case string(kind) + "prev_page":
				h.handlePage(s, i, kind, -1)
			case string(kind) + "next_page":
				h.handlePage(s, i, kind, +1)
			}
		}
	}
}

func (h *CommandHandler) ListVideo(s *discordgo.Session, i *discordgo.InteractionCreate) {
	h.startPagination(s, i, models.PaginationState{Kind: models.KindList})
}

func (h *CommandHandler) Search(s *discordgo.Session, i *discordgo.InteractionCreate) {
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "search" {
			h.startPagination(s, i, models.PaginationState{
				Kind:        models.KindSearch,
				SearchValue: strings.TrimSpace(opt.StringValue()),
			})
			return
		}
	}
	shared.RespondWithError(s, i, "Please provide a song title to search for")
}

func (h *CommandHandler) Random(s *discordgo.Session, i *discordgo.InteractionCreate) {
	h.respondRandom(s, i, discordgo.InteractionResponseChannelMessageWithSource)
}

// fetchPage loads one page of st's view and returns its embed and the
// current page count, so pagination tracks rows added after the first page.
func (h *CommandHandler) fetchPage(st models.PaginationState, page int64) (*discordgo.MessageEmbed, int64, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	switch st.Kind {
	case models.KindList:
		videos, total, err := h.db.FetchVideos(ctx, page, itemsPerPage)
		pages := pageCount(total)
		return embeds.CreateVideoEmbed(videos, page, pages), total, pages, err
	case models.KindSearch:
		songs, total, err := h.db.FetchSearchResult(ctx, st.SearchValue, page, itemsPerPage)
		pages := pageCount(total)
		return embeds.CreateSongEmbed(songs, st.SearchValue, page, pages), total, pages, err
	}
	return nil, 0, 0, fmt.Errorf("unknown pagination kind %q", st.Kind)
}

func pageCount(total int64) int64 {
	return max(1, (total+itemsPerPage-1)/itemsPerPage)
}

func (h *CommandHandler) startPagination(s *discordgo.Session, i *discordgo.InteractionCreate, st models.PaginationState) {
	embed, total, pages, err := h.fetchPage(st, 1)
	if err != nil {
		log.Printf("Error fetching %s page: %v", st.Kind, err)
		shared.RespondWithError(s, i, "Failed to fetch data, please try again")
		return
	}
	if total == 0 {
		msg := "No karaoke videos yet"
		if st.Kind == models.KindSearch {
			msg = fmt.Sprintf("No songs found for %q", st.SearchValue)
		}
		shared.RespondWithError(s, i, msg)
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: embeds.CreatePaginationButtons(1, pages, string(st.Kind)),
		},
	})
	if err != nil {
		log.Printf("Error sending %s response: %v", st.Kind, err)
		return
	}

	msg, err := s.InteractionResponse(i.Interaction)
	if err != nil {
		log.Printf("Error reading %s response message: %v", st.Kind, err)
		return
	}
	st.CurrentPage, st.TotalPages, st.CreatedAt = 1, pages, time.Now()
	h.pm.Set(msg.ID, st)
}

func (h *CommandHandler) handlePage(s *discordgo.Session, i *discordgo.InteractionCreate, kind models.PaginationKind, delta int64) {
	msgID := i.Message.ID
	st, ok := h.pm.Get(msgID)
	if !ok || st.Kind != kind {
		shared.RespondWithError(s, i, "This list has expired, please run the command again")
		return
	}

	page := min(max(1, st.CurrentPage+delta), st.TotalPages)
	embed, _, pages, err := h.fetchPage(st, page)
	if err != nil {
		log.Printf("Error fetching %s page %d: %v", kind, page, err)
		shared.RespondWithError(s, i, "Failed to fetch data, please try again")
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: embeds.CreatePaginationButtons(page, pages, string(kind)),
		},
	})
	if err != nil {
		log.Printf("Error updating %s message: %v", kind, err)
		return
	}
	st.CurrentPage, st.TotalPages = page, pages
	h.pm.Set(msgID, st)
}

func (h *CommandHandler) handleClose(s *discordgo.Session, i *discordgo.InteractionCreate) {
	h.pm.Delete(i.Message.ID)
	// Acknowledge this click, then delete through its own token: for a component
	// interaction the "original response" is the message the button sits on.
	// The token of the slash command that created it expires after 15 minutes.
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	if err != nil {
		log.Printf("Error acknowledging close: %v", err)
		return
	}
	if err := s.InteractionResponseDelete(i.Interaction); err != nil {
		log.Printf("Error deleting message: %v", err)
	}
}

func (h *CommandHandler) respondRandom(s *discordgo.Session, i *discordgo.InteractionCreate, respType discordgo.InteractionResponseType) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	song, err := h.db.FetchRandomSong(ctx)
	if err != nil {
		log.Printf("Error fetching random song: %v", err)
		shared.RespondWithError(s, i, "Failed to fetch a random song, please try again")
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: respType,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🎵 **%s**\n%s", song.Label(), song.URL),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Get Another Random Song",
							Style:    discordgo.PrimaryButton,
							Emoji:    &discordgo.ComponentEmoji{Name: "🎲"},
							CustomID: "random",
						},
					},
				},
			},
		},
	})
	if err != nil {
		log.Printf("Error sending random song: %v", err)
	}
}
