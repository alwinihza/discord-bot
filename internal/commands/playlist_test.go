package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/bwmarrin/discordgo"
)

var escapedPair = regexp.MustCompile(`\\[[:punct:]]`)

const hostileName = "@everyone [free nitro](https://evil.example/x) <@&900000000000000099> `x` *b* ||s|| # h"

type discordCall struct {
	Method string
	Path   string
	Body   map[string]any
	Status int
}

// fakeDiscord refuses content over Discord's 2000 character limit, as Discord does, and can refuse every followup.
type fakeDiscord struct {
	mu           sync.Mutex
	calls        []discordCall
	failFollowup bool
}

func (f *fakeDiscord) RoundTrip(r *http.Request) (*http.Response, error) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	json.Unmarshal(raw, &body)
	call := discordCall{Method: r.Method, Path: r.URL.Path, Body: body, Status: 200}
	if utf8.RuneCountInString(contentOf(call)) > 2000 || f.failFollowup && r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/webhooks/2/tok") {
		call.Status = 400
	}
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
	return &http.Response{StatusCode: call.Status, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}, Request: r}, nil
}

func (f *fakeDiscord) snapshot() []discordCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]discordCall(nil), f.calls...)
}

func newSession(t *testing.T) (*discordgo.Session, *fakeDiscord) {
	t.Helper()
	s, err := discordgo.New("Bot x")
	if err != nil {
		t.Fatal(err)
	}
	fd := &fakeDiscord{}
	s.Client = &http.Client{Transport: fd}
	return s, fd
}

func newSite(t *testing.T, h http.HandlerFunc) *site.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return site.NewClient(srv.URL, "secret", "https://fio.example")
}

func summaryHandler(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(site.Summary{ID: "aB3dE5fG", Name: name, EntryCount: 3, Deleted: r.Method == http.MethodDelete})
	}
}

func slash(perms int64, sub, link string) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID: "1", AppID: "2", Token: "tok", Type: discordgo.InteractionApplicationCommand,
		Member: &discordgo.Member{Permissions: perms},
		Data: discordgo.ApplicationCommandInteractionData{Name: "playlist-mod", Options: []*discordgo.ApplicationCommandInteractionDataOption{{
			Name: sub, Type: discordgo.ApplicationCommandOptionSubCommand,
			Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: "link", Type: discordgo.ApplicationCommandOptionString, Value: link}},
		}}},
	}}
}

func button(perms int64, customID string) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID: "1", AppID: "2", Token: "tok", Type: discordgo.InteractionMessageComponent,
		Member: &discordgo.Member{Permissions: perms},
		Data:   discordgo.MessageComponentInteractionData{CustomID: customID},
	}}
}

func handler(sc *site.Client) *CommandHandler { return &CommandHandler{site: sc} }

func contentOf(c discordCall) string {
	if d, ok := c.Body["data"].(map[string]any); ok {
		c.Body = d
	}
	s, _ := c.Body["content"].(string)
	return s
}

func assertNoMentions(t *testing.T, c discordCall) {
	t.Helper()
	body := c.Body
	if d, ok := body["data"].(map[string]any); ok {
		body = d
	}
	am, ok := body["allowed_mentions"].(map[string]any)
	if !ok {
		t.Errorf("%s %s has no allowed_mentions", c.Method, c.Path)
		return
	}
	if p, _ := am["parse"].([]any); len(p) != 0 || am["roles"] != nil || am["users"] != nil {
		t.Errorf("%s %s allows mentions: %v", c.Method, c.Path, am)
	}
}

func TestPlaylistModNeutralisesFanControlledName(t *testing.T) {
	sc := newSite(t, summaryHandler(hostileName))
	for _, sub := range []string{"info", "delete"} {
		s, fd := newSession(t)
		handler(sc).PlaylistMod(s, slash(discordgo.PermissionManageMessages, sub, "aB3dE5fG"))
		calls := fd.snapshot()
		if len(calls) == 0 {
			t.Fatalf("%s: no Discord calls", sub)
		}
		final := calls[len(calls)-1]
		literal := escapedPair.ReplaceAllString(contentOf(final), "")
		literal = strings.ReplaceAll(literal, "**", "")
		if strings.ContainsAny(literal, "[]()<>@`|*#") {
			t.Errorf("%s: unescaped markup in %q", sub, contentOf(final))
		}
		for _, c := range calls {
			assertNoMentions(t, c)
		}
	}
}

