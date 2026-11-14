package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alwinihza/discord-bot/database"
	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/bwmarrin/discordgo"
)

const (
	userA = "900000000000000011"
	userB = "900000000000000022"
)

type siteCall struct{ Method, Path, User, Body string }

type ownerSite struct {
	mu    sync.Mutex
	calls []siteCall
	reply func(line string) (int, string)
}

func (o *ownerSite) client(t *testing.T) *site.Client {
	t.Helper()
	return newSite(t, func(w http.ResponseWriter, r *http.Request) {
		buf := new(strings.Builder)
		b := make([]byte, 4096)
		for {
			n, err := r.Body.Read(b)
			buf.Write(b[:n])
			if err != nil {
				break
			}
		}
		o.mu.Lock()
		o.calls = append(o.calls, siteCall{r.Method, r.URL.Path, r.Header.Get("X-Acting-Discord-Id"), buf.String()})
		o.mu.Unlock()
		status, body := o.reply(r.Method + " " + r.URL.Path)
		w.WriteHeader(status)
		w.Write([]byte(body))
	})
}

func (o *ownerSite) seen() []siteCall {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]siteCall(nil), o.calls...)
}

func playlistJSON(name string, titles ...string) string {
	entries := make([]string, len(titles))
	for n, title := range titles {
		entries[n] = fmt.Sprintf(`{"songId":"song-%d","title":%q}`, n+1, title)
	}
	return fmt.Sprintf(`{"id":"Rainy123","name":%q,"entries":[%s],"version":1}`, name, strings.Join(entries, ","))
}

func okSite(name string, titles ...string) *ownerSite {
	return &ownerSite{reply: func(line string) (int, string) {
		if line == "GET /admin/owner/playlists" {
			return 200, fmt.Sprintf(`{"playlists":[{"id":"Rainy123","name":%q,"entryCount":%d},{"id":"Other456","name":"Sunny","entryCount":1}]}`, name, len(titles))
		}
		if strings.HasPrefix(line, "POST /admin/owner/playlists") && strings.Count(line, "/") == 3 {
			return 201, playlistJSON(name, titles...)
		}
		return 200, playlistJSON(name, titles...)
	}}
}

func fan(user, sub string, opts ...string) *discordgo.InteractionCreate {
	data := discordgo.ApplicationCommandInteractionData{Name: "playlist", Options: []*discordgo.ApplicationCommandInteractionDataOption{{
		Name: sub, Type: discordgo.ApplicationCommandOptionSubCommand,
	}}}
	for n := 0; n+1 < len(opts); n += 2 {
		data.Options[0].Options = append(data.Options[0].Options, &discordgo.ApplicationCommandInteractionDataOption{
			Name: opts[n], Type: discordgo.ApplicationCommandOptionString, Value: opts[n+1],
		})
	}
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID: "1", AppID: "2", Token: "tok", Type: discordgo.InteractionApplicationCommand,
		Member: &discordgo.Member{User: &discordgo.User{ID: user}}, Data: data,
	}}
}

func focus(i *discordgo.InteractionCreate, option string) *discordgo.InteractionCreate {
	i.Type = discordgo.InteractionApplicationCommandAutocomplete
	for _, o := range i.ApplicationCommandData().Options[0].Options {
		o.Focused = o.Name == option
	}
	return i
}

func clickBy(user, customID string) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID: "1", AppID: "2", Token: "tok", Type: discordgo.InteractionMessageComponent,
		Member: &discordgo.Member{User: &discordgo.User{ID: user}},
		Data:   discordgo.MessageComponentInteractionData{CustomID: customID},
	}}
}

func lastContent(t *testing.T, fd *fakeDiscord) string {
	t.Helper()
	calls := fd.snapshot()
	if len(calls) == 0 {
		t.Fatal("no Discord calls")
	}
	return contentOf(calls[len(calls)-1])
}

func flagsOf(c discordCall) int {
	body := c.Body
	if d, ok := body["data"].(map[string]any); ok {
		body = d
	}
	f, _ := body["flags"].(float64)
	return int(f)
}

func seededDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "karaoke.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func fanHandler(sc *site.Client) *CommandHandler { return &CommandHandler{site: sc} }

func TestEveryPlaylistSubcommandActsAsTheInteractionUser(t *testing.T) {
	cases := []struct {
		sub  string
		opts []string
	}{
		{"mine", nil},
		{"add", []string{"playlist", "Rainy123", "song", "x"}},
		{"remove", []string{"playlist", "Rainy123", "song", "song-1"}},
		{"rename", []string{"playlist", "Rainy123", "name", "New"}},
		{"delete", []string{"playlist", "Rainy123"}},
		{"share", []string{"playlist", "Rainy123"}},
	}
	for _, tc := range cases {
		o := okSite("Rainy", "Halu", "Runtuh")
		s, _ := newSession(t)
		h := fanHandler(o.client(t))
		h.db = seededDB(t)
		in := fan(userA, tc.sub, append(tc.opts, "extra", userB)...)
		if tc.sub == "mine" {
			in = fan(userA, tc.sub)
		}
		h.Playlist(s, in)
		calls := o.seen()
		if len(calls) == 0 {
			t.Fatalf("%s: site was not called", tc.sub)
		}
		for _, c := range calls {
			if c.User != userA {
				t.Errorf("%s: %s %s acted as %q, want %q", tc.sub, c.Method, c.Path, c.User, userA)
			}
		}
	}
}

func TestCreateResolvesTypedSongTextAndSendsItsID(t *testing.T) {
	db := seededDB(t)
	want, err := db.ResolveSong(context.Background(), "celengan rindu")
	if err != nil {
		t.Fatal(err)
	}
	o := okSite("Rainy", "Celengan Rindu")
	s, fd := newSession(t)
	h := fanHandler(o.client(t))
	h.db = db
	h.Playlist(s, fan(userA, "create", "name", "Rainy", "song", "celengan rindu"))
	calls := o.seen()
	if len(calls) != 1 || calls[0].Method != "POST" {
		t.Fatalf("site calls = %+v", calls)
	}
	var body struct {
		Name    string
		SongIDs []string
	}
	json.Unmarshal([]byte(calls[0].Body), &body)
	if body.Name != "Rainy" || len(body.SongIDs) != 1 || body.SongIDs[0] != want.ID {
		t.Errorf("body = %s, want song %s", calls[0].Body, want.ID)
	}
	if got := lastContent(t, fd); !strings.Contains(got, "https://fio.example/p/Rainy123") || !strings.Contains(got, "Created") {
		t.Errorf("reply = %q", got)
	}
}

func TestCreateWithUnknownSongNeverReachesTheSite(t *testing.T) {
	o := okSite("Rainy", "x")
	s, fd := newSession(t)
	h := fanHandler(o.client(t))
	h.db = seededDB(t)
	h.Playlist(s, fan(userA, "create", "name", "Rainy", "song", "zzzznosuchsong"))
	if len(o.seen()) != 0 {
		t.Errorf("site calls = %+v", o.seen())
	}
	if got := lastContent(t, fd); !strings.Contains(got, "❌") || !strings.Contains(got, "zzzznosuchsong") {
		t.Errorf("reply = %q", got)
	}
}

