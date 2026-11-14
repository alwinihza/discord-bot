package commissions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/alwinihza/discord-bot/internal/site"
	"github.com/bwmarrin/discordgo"
)

const (
	channel = "900000000000000001"
	manager = "100000000000000001"
	other   = "100000000000000002"
)

type discordCall struct {
	Method string
	Path   string
	Body   map[string]any
}

// fakeDiscord answers Discord's REST API for the calls this feature makes.
type fakeDiscord struct {
	mu        sync.Mutex
	calls     []discordCall
	sendFails int
	nextID    int
}

func (f *fakeDiscord) RoundTrip(r *http.Request) (*http.Response, error) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	json.Unmarshal(raw, &body)

	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, discordCall{Method: r.Method, Path: r.URL.Path, Body: body})

	status, reply := 200, "{}"
	isSend := r.Method == http.MethodPost && r.URL.Path == "/api/v9/channels/"+channel+"/messages"
	switch {
	case isSend && f.sendFails > 0:
		f.sendFails--
		status = 500
	case isSend:
		f.nextID++
		reply = fmt.Sprintf(`{"id":"m%d","channel_id":%q}`, f.nextID, channel)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(reply)), Header: http.Header{}, Request: r}, nil
}

func (f *fakeDiscord) snapshot() []discordCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]discordCall(nil), f.calls...)
}

// count returns how many recorded calls match the method and a path suffix.
func (f *fakeDiscord) count(method, suffix string) int {
	n := 0
	for _, c := range f.snapshot() {
		if c.Method == method && strings.HasSuffix(c.Path, suffix) {
			n++
		}
	}
	return n
}

func (f *fakeDiscord) find(method, suffix string) []discordCall {
	var out []discordCall
	for _, c := range f.snapshot() {
		if c.Method == method && strings.HasSuffix(c.Path, suffix) {
			out = append(out, c)
		}
	}
	return out
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

type siteCall struct {
	Method, Path, Actor, Body string
}

// fakeSite serves the commission admin API. postedStatus and eventStatus pick each reply by call number.
type fakeSite struct {
	mu           sync.Mutex
	unposted     []site.Commission
	postedStatus func(n int, id string) int
	eventStatus  int
	eventReply   string
	calls        []siteCall
	postedCalls  int
}

func (f *fakeSite) client(t *testing.T) *site.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, siteCall{r.Method, r.URL.Path, r.Header.Get("X-Acting-Discord-Id"), string(raw)})
		switch {
		case strings.HasSuffix(r.URL.Path, "/unposted"):
			json.NewEncoder(w).Encode(f.unposted)
		case strings.HasSuffix(r.URL.Path, "/posted"):
			f.postedCalls++
			id := strings.Split(r.URL.Path, "/")[3]
			status := 200
			if f.postedStatus != nil {
				status = f.postedStatus(f.postedCalls, id)
			}
			w.WriteHeader(status)
			w.Write([]byte(`{"error":"x"}`))
		case strings.HasSuffix(r.URL.Path, "/events"):
			status := f.eventStatus
			if status == 0 {
				status = 200
			}
			w.WriteHeader(status)
			w.Write([]byte(f.eventReply))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return site.NewClient(srv.URL, "secret", "https://fio.example")
}

func (f *fakeSite) callsTo(suffix string) []siteCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []siteCall
	for _, c := range f.calls {
		if strings.HasSuffix(c.Path, suffix) {
			out = append(out, c)
		}
	}
	return out
}

func commission(id string) site.Commission {
	return site.Commission{
		ID: id, Kind: "voice", CreatedAt: "2026-03-01T10:00:00Z", Status: "received",
		Next: []string{"quoted", "declined"}, Fields: map[string]any{"name": "Rin"},
	}
}
