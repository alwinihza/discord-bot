package commands

import (
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

const ITEMS_PER_PAGE = 10

type CommandHandler struct {
	db *database.DB
	pm *state.PaginationManager
}

func NewCommandHandler(db *database.DB, pm *state.PaginationManager) *CommandHandler {
	return &CommandHandler{
		db: db,
		pm: pm,
	}
}

// Add this to your CommandHandler struct
func (h *CommandHandler) GetCommandHandlers() map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate) {
	return map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"list-video": h.ListVideo(),
		"random":     h.Random(),
		"search":     h.Search(),
		// Add other commands here
	}
}

func (h *CommandHandler) HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.MessageComponentData().CustomID {
	case "listprev_page", "listnext_page", "close":
		h.HandlePagination(s, i)
	case "searchprev_page", "searchnext_page":
		h.HandleSearchPagination(s, i)
	case "random":
		h.HandleRandom(s, i)
	default:
		return
	}
}

func (h *CommandHandler) ListVideo() func(s *discordgo.Session, i *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		// Fetch initial data
		videos, totalCount, err := h.db.FetchVideos(1, ITEMS_PER_PAGE)
		if err != nil {
			shared.RespondWithError(s, i, "Failed to fetch videos")
			return
		}

		totalPages := (totalCount + ITEMS_PER_PAGE - 1) / ITEMS_PER_PAGE
		currentPage := int64(1)

		// Create initial response
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds:     []*discordgo.MessageEmbed{embeds.CreateVideoEmbed(videos, currentPage, totalPages)},
				Components: embeds.CreatePaginationButtons(currentPage, totalPages, "list"),
			},
		})

		if err != nil {
			log.Printf("Error sending initial response: %v", err)
			return
		}

		// Store pagination state
		msg, _ := s.InteractionResponse(i.Interaction)
		h.pm.Set(msg.ID, &models.PaginationState{
			CurrentPage:  currentPage,
			TotalPages:   totalPages,
			TotalItems:   totalCount,
			ItemsPerPage: ITEMS_PER_PAGE,
			Interaction:  i.Interaction,
			CreatedAt:    time.Now(),
		})
	}
}

func (h *CommandHandler) HandlePagination(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("Handling pagination for message ID: %s", i.Message.ID)

	msgID := i.Message.ID

	state, exists := h.pm.Get(msgID)
	if !exists {
		shared.RespondWithError(s, i, "Pagination expired or invalid")
		return
	}

	var newPage int64
	switch i.MessageComponentData().CustomID {
	case "listprev_page":
		newPage = state.CurrentPage - 1
	case "listnext_page":
		newPage = state.CurrentPage + 1
	case "close":
		fmt.Println("closing...")
		err := s.InteractionResponseDelete(state.Interaction)
		if err != nil {
			log.Printf("Error responding to cancel: %v", err)
		}
		h.pm.Delete(msgID) // Clean up the session
		return
	default:
		return
	}

	var (
		videos []models.VideoSong
		err    error
	)

	if strings.HasPrefix(i.Message.Embeds[0].Title, "Karaoke Video List") {
		videos, _, err = h.db.FetchVideos(newPage, state.ItemsPerPage)
	}

	if err != nil {
		log.Printf("Error fetching paginated data: %v", err)
		shared.RespondWithError(s, i, "Failed to fetch paginated data")
		return
	}

	buttons := embeds.CreatePaginationButtons(newPage, state.TotalPages, "list")

	// Acknowledge the interaction
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embeds.CreateVideoEmbed(videos, newPage, state.TotalPages)},
			Components: buttons,
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})

	if err != nil {
		log.Printf("Error updating interaction: %v", err)
		return
	}
	log.Printf("Successfully updated to page %d", newPage)
	success := h.pm.UpdateCurrentPage(msgID, newPage, i.Interaction)
	if !success {
		log.Printf("Error updating state: %v", err)
	}
}