func TestSiteStatusesBecomeEphemeralRepliesTheCallerCanActOn(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{403, `{"error":"This playlist belongs to someone else"}`, "belongs to someone else"},
		{403, `{"error":"Forbidden"}`, "SITE_ADMIN_SECRET"},
		{404, `{"error":"No such playlist"}`, "No playlist"},
		{404, `{"error":"Not Found"}`, "does not support playlists from Discord yet"},
		{409, `{"error":"The playlist changed. Reload it."}`, "try again"},
		{410, `{"error":"This playlist was deleted"}`, "was deleted"},
		{429, `{"error":"Too many"}`, "Slow down"},
		{400, `{"error":"Each song may appear once"}`, "Each song may appear once"},
		{503, `{"error":"cap"}`, "tomorrow"},
		{500, `{"error":"boom"}`, "Could not reach the site"},
	}
	for _, tc := range cases {
		o := &ownerSite{reply: func(string) (int, string) { return tc.status, tc.body }}
		s, fd := newSession(t)
		h := fanHandler(o.client(t))
		h.db = seededDB(t)
		h.Playlist(s, fan(userA, "rename", "playlist", "Rainy123", "name", "New"))
		calls := fd.snapshot()
		if flagsOf(calls[0])&int(discordgo.MessageFlagsEphemeral) == 0 {
			t.Errorf("%d: acknowledgement is not ephemeral", tc.status)
		}
		if got := contentOf(calls[len(calls)-1]); !strings.HasPrefix(got, "❌") || !strings.Contains(got, tc.want) {
			t.Errorf("%d %s: reply = %q, want ❌ and %q", tc.status, tc.body, got, tc.want)
		}
	}
}

func TestFanControlledNamesStayLiteralAndPingNobody(t *testing.T) {
	for _, sub := range []string{"mine", "rename", "share", "delete", "add", "remove"} {
		o := okSite(hostileName, hostileName)
		s, fd := newSession(t)
		h := fanHandler(o.client(t))
		h.db = seededDB(t)
		opts := []string{"playlist", "Rainy123", "name", "New", "song", "song-1"}
		if sub == "mine" {
			opts = nil
		}
		h.Playlist(s, fan(userA, sub, opts...))
		for _, c := range fd.snapshot() {
			assertNoMentions(t, c)
			if got := contentOf(c); strings.Contains(got, "[free nitro](") || strings.Contains(got, "<@&") || strings.Contains(got, "@everyone") && !strings.Contains(got, `\@everyone`) {
				t.Errorf("%s: live markup in %q", sub, got)
			}
		}
	}
}

func TestShareIsTheOnlyPublicReplyAndHoldsNoEditToken(t *testing.T) {
	o := okSite("Rainy", "Halu")
	s, fd := newSession(t)
	h := fanHandler(o.client(t))
	h.Playlist(s, fan(userA, "share", "playlist", "https://fio.example/p/Rainy123#edit=SECRETTOKEN"))
	calls := fd.snapshot()
	if len(calls) != 4 {
		t.Fatalf("calls = %+v", calls)
	}
	public := calls[2]
	if public.Method != http.MethodPost || !strings.HasSuffix(public.Path, "/webhooks/2/tok") {
		t.Fatalf("third call = %s %s", public.Method, public.Path)
	}
	if flagsOf(public) != 0 {
		t.Errorf("shared message flags = %d, want public", flagsOf(public))
	}
	text := contentOf(public)
	if !strings.Contains(text, "https://fio.example/p/Rainy123") || strings.Contains(text, "SECRETTOKEN") || strings.Contains(text, "edit") {
		t.Errorf("public message = %q", text)
	}
	for _, c := range []discordCall{calls[0], calls[1], calls[3]} {
		if strings.Contains(contentOf(c), "https://") {
			t.Errorf("private reply carries the link: %q", contentOf(c))
		}
	}
}

func TestDeleteAsksFirstAndConfirmDeletesAsTheClicker(t *testing.T) {
	o := okSite("Rainy", "Halu")
	s, fd := newSession(t)
	h := fanHandler(o.client(t))
	h.Playlist(s, fan(userA, "delete", "playlist", "Rainy123"))
	for _, c := range o.seen() {
		if c.Method == http.MethodDelete {
			t.Fatalf("deleted before confirmation: %+v", c)
		}
	}
	final := fd.snapshot()[1]
	if !strings.Contains(fmt.Sprint(final.Body["components"]), fanConfirmPrefix+"Rainy123") {
		t.Errorf("no confirm button in %+v", final.Body)
	}

	s, fd = newSession(t)
	h.handleFanPlaylistButton(s, clickBy(userB, fanConfirmPrefix+"Rainy123"), fanConfirmPrefix+"Rainy123")
	calls := o.seen()
	last := calls[len(calls)-1]
	if last.Method != http.MethodDelete || last.Path != "/admin/owner/playlists/Rainy123" || last.User != userB {
		t.Errorf("confirm call = %+v, want DELETE as the clicker", last)
	}
	if got := lastContent(t, fd); !strings.Contains(got, "Deleted") {
		t.Errorf("reply = %q", got)
	}
}