func TestEscapeMarkdownLeavesNoUnescapedPunctuation(t *testing.T) {
	out := escapeMarkdown(hostileName + "\nnext line")
	if strings.Contains(out, "\n") {
		t.Errorf("newline survived: %q", out)
	}
	rs := []rune(out)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\\' {
			i++
			continue
		}
		if strings.ContainsRune("*_~`|>[]()#<@:&", rs[i]) {
			t.Fatalf("unescaped %q at %d in %q", rs[i], i, out)
		}
	}
}

func TestConfirmAndCancelRepliesAllowNoMentions(t *testing.T) {
	sc := newSite(t, summaryHandler(hostileName))
	s, fd := newSession(t)
	handler(sc).handlePlaylistButton(s, button(discordgo.PermissionManageMessages, playlistConfirmPrefix+"aB3dE5fG"), playlistConfirmPrefix+"aB3dE5fG")
	handler(sc).handlePlaylistButton(s, button(0, playlistCancelID), playlistCancelID)
	handler(sc).handlePlaylistButton(s, button(0, playlistConfirmPrefix+"aB3dE5fG"), playlistConfirmPrefix+"aB3dE5fG")
	calls := fd.snapshot()
	if len(calls) < 4 {
		t.Fatalf("calls = %d", len(calls))
	}
	for _, c := range calls {
		assertNoMentions(t, c)
	}
	if got := contentOf(calls[1]); strings.Contains(got, "[free nitro](") || strings.Contains(got, "<@&") {
		t.Errorf("confirm edit has live markup: %s", got)
	}
}

func TestPlaylistModRefusesWithoutManageMessagesAtRuntime(t *testing.T) {
	var hits int
	sc := newSite(t, func(w http.ResponseWriter, r *http.Request) { hits++; summaryHandler("x")(w, r) })
	for _, sub := range []string{"info", "delete"} {
		s, fd := newSession(t)
		handler(sc).PlaylistMod(s, slash(0, sub, "aB3dE5fG"))
		calls := fd.snapshot()
		if len(calls) != 1 || !strings.Contains(contentOf(calls[0]), "mods") {
			t.Errorf("%s: calls = %+v", sub, calls)
		}
		if flags, _ := calls[0].Body["data"].(map[string]any)["flags"].(float64); int(flags)&int(discordgo.MessageFlagsEphemeral) == 0 {
			t.Errorf("%s: refusal is not ephemeral", sub)
		}
	}
	if hits != 0 {
		t.Errorf("site was called %d times for a non-mod", hits)
	}
}

func TestConfirmRejectsMalformedIDWithoutReachingSite(t *testing.T) {
	var paths []string
	sc := newSite(t, func(w http.ResponseWriter, r *http.Request) { paths = append(paths, r.URL.RequestURI()) })
	for _, id := range []string{"../../x", "a?all=1", "short"} {
		s, fd := newSession(t)
		h := playlistConfirmPrefix + url.PathEscape(id)
		handler(sc).handlePlaylistButton(s, button(discordgo.PermissionManageMessages, h), h)
		calls := fd.snapshot()
		if got := contentOf(calls[len(calls)-1]); !strings.Contains(got, "❌") {
			t.Errorf("%q: final reply = %q, want an error", id, got)
		}
	}
	if len(paths) != 0 {
		t.Errorf("site saw %v", paths)
	}
}

func TestPlaylistModAcknowledgesBeforeSlowSiteAnswers(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	started := make(chan struct{}, 1)
	sc := newSite(t, func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-release
		summaryHandler("Slow")(w, r)
	})
	s, fd := newSession(t)
	done := make(chan struct{})
	go func() {
		handler(sc).PlaylistMod(s, slash(discordgo.PermissionManageMessages, "info", "aB3dE5fG"))
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("site never called")
	}
	calls := fd.snapshot()
	if len(calls) != 1 || calls[0].Method != http.MethodPost || calls[0].Body["type"] != float64(discordgo.InteractionResponseDeferredChannelMessageWithSource) {
		t.Fatalf("before the site answered, calls = %+v; want one deferred ack", calls)
	}
	if flags, _ := calls[0].Body["data"].(map[string]any)["flags"].(float64); int(flags)&int(discordgo.MessageFlagsEphemeral) == 0 {
		t.Error("deferred ack is not ephemeral")
	}
	unblock()
	<-done
	calls = fd.snapshot()
	if len(calls) != 2 || calls[1].Method != http.MethodPatch || !strings.Contains(contentOf(calls[1]), "Slow") {
		t.Errorf("after release, calls = %+v; want an edit of the original with the summary", calls)
	}
}
