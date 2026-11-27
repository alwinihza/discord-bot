// Package golive posts a "live now" message when OBS starts streaming and
// edits it to "ended" when OBS stops.
//
// Discord is the only store. Each event reads the channel's recent messages
// to find the bot's newest go-live message, so a restart mid-stream loses
// nothing and a repeated event changes nothing.
package golive

import (
	"context"
	"log"
	"time"

	"github.com/alwinihza/discord-bot/internal/embeds"
	"github.com/alwinihza/discord-bot/internal/webhook"
	"github.com/alwinihza/discord-bot/models"
	"github.com/bwmarrin/discordgo"
)

const (
	historyLimit = 50
	callTimeout  = 10 * time.Second
	eventBuffer  = 8
)

// Discord is the part of *discordgo.Session the announcer uses, plus the
// bot's own user id.
type Discord interface {
	ChannelMessages(channelID string, limit int, beforeID, afterID, aroundID string, options ...discordgo.RequestOption) ([]*discordgo.Message, error)
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageEditComplex(m *discordgo.MessageEdit, options ...discordgo.RequestOption) (*discordgo.Message, error)
	BotUserID() (string, error)
}

type YouTube interface {
	ActiveLiveVideo(ctx context.Context, channelID string) (videoID string, found bool, err error)
	Fetch(ctx context.Context, id string) (models.VideoInfo, error)
}

type Config struct {
	ChannelID        string
	YouTubeChannelID string
	// PingRoleID is pinged on the live post when set.
	PingRoleID     string
	SearchInterval time.Duration
	SearchWindow   time.Duration
	Now            func() time.Time
}

type Announcer struct {
	cfg     Config
	discord Discord
	youtube YouTube
	events  chan webhook.Event
}

func New(cfg Config, d Discord, yt YouTube) *Announcer {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Announcer{cfg: cfg, discord: d, youtube: yt, events: make(chan webhook.Event, eventBuffer)}
}

// OnEvent queues e for Run. It never blocks, so it is safe to call from the
// webhook handler.
func (a *Announcer) OnEvent(e webhook.Event) {
	select {
	case a.events <- e:
	default:
		log.Printf("go-live: dropped %s event, %d events already queued", e, eventBuffer)
	}
}

// search is the lookup loop for one started event. The zero value means no
// search is running.
type search struct {
	tick   <-chan time.Time
	window <-chan time.Time
	stop   func()
}

func (s search) active() bool { return s.tick != nil }

// Run handles events until ctx is done.
func (a *Announcer) Run(ctx context.Context) {
	var s search
	end := func() {
		if s.active() {
			s.stop()
			s = search{}
		}
	}
	defer end()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-a.events:
			switch e {
			case webhook.Started:
				if s.active() || a.streamOpen(ctx) {
					continue
				}
				ticker := time.NewTicker(a.cfg.SearchInterval)
				window := time.NewTimer(a.cfg.SearchWindow)
				s = search{tick: ticker.C, window: window.C, stop: func() { ticker.Stop(); window.Stop() }}
				if a.lookup(ctx) {
					end()
				}
			case webhook.Stopped:
				end()
				a.close(ctx)
			}
		case <-s.tick:
			if a.lookup(ctx) {
				end()
			}
		case <-s.window:
			log.Printf("go-live: no live broadcast found for channel %s", a.cfg.YouTubeChannelID)
			end()
		}
	}
}

// newest returns the bot's newest go-live message in the channel, or nil.
func (a *Announcer) newest(ctx context.Context) (*discordgo.Message, embeds.GoLiveKind, models.VideoInfo, error) {
	me, err := a.discord.BotUserID()
	if err != nil {
		return nil, embeds.NotGoLive, models.VideoInfo{}, err
	}
	msgs, err := a.discord.ChannelMessages(a.cfg.ChannelID, historyLimit, "", "", "", discordgo.WithContext(ctx))
	if err != nil {
		return nil, embeds.NotGoLive, models.VideoInfo{}, err
	}
	for _, m := range msgs {
		if m.Author == nil || m.Author.ID != me {
			continue
		}
		if kind, video := embeds.ParseGoLive(m); kind != embeds.NotGoLive {
			return m, kind, video, nil
		}
	}
	return nil, embeds.NotGoLive, models.VideoInfo{}, nil
}