func TestCancelAndBadConfirmDoNotTouchTheSite(t *testing.T) {
	o := okSite("Rainy", "Halu")
	h := fanHandler(o.client(t))
	s, fd := newSession(t)
	h.HandleInteraction(s, clickBy(userA, fanCancelID))
	if got := lastContent(t, fd); !strings.Contains(got, "Cancelled") {
		t.Errorf("cancel reply = %q", got)
	}
	for _, id := range []string{"../../x", "a?all=1", "short"} {
		s, fd = newSession(t)
		h.HandleInteraction(s, clickBy(userA, fanConfirmPrefix+id))
		if got := lastContent(t, fd); !strings.Contains(got, "❌") {
			t.Errorf("%q: reply = %q", id, got)
		}
	}
	if len(o.seen()) != 0 {
		t.Errorf("site calls = %+v", o.seen())
	}
}

func choiceNames(fd *fakeDiscord) (names, values []string) {
	calls := fd.snapshot()
	d, _ := calls[len(calls)-1].Body["data"].(map[string]any)
	list, _ := d["choices"].([]any)
	for _, c := range list {
		m := c.(map[string]any)
		names = append(names, m["name"].(string))
		values = append(values, m["value"].(string))
	}
	return
}

func TestPlaylistAutocompleteListsOnlyTheCallersPlaylists(t *testing.T) {
	o := okSite("Rainy", "Halu")
	h := fanHandler(o.client(t))
	s, fd := newSession(t)
	h.Autocomplete(s, focus(fan(userB, "rename", "playlist", "rai", "name", ""), "playlist"))
	names, values := choiceNames(fd)
	if len(values) != 1 || values[0] != "Rainy123" || !strings.HasPrefix(names[0], "Rainy") {
		t.Errorf("choices = %v / %v, want only the playlist matching rai", names, values)
	}
	calls := o.seen()
	if len(calls) != 1 || calls[0].User != userB {
		t.Errorf("site calls = %+v, want one list as user B", calls)
	}
}

func TestRemoveSongAutocompleteOffersOnlySongsInThePlaylist(t *testing.T) {
	o := okSite("Rainy", "Halu", "Runtuh")
	h := fanHandler(o.client(t))
	h.db = seededDB(t)
	s, fd := newSession(t)
	h.Autocomplete(s, focus(fan(userA, "remove", "playlist", "Rainy123", "song", "run"), "song"))
	names, values := choiceNames(fd)
	if len(values) != 1 || values[0] != "song-2" || names[0] != "Runtuh" {
		t.Errorf("choices = %v / %v", names, values)
	}
	calls := o.seen()
	if len(calls) != 1 || calls[0].Path != "/admin/owner/playlists/Rainy123" || calls[0].User != userA {
		t.Errorf("site calls = %+v", calls)
	}
}

func TestAddSongAutocompleteUsesTheSharedSongSearch(t *testing.T) {
	o := okSite("Rainy", "Halu")
	h := fanHandler(o.client(t))
	h.db = seededDB(t)
	s, fd := newSession(t)
	h.Autocomplete(s, focus(fan(userA, "add", "playlist", "Rainy123", "song", "celengan"), "song"))
	names, values := choiceNames(fd)
	if len(values) == 0 || !strings.Contains(strings.ToLower(names[0]), "celengan") {
		t.Errorf("choices = %v", names)
	}
	if len(o.seen()) != 0 {
		t.Errorf("song search called the site: %+v", o.seen())
	}
}