func (h *CommandHandler) Random() func(s *discordgo.Session, i *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		ret, err := h.db.FetchRandomSong()
		if err != nil {
			log.Printf("Error fetch random song: %v", err)
			return
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("🎵 **%s**\n%s", ret.Title, ret.URL),
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.Button{
								Label:    "Get Another Random Song",
								Style:    discordgo.PrimaryButton,
								Emoji:    discordgo.ComponentEmoji{Name: "🎲"}, // Emoji defined here
								CustomID: "random",
							},
						},
					},
				},
			},
		})
	}
}

func (h *CommandHandler) HandleRandom(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ret, err := h.db.FetchRandomSong()
	if err != nil {
		log.Printf("Error fetch random song: %v", err)
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🎵 **%s**\n%s", ret.Title, ret.URL),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Get Another Random Song",
							Style:    discordgo.PrimaryButton,
							Emoji:    discordgo.ComponentEmoji{Name: "🎲"},
							CustomID: "random",
						},
					},
				},
			},
		},
	})

	if err != nil {
		log.Printf("Error updating interaction: %v", err)
		return
	}
}

func (h *CommandHandler) Search() func(s *discordgo.Session, i *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		options := i.ApplicationCommandData().Options
		optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))

		for _, opt := range options {
			optionMap[opt.Name] = opt
		}

		if option, ok := optionMap["search"]; ok {
			search := option.StringValue()

			// Fetch initial page of results
			videos, totalCount, err := h.db.FetchSearchResult(search, 1, ITEMS_PER_PAGE)
			if err != nil {
				shared.RespondWithError(s, i, "Failed to fetch videos")
				return
			}

			totalPages := (totalCount + ITEMS_PER_PAGE - 1) / ITEMS_PER_PAGE
			currentPage := int64(1)

			// Create and send initial response
			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Embeds:     []*discordgo.MessageEmbed{embeds.CreateSongEmbed(videos, currentPage, totalPages)},
					Components: embeds.CreatePaginationButtons(currentPage, totalPages, "search"),
				},
			})

			if err != nil {
				log.Printf("Error sending initial response: %v", err)
				return
			}

			// Store pagination state for future interactions
			msg, _ := s.InteractionResponse(i.Interaction)
			h.pm.Set(msg.ID, &models.PaginationState{
				CurrentPage:  currentPage,
				TotalPages:   totalPages,
				TotalItems:   totalCount,
				ItemsPerPage: ITEMS_PER_PAGE,
				Interaction:  i.Interaction,
				SearchValue:  search,
				CreatedAt:    time.Now(),
			})
		}
	}
}

func (h *CommandHandler) HandleSearchPagination(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("Handling pagination for message ID: %s", i.Message.ID)

	msgID := i.Message.ID

	state, exists := h.pm.Get(msgID)
	if !exists {
		shared.RespondWithError(s, i, "Pagination expired or invalid")
		return
	}

	var newPage int64
	switch i.MessageComponentData().CustomID {
	case "searchprev_page":
		newPage = state.CurrentPage - 1
	case "searchnext_page":
		newPage = state.CurrentPage + 1
	default:
		return
	}

	// Fetch initial page of results
	videos, _, err := h.db.FetchSearchResult(state.SearchValue, newPage, ITEMS_PER_PAGE)
	if err != nil {
		shared.RespondWithError(s, i, "Failed to fetch videos")
		return
	}

	buttons := embeds.CreatePaginationButtons(newPage, state.TotalPages, "search")

	// Acknowledge the interaction
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embeds.CreateSongEmbed(videos, newPage, state.TotalPages)},
			Components: buttons,
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})

	if err != nil {
		log.Printf("Error updating interaction: %v", err)
		return
	}
	log.Printf("Successfully updated to page %d", newPage)
	success := h.pm.UpdateCurrentPage(msgID, newPage, i.Interaction)
	if !success {
		log.Printf("Error updating state: %v", err)
	}
}

// Add other command handlers...
