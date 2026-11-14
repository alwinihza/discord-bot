package bot

import (
	"context"
	"fmt"
	"log"

	"github.com/alwinihza/discord-bot/database"
	"github.com/alwinihza/discord-bot/internal/commands"
	"github.com/alwinihza/discord-bot/internal/commissions"
	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/alwinihza/discord-bot/internal/youtube"
	"github.com/alwinihza/discord-bot/pkg/state"
	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session         *discordgo.Session
	db              *database.DB
	pm              *state.PaginationManager
	announceChannel string
	youtube         *youtube.Client
	site            *site.Client

	commissions *CommissionsConfig
	handler     *commissions.Handler
	stopPoller  context.CancelFunc
	pollerDone  chan struct{}
}

// CommissionsConfig switches the commission feature on. The bot needs a site client for it.
type CommissionsConfig struct {
	ChannelID string
	Managers  map[string]bool
}

func NewBot(token string, db *database.DB, pm *state.PaginationManager, announceChannel string, yt *youtube.Client, sc *site.Client, cc *CommissionsConfig) (*Bot, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	// Slash commands and buttons arrive as interactions, which need no gateway intents.
	s.Identify.Intents = discordgo.IntentsNone
	return &Bot{session: s, db: db, pm: pm, announceChannel: announceChannel, youtube: yt, site: sc, commissions: cc}, nil
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
	if b.handler != nil {
		var ctx context.Context
		ctx, b.stopPoller = context.WithCancel(context.Background())
		b.pollerDone = make(chan struct{})
		poller := commissions.NewPoller(b.site, b.session, b.commissions.ChannelID)
		go func() {
			defer close(b.pollerDone)
			poller.Run(ctx)
		}()
	}
	return nil
}

func (b *Bot) Stop() error {
	if b.stopPoller != nil {
		b.stopPoller()
		<-b.pollerDone
	}
	return b.session.Close()
}

func (b *Bot) registerHandlers() {
	commandHandler := commands.NewCommandHandler(b.db, b.pm, b.announceChannel, b.youtube, b.site)
	handlers := commandHandler.GetCommandHandlers()
	if b.commissions != nil && b.site != nil {
		b.handler = commissions.NewHandler(b.site, b.commissions.Managers)
	}

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
			if b.handler != nil && commissions.OwnsComponent(i.MessageComponentData().CustomID) {
				b.handler.HandleButton(s, i)
				return
			}
			commandHandler.HandleInteraction(s, i)
		case discordgo.InteractionModalSubmit:
			if b.handler != nil && commissions.OwnsComponent(i.ModalSubmitData().CustomID) {
				b.handler.HandleModal(s, i)
				return
			}
			commandHandler.HandleModal(s, i)
		}
	})
}
