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
	fs.postedStatus = func(n int, _ string) int {
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
	fs.postedStatus = func(int, string) int { return 409 }
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