func TestPlaylistAutocompleteAnswersEmptyWhenTheSiteFails(t *testing.T) {
	o := &ownerSite{reply: func(string) (int, string) { return 404, `{"error":"Not Found"}` }}
	h := fanHandler(o.client(t))
	s, fd := newSession(t)
	h.Autocomplete(s, focus(fan(userA, "delete", "playlist", ""), "playlist"))
	if _, values := choiceNames(fd); len(values) != 0 {
		t.Errorf("choices = %v", values)
	}
}

func TestPlaylistCommandsWithoutSiteConfigAnswerPrivatelyAndDoNotCrash(t *testing.T) {
	h := &CommandHandler{}
	for _, sub := range []string{"mine", "create", "delete"} {
		s, fd := newSession(t)
		h.Playlist(s, fan(userA, sub, "name", "x", "song", "y", "playlist", "Rainy123"))
		calls := fd.snapshot()
		if len(calls) != 1 || !strings.Contains(contentOf(calls[0]), "not set up") || flagsOf(calls[0])&int(discordgo.MessageFlagsEphemeral) == 0 {
			t.Errorf("%s: calls = %+v", sub, calls)
		}
	}
	s, fd := newSession(t)
	h.Autocomplete(s, focus(fan(userA, "add", "playlist", ""), "playlist"))
	if _, values := choiceNames(fd); len(values) != 0 {
		t.Errorf("choices = %v", values)
	}
	s, fd = newSession(t)
	h.HandleInteraction(s, clickBy(userA, fanConfirmPrefix+"Rainy123"))
	if got := lastContent(t, fd); !strings.Contains(got, "❌") {
		t.Errorf("confirm reply = %q", got)
	}
}

func TestPlaylistIsOpenToEveryMemberInGuildsOnly(t *testing.T) {
	subs := map[string]bool{}
	for _, def := range Definitions {
		if def.Name != "playlist" {
			continue
		}
		if def.DefaultMemberPermissions != nil {
			t.Errorf("DefaultMemberPermissions = %v, want open to everyone", *def.DefaultMemberPermissions)
		}
		if def.Contexts == nil || len(*def.Contexts) != 1 || (*def.Contexts)[0] != discordgo.InteractionContextGuild {
			t.Errorf("Contexts = %v, want guild only", def.Contexts)
		}
		for _, sub := range def.Options {
			subs[sub.Name] = true
			for _, opt := range sub.Options {
				if opt.Name == "playlist" && !opt.Autocomplete {
					t.Errorf("%s: playlist option has no autocomplete", sub.Name)
				}
			}
		}
	}
	for _, want := range []string{"mine", "create", "add", "remove", "rename", "delete", "share"} {
		if !subs[want] {
			t.Errorf("missing subcommand %s", want)
		}
	}
	if len(subs) != 7 {
		t.Errorf("subcommands = %v", subs)
	}
}

func TestMineWithNothingSavedPointsToCreate(t *testing.T) {
	o := &ownerSite{reply: func(string) (int, string) { return 200, `{"playlists":[]}` }}
	s, fd := newSession(t)
	fanHandler(o.client(t)).Playlist(s, fan(userA, "mine"))
	if got := lastContent(t, fd); !strings.Contains(got, "no playlists") {
		t.Errorf("reply = %q", got)
	}
}

func TestMineStaysUnderDiscordsLimitForManyPlaylists(t *testing.T) {
	var items []string
	for n := range 60 {
		items = append(items, fmt.Sprintf(`{"id":"Rainy%03d","name":%q,"entryCount":2}`, n, strings.Repeat("@", 60)))
	}
	o := &ownerSite{reply: func(string) (int, string) { return 200, `{"playlists":[` + strings.Join(items, ",") + `]}` }}
	s, fd := newSession(t)
	fanHandler(o.client(t)).Playlist(s, fan(userA, "mine"))
	got := lastContent(t, fd)
	shown := strings.Count(got, "/p/Rainy")
	if len([]rune(got)) > 2000 || shown == 0 || !strings.Contains(got, fmt.Sprintf("%d more", 60-shown)) {
		t.Errorf("reply shows %d of 60 and has %d chars: %.80q", shown, len([]rune(got)), got)
	}
}

