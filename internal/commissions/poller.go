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

// posting is what this process knows about one commission it has sent to Discord.
// It outlives a failed /posted call so the next tick retries /posted instead of sending again.
type posting struct {
	messageID string
	done      bool
	// adopted marks a message found in the channel at startup rather than sent this run.
	adopted bool
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
	p.seed()
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

// seed adopts commission messages this bot already sent, so a restart between the send and
// /posted retries /posted instead of sending a second message. Discord is the source of truth.
func (p *Poller) seed() {
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
			p.sent[id] = &posting{messageID: m.ID, adopted: true}
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
		// A thread started on a message shares its id, which is how the handler finds it later.
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
		if sent.adopted {
			// The site holds a different message. This one was not sent this run, so leave it.
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
