package bot

import (
	"fmt"
	"log"

	"github.com/alwinihza/discord-bot/database"
	"github.com/alwinihza/discord-bot/internal/commands"
	"github.com/alwinihza/discord-bot/pkg/state"
	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session         *discordgo.Session
	db              *database.DB
	pm              *state.PaginationManager
	announceChannel string
}

func NewBot(token string, db *database.DB, pm *state.PaginationManager, announceChannel string) (*Bot, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	// Slash commands and buttons arrive as interactions, which need no gateway intents.
	s.Identify.Intents = discordgo.IntentsNone
	return &Bot{session: s, db: db, pm: pm, announceChannel: announceChannel}, nil
}

func (b *Bot) Start() error {
	b.registerHandlers()
	if err := b.session.Open(); err != nil {
		return err
	}
	// session.State fills in asynchronously on Ready, so ask the API for our ID.
	me, err := b.session.User("@me")
	if err == nil {
		_, err = b.session.ApplicationCommandBulkOverwrite(me.ID, "", commands.Definitions)
	}
	if err != nil {
		b.session.Close()
		return fmt.Errorf("register slash commands: %w", err)
	}
	return nil
}

func (b *Bot) Stop() error {
	return b.session.Close()
}

func (b *Bot) registerHandlers() {
	commandHandler := commands.NewCommandHandler(b.db, b.pm, b.announceChannel)
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
		case discordgo.InteractionApplicationCommandAutocomplete:
			commandHandler.Autocomplete(s, i)
		case discordgo.InteractionMessageComponent:
			commandHandler.HandleInteraction(s, i)
		case discordgo.InteractionModalSubmit:
			commandHandler.HandleModal(s, i)
		}
	})
}
