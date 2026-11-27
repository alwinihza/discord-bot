package webhook

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testSecret = "s3cret"

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		auth       string
		body       string
		wantStatus int
		wantEvents []Event
	}{
		{"missing auth", "POST", "/hooks/obs", "", `{"event":"started"}`, 401, nil},
		{"wrong secret", "POST", "/hooks/obs", "Bearer nope", `{"event":"started"}`, 401, nil},
		{"wrong scheme", "POST", "/hooks/obs", testSecret, `{"event":"started"}`, 401, nil},
		{"started", "POST", "/hooks/obs", "Bearer " + testSecret, `{"event":"started"}`, 202, []Event{Started}},
		{"stopped", "POST", "/hooks/obs", "Bearer " + testSecret, `{"event":"stopped"}`, 202, []Event{Stopped}},
		{"unknown event", "POST", "/hooks/obs", "Bearer " + testSecret, `{"event":"paused"}`, 400, nil},
		{"missing event", "POST", "/hooks/obs", "Bearer " + testSecret, `{}`, 400, nil},
		{"bad json", "POST", "/hooks/obs", "Bearer " + testSecret, `{"event":`, 400, nil},
		{"oversize body", "POST", "/hooks/obs", "Bearer " + testSecret, `{"event":"started","pad":"` + strings.Repeat("x", 2048) + `"}`, 413, nil},
		{"GET on hook", "GET", "/hooks/obs", "Bearer " + testSecret, "", 405, nil},
		{"healthz without auth", "GET", "/healthz", "", "", 200, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []Event
			h := Handler(testSecret, func(e Event) { got = append(got, e) })

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if len(got) != len(tt.wantEvents) || (len(got) == 1 && got[0] != tt.wantEvents[0]) {
				t.Errorf("events = %v, want %v", got, tt.wantEvents)
			}
		})
	}
}

func TestEmptySecretRejectsEverything(t *testing.T) {
	called := false
	h := Handler("", func(Event) { called = true })
	for _, auth := range []string{"", "Bearer ", "Bearer x"} {
		req := httptest.NewRequest("POST", "/hooks/obs", strings.NewReader(`{"event":"started"}`))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Errorf("auth %q: status = %d, want 401", auth, rec.Code)
		}
	}
	if called {
		t.Error("onEvent ran with an empty secret")
	}
}

func TestHealthzBody(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(testSecret, func(Event) {}).ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want ok", rec.Body.String())
	}
}

func TestServeStopsOnCancel(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, addr, Handler(testSecret, func(Event) {})) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never came up: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}