var stackedName = strings.Repeat("a"+strings.Repeat("\u0301", 40), 60)

func anyRejected(fd *fakeDiscord) bool {
	for _, c := range fd.snapshot() {
		if c.Status != 200 {
			return true
		}
	}
	return false
}

func TestSixtyGraphemeNameWithStackedMarksFitsDiscordInEveryReply(t *testing.T) {
	for _, sub := range []string{"mine", "create", "add", "remove", "rename", "delete", "share"} {
		o := okSite(stackedName, "Halu")
		s, fd := newSession(t)
		h := fanHandler(o.client(t))
		h.db = seededDB(t)
		h.Playlist(s, fan(userA, sub, "playlist", "Rainy123", "name", "New", "song", "song-1"))
		if sub == "delete" {
			h.handleFanPlaylistButton(s, clickBy(userA, fanConfirmPrefix+"Rainy123"), fanConfirmPrefix+"Rainy123")
		}
		if anyRejected(fd) {
			t.Errorf("%s: Discord rejected a message: %+v", sub, fd.snapshot())
		}
	}
}

func TestMineSummarisesLongNamesInsteadOfCuttingThem(t *testing.T) {
	var items []string
	for n := range 9 {
		items = append(items, fmt.Sprintf(`{"id":"Rainy%03d","name":%q,"entryCount":2}`, n, stackedName))
	}
	o := &ownerSite{reply: func(string) (int, string) { return 200, `{"playlists":[` + strings.Join(items, ",") + `]}` }}
	s, fd := newSession(t)
	fanHandler(o.client(t)).Playlist(s, fan(userA, "mine"))
	got := lastContent(t, fd)
	shown := strings.Count(got, "/p/Rainy")
	if anyRejected(fd) || shown == 0 || (shown < 9 && !strings.Contains(got, fmt.Sprintf("%d more", 9-shown))) {
		t.Errorf("shows %d of 9, %d chars, tail %q", shown, len([]rune(got)), got[max(0, len(got)-40):])
	}
}

func TestShareOnlyClaimsSharedWhenTheChannelPostWorked(t *testing.T) {
	o := okSite("Rainy", "Halu")
	s, fd := newSession(t)
	fd.failFollowup = true
	fanHandler(o.client(t)).Playlist(s, fan(userA, "share", "playlist", "Rainy123"))
	calls := fd.snapshot()
	last := calls[len(calls)-1]
	if got := contentOf(last); strings.Contains(got, "Shared in this channel") || !strings.HasPrefix(got, "❌") {
		t.Errorf("final reply after a failed post = %q", got)
	}
	if last.Method != http.MethodPatch {
		t.Errorf("final call = %s, want the private reply edited", last.Method)
	}
}

