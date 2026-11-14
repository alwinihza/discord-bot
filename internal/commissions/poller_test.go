package commissions

import (
	"context"
	"testing"
	"time"

	"github.com/alwinihza/discord-bot/internal/site"
)

func newPoller(t *testing.T, fs *fakeSite) (*Poller, *fakeDiscord) {
	t.Helper()
	s, fd := newSession(t)
	return NewPoller(fs.client(t), s, channel), fd
}

func TestPollerPostsEachCommissionOnce(t *testing.T) {
	fs := &fakeSite{unposted: []site.Commission{commission("c1")}}
	p, fd := newPoller(t, fs)
	ctx := context.Background()

	p.Tick(ctx)
	p.Tick(ctx)

	if n := fd.count("POST", "/channels/"+channel+"/messages"); n != 1 {
		t.Errorf("messages sent = %d, want 1", n)
	}
	if n := fd.count("POST", "/messages/m1/threads"); n != 1 {
		t.Errorf("threads started = %d, want 1", n)
	}
	posted := fs.callsTo("/posted")
	if len(posted) != 1 || posted[0].Body != `{"channelId":"`+channel+`","messageId":"m1"}` {
		t.Errorf("posted calls = %+v", posted)
	}
}

func TestPollerRetriesPostedBeforeSendingAgain(t *testing.T) {
	fs := &fakeSite{unposted: []site.Commission{commission("c1")}}
	fs.postedStatusByCall = func(n int, _ string) int {
		if n == 1 {
			return 500
		}
		return 200
	}
	p, fd := newPoller(t, fs)
	ctx := context.Background()

	p.Tick(ctx)
	p.Tick(ctx)
	p.Tick(ctx)

	if n := fd.count("POST", "/channels/"+channel+"/messages"); n != 1 {
		t.Errorf("messages sent = %d, want 1 even though /posted failed once", n)
	}
	posted := fs.callsTo("/posted")
	if len(posted) != 2 {
		t.Fatalf("posted calls = %d, want 2 (fail, then success, then stop)", len(posted))
	}
	if posted[0].Body != posted[1].Body {
		t.Errorf("retry sent a different pair: %s vs %s", posted[0].Body, posted[1].Body)
	}
}

func TestPollerDeletesItsMessageOnConflict(t *testing.T) {
	fs := &fakeSite{unposted: []site.Commission{commission("c1")}}
	fs.postedStatusByCall = func(int, string) int { return 409 }
	p, fd := newPoller(t, fs)
	ctx := context.Background()

	p.Tick(ctx)
	p.Tick(ctx)

	if n := fd.count("DELETE", "/channels/"+channel+"/messages/m1"); n != 1 {
		t.Errorf("message deletes = %d, want 1", n)
	}
	if n := fd.count("DELETE", "/channels/m1"); n != 1 {
		t.Errorf("thread deletes = %d, want 1", n)
	}
	if n := fd.count("POST", "/channels/"+channel+"/messages"); n != 1 {
		t.Errorf("messages sent = %d, want 1: a 409 must not cause a resend", n)
	}
}

func TestPollerContinuesPastOneFailure(t *testing.T) {
	fs := &fakeSite{unposted: []site.Commission{commission("c1"), commission("c2")}}
	p, fd := newPoller(t, fs)
	fd.sendFails = 1

	p.Tick(context.Background())

	posted := fs.callsTo("/posted")
	if len(posted) != 1 || posted[0].Path != "/admin/commissions/c2/posted" {
		t.Fatalf("posted calls = %+v, want only c2", posted)
	}

	p.Tick(context.Background())
	if n := len(fs.callsTo("/posted")); n != 2 {
		t.Errorf("after the retry tick posted calls = %d, want 2", n)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	fs := &fakeSite{unposted: []site.Commission{commission("c1")}}
	p, fd := newPoller(t, fs)
	p.interval = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()

	time.Sleep(40 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if n := fd.count("POST", "/channels/"+channel+"/messages"); n != 1 {
		t.Errorf("messages sent over many ticks = %d, want 1", n)
	}
}

func TestRunAdoptsOrphanMessageAfterRestart(t *testing.T) {
	id := "11111111-1111-1111-1111-111111111111"
	fs := &fakeSite{unposted: []site.Commission{commission(id)}}
	p, fd := newPoller(t, fs)
	fd.history = []map[string]any{botMessage("orphan1", botID, "Commission "+id)}
	p.interval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	waitFor(t, func() bool { return len(fs.callsTo("/posted")) == 1 })
	cancel()
	<-done

	if n := fd.count("POST", "/channels/"+channel+"/messages"); n != 0 {
		t.Errorf("messages sent = %d, want 0", n)
	}
	posted := fs.callsTo("/posted")
	if posted[0].Body != `{"channelId":"`+channel+`","messageId":"orphan1"}` {
		t.Errorf("posted body = %s", posted[0].Body)
	}
}

func TestSeedIgnoresForeignAndMalformedMessages(t *testing.T) {
	id := "22222222-2222-2222-2222-222222222222"
	fs := &fakeSite{unposted: []site.Commission{commission(id)}}
	p, fd := newPoller(t, fs)
	fd.history = []map[string]any{
		botMessage("theirs", other, "Commission "+id),
		botMessage("badid", botID, "Commission ../x"),
		botMessage("nofooter", botID, "something else "+id),
		{"id": "plain", "author": map[string]any{"id": botID}},
	}
	p.adoptMessagesFromBeforeRestart()
	p.Tick(context.Background())

	if n := fd.count("POST", "/channels/"+channel+"/messages"); n != 1 {
		t.Errorf("messages sent = %d, want 1: nothing should have been adopted", n)
	}
	posted := fs.callsTo("/posted")
	if len(posted) != 1 || posted[0].Body != `{"channelId":"`+channel+`","messageId":"m1"}` {
		t.Errorf("posted calls = %+v", posted)
	}
}

func TestSeedFailureStillPosts(t *testing.T) {
	fs := &fakeSite{unposted: []site.Commission{commission("c1")}}
	p, fd := newPoller(t, fs)
	fd.historyFails = true

	p.adoptMessagesFromBeforeRestart()
	p.Tick(context.Background())

	if n := fd.count("POST", "/channels/"+channel+"/messages"); n != 1 {
		t.Errorf("messages sent = %d, want 1", n)
	}
	if n := len(fs.callsTo("/posted")); n != 1 {
		t.Errorf("posted calls = %d, want 1", n)
	}
}

func TestAdoptedMessageIsKeptOnConflict(t *testing.T) {
	id := "33333333-3333-3333-3333-333333333333"
	fs := &fakeSite{unposted: []site.Commission{commission(id)}}
	fs.postedStatusByCall = func(int, string) int { return 409 }
	p, fd := newPoller(t, fs)
	fd.history = []map[string]any{botMessage("orphan1", botID, "Commission "+id)}

	p.adoptMessagesFromBeforeRestart()
	p.Tick(context.Background())
	p.Tick(context.Background())

	if n := fd.count("DELETE", "/messages/orphan1") + fd.count("DELETE", "/channels/orphan1"); n != 0 {
		t.Errorf("deletes of the adopted message = %d, want 0", n)
	}
	if n := len(fs.callsTo("/posted")); n != 1 {
		t.Errorf("posted calls = %d, want 1: a 409 ends the retries", n)
	}
	if n := fd.count("POST", "/channels/"+channel+"/messages"); n != 0 {
		t.Errorf("messages sent = %d, want 0", n)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
