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

	return &Bot{
		session: s,
		db:      db,
		pm:      pm,
	}, nil
}

func (b *Bot) Start() error {
	b.RegisterHandlers()
	return b.session.Open()
}

func (b *Bot) Stop() {
	b.session.Close()
}

func (b *Bot) readyHandler(s *discordgo.Session, r *discordgo.Ready) {
	log.Println("Bot is ready!")
}

func (b *Bot) RegisterHandlers() {
	commandHandler := commands.NewCommandHandler(b.db, b.pm)

	b.session.AddHandler(b.readyHandler)
	b.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if h, ok := commandHandler.GetCommandHandlers()[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		case discordgo.InteractionMessageComponent:
			commandHandler.HandleInteraction(s, i)
		}
	})
}

// Add other bot methods...
