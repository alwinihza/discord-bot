package golive

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alwinihza/discord-bot/internal/embeds"
	"github.com/alwinihza/discord-bot/internal/webhook"
	"github.com/alwinihza/discord-bot/models"
	"github.com/bwmarrin/discordgo"
)

const (
	botID     = "800000000000000001"
	channel   = "900000000000000005"
	ytChannel = "UCfiona"
	interval  = 2 * time.Millisecond
)

var start = time.Date(2026, 11, 27, 13, 0, 0, 0, time.UTC)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type fakeDiscord struct {
	mu     sync.Mutex
	clock  *clock
	msgs   []*discordgo.Message // newest first, like the API
	sends  []*discordgo.MessageSend
	edits  []*discordgo.MessageEdit
	reads  int
	nextID int
}

func (f *fakeDiscord) BotUserID() (string, error) { return botID, nil }

func (f *fakeDiscord) ChannelMessages(channelID string, limit int, _, _, _ string, _ ...discordgo.RequestOption) ([]*discordgo.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if channelID != channel {
		return nil, fmt.Errorf("read of unexpected channel %s", channelID)
	}
	out := make([]*discordgo.Message, 0, limit)
	for _, m := range f.msgs[:min(limit, len(f.msgs))] {
		c := *m
		out = append(out, &c)
	}
	return out, nil
}

func (f *fakeDiscord) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if channelID != channel {
		return nil, fmt.Errorf("send to unexpected channel %s", channelID)
	}
	f.sends = append(f.sends, data)
	f.nextID++
	m := &discordgo.Message{
		ID: fmt.Sprintf("m%d", f.nextID), ChannelID: channelID, Content: data.Content, Embeds: data.Embeds,
		Author: &discordgo.User{ID: botID}, Timestamp: f.clock.Now(),
	}
	f.msgs = append([]*discordgo.Message{m}, f.msgs...)
	return m, nil
}

func (f *fakeDiscord) ChannelMessageEditComplex(e *discordgo.MessageEdit, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.edits = append(f.edits, e)
	for _, m := range f.msgs {
		if m.ID == e.ID && e.Channel == channel {
			if e.Content != nil {
				m.Content = *e.Content
			}
			if e.Embeds != nil {
				m.Embeds = *e.Embeds
			}
			return m, nil
		}
	}
	return nil, errors.New("unknown message")
}

func (f *fakeDiscord) seed(m *discordgo.Message) {
	c := *m
	f.mu.Lock()
	f.msgs = append([]*discordgo.Message{&c}, f.msgs...)
	f.mu.Unlock()
}

func (f *fakeDiscord) counts() (sends, edits, reads int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends), len(f.edits), f.reads
}

type fakeYouTube struct {
	mu      sync.Mutex
	live    string // empty means the channel is not live
	lookups int
}

func (y *fakeYouTube) ActiveLiveVideo(_ context.Context, channelID string) (string, bool, error) {
	y.mu.Lock()
	defer y.mu.Unlock()
	y.lookups++
	if channelID != ytChannel {
		return "", false, fmt.Errorf("lookup of unexpected channel %s", channelID)
	}
	return y.live, y.live != "", nil
}

func (y *fakeYouTube) Fetch(_ context.Context, id string) (models.VideoInfo, error) {
	return video(id), nil
}

func (y *fakeYouTube) setLive(id string) {
	y.mu.Lock()
	y.live = id
	y.mu.Unlock()
}

func (y *fakeYouTube) count() int {
	y.mu.Lock()
	defer y.mu.Unlock()
	return y.lookups
}

func video(id string) models.VideoInfo {
	return models.VideoInfo{ID: id, Title: "Stream " + id, Thumbnail: "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"}
}

type rig struct {
	a     *Announcer
	d     *fakeDiscord
	yt    *fakeYouTube
	clock *clock
}

func newRig(t *testing.T, live, pingRole string, window time.Duration, seed ...*discordgo.Message) *rig {
	t.Helper()
	c := &clock{now: start}
	d := &fakeDiscord{clock: c}
	for _, m := range seed {
		d.seed(m)
	}
	yt := &fakeYouTube{live: live}
	a := New(Config{
		ChannelID: channel, YouTubeChannelID: ytChannel, PingRoleID: pingRole,
		SearchInterval: interval, SearchWindow: window, Now: c.Now,
	}, d, yt)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); a.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	return &rig{a: a, d: d, yt: yt, clock: c}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// handled sends e and waits until Run has read the channel for it.
func (r *rig) handled(t *testing.T, e webhook.Event) {
	t.Helper()
	_, _, before := r.d.counts()
	r.a.OnEvent(e)
	waitFor(t, string(e)+" to read the channel", func() bool { _, _, reads := r.d.counts(); return reads > before })
}