func TestRemoveByArtistTextFindsTheEntryTheSiteTitlesWithoutAnArtist(t *testing.T) {
	db := seededDB(t)
	song, err := db.ResolveSong(context.Background(), "celengan rindu")
	if err != nil || song.Artist == "" {
		t.Fatalf("seed song: %+v, %v", song, err)
	}
	other, err := db.ResolveSong(context.Background(), "kokoronashi")
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"id":"Rainy123","name":"Rainy","entries":[{"songId":%q,"title":%q},{"songId":%q,"title":%q}],"version":1}`,
		song.ID, song.Title, other.ID, other.Title)
	o := &ownerSite{reply: func(string) (int, string) { return 200, body }}
	h := fanHandler(o.client(t))
	h.db = db

	s, fd := newSession(t)
	h.Playlist(s, fan(userA, "remove", "playlist", "Rainy123", "song", strings.ToLower(song.Artist)))
	var deleted []string
	for _, c := range o.seen() {
		if c.Method == http.MethodDelete {
			deleted = append(deleted, c.Path)
		}
	}
	if want := "/admin/owner/playlists/Rainy123/songs/" + song.ID; len(deleted) != 1 || deleted[0] != want {
		t.Errorf("deletes = %v, want [%s]; reply %q", deleted, want, lastContent(t, fd))
	}

	s, fd = newSession(t)
	h.Autocomplete(s, focus(fan(userA, "remove", "playlist", "Rainy123", "song", ""), "song"))
	names, _ := choiceNames(fd)
	if len(names) != 2 || names[0] != song.Label() {
		t.Errorf("choices = %v, want %q first", names, song.Label())
	}
}

func TestRemoveRacingAnotherRemovalSaysTheSongIsGoneNotThePlaylist(t *testing.T) {
	db := seededDB(t)
	song, err := db.ResolveSong(context.Background(), "celengan rindu")
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"id":"Rainy123","name":"Rainy","entries":[{"songId":%q,"title":%q}],"version":1}`, song.ID, song.Title)
	o := &ownerSite{reply: func(line string) (int, string) {
		if strings.HasPrefix(line, "DELETE") {
			return 404, `{"error":"That song is not in the playlist"}`
		}
		return 200, body
	}}
	h := fanHandler(o.client(t))
	h.db = db
	s, fd := newSession(t)
	h.Playlist(s, fan(userA, "remove", "playlist", "Rainy123", "song", song.ID))
	got := lastContent(t, fd)
	if !strings.HasPrefix(got, "❌") || strings.Contains(got, "No playlist") || !strings.Contains(got, "song") {
		t.Errorf("reply = %q", got)
	}
}

func TestUnknownSongReplyShowsBackslashesOnce(t *testing.T) {
	s, fd := newSession(t)
	h := fanHandler(okSite("Rainy", "Halu").client(t))
	h.db = seededDB(t)
	h.Playlist(s, fan(userA, "add", "playlist", "Rainy123", "song", `zzz\qqq`))
	if got := lastContent(t, fd); !strings.Contains(got, `No song matches "zzz\\qqq"`) {
		t.Errorf("reply = %q", got)
	}
}

func TestBoundNameKeepsRealScriptsAndBoundsStackedMarks(t *testing.T) {
	flags := strings.Repeat("a", 78) + "🇮🇩🇯🇵"
	cases := []struct {
		name, in, want string
		maxRunes       int
	}{
		{"burmese", "ပြီး ကြွေး သီချင်း", "ပြီး ကြွေး သီချင်း", 0},
		{"vietnamese", "Việt Nam tiếng", "Việt Nam tiếng", 0},
		{"thai", "เพลงรักภาษาไทย ที่ดี", "เพลงรักภาษาไทย ที่ดี", 0},
		{"hindi", "मेरी पसंदीदा गाने कौन", "मेरी पसंदीदा गाने कौन", 0},
		{"keycap", "1️⃣ top 2️⃣", "1️⃣ top 2️⃣", 0},
		{"zalgo", "a" + strings.Repeat("́", 40) + "b", "á́b", 0},
		{"flag pair not split", flags, strings.Repeat("a", 78) + "…", 80},
	}
	for _, c := range cases {
		got := boundName(c.in)
		if got != c.want {
			t.Errorf("%s: boundName = %q, want %q", c.name, got, c.want)
		}
		if c.maxRunes > 0 && len([]rune(got)) > c.maxRunes {
			t.Errorf("%s: %d runes", c.name, len([]rune(got)))
		}
	}
}

func TestPlaylistAutocompleteBoundsStackedMarksWithinChoiceLimit(t *testing.T) {
	o := okSite(stackedName, "Halu")
	h := fanHandler(o.client(t))
	s, fd := newSession(t)
	h.Autocomplete(s, focus(fan(userA, "rename", "playlist", "a", "name", ""), "playlist"))
	names, _ := choiceNames(fd)
	if len(names) != 1 || len([]rune(names[0])) > 100 || strings.Contains(names[0], strings.Repeat("́", 3)) || !strings.HasSuffix(names[0], "(1 song)") {
		t.Errorf("choice = %q", names)
	}
}
