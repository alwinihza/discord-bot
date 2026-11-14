package commissions

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/bwmarrin/discordgo"
)

const (
	PollInterval         = 30 * time.Second
	siteTimeout          = 10 * time.Second
	threadArchiveMinutes = 10080
	seedHistory          = 100
	footerPrefix         = "Commission "
)

var noMentions = &discordgo.MessageAllowedMentions{}

type posting struct {
	messageID      string
	done           bool
	foundAtStartup bool
}

type Poller struct {
	site      *site.Client
	session   *discordgo.Session
	channelID string
	interval  time.Duration
	sent      map[string]*posting
}

func NewPoller(sc *site.Client, s *discordgo.Session, channelID string) *Poller {
	return &Poller{site: sc, session: s, channelID: channelID, interval: PollInterval, sent: map[string]*posting{}}
}

// Run polls until ctx ends. It returns once the in-flight tick has finished.
func (p *Poller) Run(ctx context.Context) {
	p.adoptMessagesFromBeforeRestart()
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		p.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Poller) adoptMessagesFromBeforeRestart() {
	me, err := p.session.User("@me")
	if err != nil {
		log.Printf("Commissions: seed: look up own user: %v", err)
		return
	}
	msgs, err := p.session.ChannelMessages(p.channelID, seedHistory, "", "", "")
	if err != nil {
		log.Printf("Commissions: seed: read channel history: %v", err)
		return
	}
	// History is newest first, so the oldest message of a commission wins.
	for _, m := range msgs {
		if m.Author == nil || m.Author.ID != me.ID {
			continue
		}
		for _, e := range m.Embeds {
			if e.Footer == nil {
				continue
			}
			id, ok := strings.CutPrefix(e.Footer.Text, footerPrefix)
			if !ok || !site.ValidCommissionID(id) {
				continue
			}
			p.sent[id] = &posting{messageID: m.ID, foundAtStartup: true}
			break
		}
	}
}

// Tick posts every unposted commission. One failure is logged and the rest go on.
func (p *Poller) Tick(ctx context.Context) {
	listCtx, cancel := context.WithTimeout(ctx, siteTimeout)
	list, err := p.site.Unposted(listCtx)
	cancel()
	if err != nil {
		log.Printf("Commissions: list unposted: %v", err)
		return
	}
	for _, c := range list {
		if ctx.Err() != nil {
			return
		}
		if err := p.post(ctx, c); err != nil {
			log.Printf("Commissions: post %s: %v", c.ID, err)
		}
	}
}

func (p *Poller) post(ctx context.Context, c site.Commission) error {
	if !site.ValidCommissionID(c.ID) {
		return errors.New("commission id is not safe to use")
	}
	sent := p.sent[c.ID]
	if sent == nil {
		msg, err := p.session.ChannelMessageSendComplex(p.channelID, &discordgo.MessageSend{
			Embeds:          []*discordgo.MessageEmbed{Embed(c)},
			Components:      Components(c),
			AllowedMentions: noMentions,
		})
		if err != nil {
			return fmt.Errorf("send message: %w", err)
		}
		sent = &posting{messageID: msg.ID}
		p.sent[c.ID] = sent
		if _, err := p.session.MessageThreadStartComplex(p.channelID, msg.ID, &discordgo.ThreadStart{
			Name:                threadName(c),
			AutoArchiveDuration: threadArchiveMinutes,
		}); err != nil {
			log.Printf("Commissions: start thread for %s: %v", c.ID, err)
		}
	}
	if sent.done {
		return nil
	}

	postedCtx, cancel := context.WithTimeout(ctx, siteTimeout)
	defer cancel()
	err := p.site.MarkPosted(postedCtx, c.ID, p.channelID, sent.messageID)
	switch {
	case err == nil:
		sent.done = true
	case errors.Is(err, site.ErrCommissionConflict):
		sent.done = true
		if sent.foundAtStartup {
			log.Printf("Commissions: site already has another message for %s; leaving %s", c.ID, sent.messageID)
			return nil
		}
		if _, derr := p.session.ChannelDelete(sent.messageID); derr != nil {
			log.Printf("Commissions: delete duplicate thread of %s: %v", c.ID, derr)
		}
		if derr := p.session.ChannelMessageDelete(p.channelID, sent.messageID); derr != nil {
			return fmt.Errorf("delete duplicate message: %w", derr)
		}
	default:
		return fmt.Errorf("mark posted (retries next tick): %w", err)
	}
	return nil
}

func threadName(c site.Commission) string {
	id := c.ID
	if len(id) > 8 {
		id = id[:8]
	}
	return truncate(kindLabel(c.Kind)+" "+id, 100)
}
