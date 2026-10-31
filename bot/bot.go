package bot

import (
	"log"

	"github.com/alwinihza/discord-bot/database"
	"github.com/alwinihza/discord-bot/internal/commands"
	"github.com/alwinihza/discord-bot/pkg/state"
	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session *discordgo.Session
	db      *database.DB
	pm      *state.PaginationManager
}

func NewBot(token string, db *database.DB, pm *state.PaginationManager) (*Bot, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	// Slash commands and buttons arrive as interactions, which need no gateway intents.
	s.Identify.Intents = discordgo.IntentsNone
	return &Bot{session: s, db: db, pm: pm}, nil
}

func (b *Bot) Start() error {
	b.registerHandlers()
	return b.session.Open()
}

func (b *Bot) Stop() error {
	return b.session.Close()
}

func (b *Bot) registerHandlers() {
	commandHandler := commands.NewCommandHandler(b.db, b.pm)
	handlers := commandHandler.GetCommandHandlers()

	b.session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Bot is ready as %s", r.User.Username)
	})
	b.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if h, ok := handlers[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		case discordgo.InteractionMessageComponent:
			commandHandler.HandleInteraction(s, i)
		}
	})
}