// settle gives the loop many intervals to do anything more it would do.
func settle() { time.Sleep(50 * interval) }

func botMessage(id string, at time.Time, embed *discordgo.MessageEmbed) *discordgo.Message {
	return &discordgo.Message{ID: id, ChannelID: channel, Author: &discordgo.User{ID: botID}, Timestamp: at, Embeds: []*discordgo.MessageEmbed{embed}}
}

func TestDoubleStartedPostsOnce(t *testing.T) {
	t.Run("second started after the post", func(t *testing.T) {
		r := newRig(t, "v1", "", time.Minute)
		r.a.OnEvent(webhook.Started)
		waitFor(t, "the live post", func() bool { s, _, _ := r.d.counts(); return s == 1 })
		lookups := r.yt.count()
		r.handled(t, webhook.Started)
		settle()
		if s, _, _ := r.d.counts(); s != 1 {
			t.Errorf("posts = %d, want 1", s)
		}
		if got := r.yt.count(); got != lookups {
			t.Errorf("second started looked up YouTube %d more times, want 0", got-lookups)
		}
	})
	t.Run("second started while searching", func(t *testing.T) {
		r := newRig(t, "", "", time.Minute)
		r.a.OnEvent(webhook.Started)
		r.a.OnEvent(webhook.Started)
		waitFor(t, "a few lookups", func() bool { return r.yt.count() >= 3 })
		r.yt.setLive("v1")
		waitFor(t, "the live post", func() bool { s, _, _ := r.d.counts(); return s == 1 })
		settle()
		if s, _, _ := r.d.counts(); s != 1 {
			t.Errorf("posts = %d, want 1", s)
		}
	})
}

func TestStoppedBeforeFoundEndsTheSearch(t *testing.T) {
	r := newRig(t, "", "", time.Minute)
	r.a.OnEvent(webhook.Started)
	waitFor(t, "a few lookups", func() bool { return r.yt.count() >= 3 })
	r.handled(t, webhook.Stopped)
	lookups := r.yt.count()
	r.yt.setLive("v1")
	settle()
	if got := r.yt.count(); got != lookups {
		t.Errorf("%d lookups after stopped, want 0", got-lookups)
	}
	if s, e, _ := r.d.counts(); s != 0 || e != 0 {
		t.Errorf("posts = %d, edits = %d, want none", s, e)
	}
}

func TestStartedFoundStoppedEditsThePost(t *testing.T) {
	r := newRig(t, "v1", "", time.Minute)
	r.a.OnEvent(webhook.Started)
	waitFor(t, "the live post", func() bool { s, _, _ := r.d.counts(); return s == 1 })
	r.clock.advance(2*time.Hour + 14*time.Minute)
	r.a.OnEvent(webhook.Stopped)
	waitFor(t, "the edit", func() bool { _, e, _ := r.d.counts(); return e == 1 })

	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	edit := r.d.edits[0]
	if edit.ID != r.d.msgs[0].ID || len(r.d.msgs) != 1 {
		t.Fatalf("edited %s, want the live post %s", edit.ID, r.d.msgs[0].ID)
	}
	kind, v := embeds.ParseGoLive(r.d.msgs[0])
	if kind != embeds.Ended || v.ID != "v1" {
		t.Errorf("message is %v for %q, want ended for v1", kind, v.ID)
	}
	if got := r.d.msgs[0].Embeds[0].Description; got != "Stream ended · 2h 14m" {
		t.Errorf("description = %q", got)
	}
}

func TestRestartReadsStateFromDiscord(t *testing.T) {
	open := botMessage("old1", start.Add(-90*time.Minute), embeds.CreateLiveEmbed(video("v1")))

	t.Run("stopped edits the open message", func(t *testing.T) {
		r := newRig(t, "", "", time.Minute, open)
		r.a.OnEvent(webhook.Stopped)
		waitFor(t, "the edit", func() bool { _, e, _ := r.d.counts(); return e == 1 })
		r.d.mu.Lock()
		defer r.d.mu.Unlock()
		if r.d.edits[0].ID != "old1" {
			t.Errorf("edited %s, want old1", r.d.edits[0].ID)
		}
		if got := r.d.msgs[0].Embeds[0].Description; got != "Stream ended · 1h 30m" {
			t.Errorf("description = %q", got)
		}
	})
	t.Run("started posts nothing", func(t *testing.T) {
		r := newRig(t, "v1", "", time.Minute, open)
		r.handled(t, webhook.Started)
		settle()
		if s, _, _ := r.d.counts(); s != 0 || r.yt.count() != 0 {
			t.Errorf("posts = %d, lookups = %d, want none", s, r.yt.count())
		}
	})
	t.Run("another author's live embed is not ours", func(t *testing.T) {
		foreign := botMessage("x1", start, embeds.CreateLiveEmbed(video("v9")))
		foreign.Author = &discordgo.User{ID: "123"}
		r := newRig(t, "v1", "", time.Minute, foreign)
		r.a.OnEvent(webhook.Started)
		waitFor(t, "the live post", func() bool { s, _, _ := r.d.counts(); return s == 1 })
	})
}