// streamOpen reports whether a live message is already up. A failed read
// counts as not open: lookup reads again before it posts.
func (a *Announcer) streamOpen(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	_, kind, _, err := a.newest(ctx)
	if err != nil {
		log.Printf("go-live: read channel history: %v", err)
		return false
	}
	if kind == embeds.Live {
		log.Println("go-live: started, but a live message is already open")
	}
	return kind == embeds.Live
}

// lookup tries once to find the broadcast and post it. It reports whether
// the search is over.
func (a *Announcer) lookup(ctx context.Context) bool {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	id, found, err := a.youtube.ActiveLiveVideo(callCtx, a.cfg.YouTubeChannelID)
	if err != nil {
		log.Printf("go-live: look up live broadcast: %v", err)
		return false
	}
	if !found {
		return false
	}

	// Read again right before posting: a failed send that Discord did
	// accept must not turn into a second post on the next tick.
	_, kind, last, err := a.newest(callCtx)
	if err != nil {
		log.Printf("go-live: read channel history: %v", err)
		return false
	}
	switch {
	case kind == embeds.Live:
		return true
	case kind == embeds.Ended && last.ID == id:
		log.Printf("go-live: %s already ended, not posting it again", id)
		return true
	}

	video, err := a.youtube.Fetch(callCtx, id)
	if err != nil {
		log.Printf("go-live: fetch %s: %v", id, err)
		return false
	}
	send := &discordgo.MessageSend{
		Embeds:          []*discordgo.MessageEmbed{embeds.CreateLiveEmbed(video)},
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	}
	if role := a.cfg.PingRoleID; role != "" {
		send.Content = "<@&" + role + ">"
		send.AllowedMentions.Roles = []string{role}
	}
	m, err := a.discord.ChannelMessageSendComplex(a.cfg.ChannelID, send, discordgo.WithContext(callCtx))
	if err != nil {
		log.Printf("go-live: post live message for %s: %v", id, err)
		return false
	}
	log.Printf("go-live: posted live message %s for %s", m.ID, id)
	return true
}

// close edits the open live message, if any, to the ended embed and drops
// the ping.
func (a *Announcer) close(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	m, kind, video, err := a.newest(ctx)
	if err != nil {
		log.Printf("go-live: read channel history: %v", err)
		return
	}
	if kind != embeds.Live {
		log.Println("go-live: stopped, but no live message is open")
		return
	}
	empty := ""
	ended := []*discordgo.MessageEmbed{embeds.CreateEndedEmbed(video, a.cfg.Now().Sub(m.Timestamp))}
	edit := &discordgo.MessageEdit{
		ID: m.ID, Channel: a.cfg.ChannelID,
		Content: &empty, Embeds: &ended, AllowedMentions: &discordgo.MessageAllowedMentions{},
	}
	if _, err := a.discord.ChannelMessageEditComplex(edit, discordgo.WithContext(ctx)); err != nil {
		log.Printf("go-live: mark %s ended: %v", m.ID, err)
		return
	}
	log.Printf("go-live: marked %s ended", m.ID)
}

// Session adapts a discordgo session to Discord.
type Session struct{ *discordgo.Session }

// BotUserID reads the id from the gateway's Ready state, and asks the API
// while Ready has not arrived yet.
func (s Session) BotUserID() (string, error) {
	s.State.RLock()
	u := s.State.User
	s.State.RUnlock()
	if u != nil {
		return u.ID, nil
	}
	u, err := s.User("@me")
	if err != nil {
		return "", err
	}
	return u.ID, nil
}