func TestFoundVideoAlreadyEndedPostsNothing(t *testing.T) {
	ended := botMessage("old1", start.Add(-time.Hour), embeds.CreateEndedEmbed(video("v1"), time.Hour))
	r := newRig(t, "v1", "", time.Minute, ended)
	r.a.OnEvent(webhook.Started)
	waitFor(t, "a lookup", func() bool { return r.yt.count() >= 1 })
	settle()
	if s, _, _ := r.d.counts(); s != 0 {
		t.Errorf("posts = %d, want 0", s)
	}
	if got := r.yt.count(); got != 1 {
		t.Errorf("lookups = %d, want 1: the search should end", got)
	}

	t.Run("a different video after an ended one posts", func(t *testing.T) {
		r := newRig(t, "v2", "", time.Minute, ended)
		r.a.OnEvent(webhook.Started)
		waitFor(t, "the live post", func() bool { s, _, _ := r.d.counts(); return s == 1 })
	})
}

func TestNeverFoundStopsAfterTheWindow(t *testing.T) {
	window := 20 * interval
	r := newRig(t, "", "", window)
	r.a.OnEvent(webhook.Started)
	time.Sleep(window + 10*interval)
	lookups := r.yt.count()
	settle()
	if got := r.yt.count(); got != lookups {
		t.Errorf("lookups kept going after the window: %d then %d", lookups, got)
	}
	if lookups < 2 || lookups > 22 {
		t.Errorf("lookups = %d, want about 21", lookups)
	}
	if s, _, _ := r.d.counts(); s != 0 {
		t.Errorf("posts = %d, want 0", s)
	}
}

func TestPingRoleOnlyOnTheLivePost(t *testing.T) {
	r := newRig(t, "v1", "777", time.Minute)
	r.a.OnEvent(webhook.Started)
	waitFor(t, "the live post", func() bool { s, _, _ := r.d.counts(); return s == 1 })
	r.a.OnEvent(webhook.Stopped)
	waitFor(t, "the edit", func() bool { _, e, _ := r.d.counts(); return e == 1 })

	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	send := r.d.sends[0]
	if send.Content != "<@&777>" {
		t.Errorf("live content = %q", send.Content)
	}
	if am := send.AllowedMentions; am == nil || len(am.Parse) != 0 || len(am.Users) != 0 || len(am.Roles) != 1 || am.Roles[0] != "777" {
		t.Errorf("live allowed mentions = %+v, want only role 777", am)
	}
	edit := r.d.edits[0]
	if edit.Content == nil || *edit.Content != "" {
		t.Errorf("edit does not clear the content: %v", edit.Content)
	}
	if am := edit.AllowedMentions; am == nil || len(am.Roles)+len(am.Users)+len(am.Parse) != 0 {
		t.Errorf("edit allowed mentions = %+v, want none", am)
	}
	if r.d.msgs[0].Content != "" {
		t.Errorf("message content after edit = %q", r.d.msgs[0].Content)
	}
}

func TestNoPingRoleMentionsNobody(t *testing.T) {
	r := newRig(t, "v1", "", time.Minute)
	r.a.OnEvent(webhook.Started)
	waitFor(t, "the live post", func() bool { s, _, _ := r.d.counts(); return s == 1 })
	r.d.mu.Lock()
	defer r.d.mu.Unlock()
	if send := r.d.sends[0]; send.Content != "" || send.AllowedMentions == nil || len(send.AllowedMentions.Roles) != 0 {
		t.Errorf("content %q, allowed mentions %+v, want no ping", send.Content, send.AllowedMentions)
	}
}

func TestStartedWhileSearchingKeepsTheWindow(t *testing.T) {
	r := newRig(t, "", "", 40*interval)
	r.a.OnEvent(webhook.Started)
	time.Sleep(20 * interval)
	r.a.OnEvent(webhook.Started)
	time.Sleep(30 * interval)
	lookups := r.yt.count()
	settle()
	if got := r.yt.count(); got != lookups {
		t.Errorf("a second started extended the search: %d lookups then %d", lookups, got)
	}
}
