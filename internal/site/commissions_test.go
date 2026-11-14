package site

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleCommissions = `[{"id":"c1","kind":"voice","createdAt":"2026-01-01T00:00:00Z","status":"received","next":["quoted","declined"],
"fields":{"name":"Rin","rush":true,"tags":["a","b"],"extra":7},
"quote":{"_tag":"Priced","lines":[{"label":"Base","idr":1500000,"usd":95}],"total":{"idr":1500000,"usd":95},"deposit":null}}]`

func commissionSite(t *testing.T, status int, body string) (*Client, *http.Request, *string) {
	t.Helper()
	var got http.Request
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, secret, "https://fio.example"), &got, &gotBody
}

func TestUnposted(t *testing.T) {
	c, req, _ := commissionSite(t, 200, sampleCommissions)
	list, err := c.Unposted(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("Unposted = %+v, %v", list, err)
	}
	if req.Method != "GET" || req.URL.Path != "/admin/commissions/unposted" || req.Header.Get("Authorization") != "Bearer "+secret {
		t.Errorf("request = %s %s auth=%q", req.Method, req.URL.Path, req.Header.Get("Authorization"))
	}
	got := list[0]
	if got.Kind != "voice" || got.Next[1] != "declined" || got.Fields["rush"] != true || got.Quote.Total.IDR != 1500000 || got.Quote.Deposit != nil {
		t.Errorf("decoded = %+v", got)
	}
}

func TestMarkPostedStatuses(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   error
	}{
		{200, `{}`, nil},
		{409, `{"error":"x"}`, ErrCommissionConflict},
		{404, `{"error":"No such commission"}`, ErrCommissionNotFound},
		{403, `{"error":"Forbidden"}`, ErrForbidden},
	}
	for _, tc := range cases {
		c, req, body := commissionSite(t, tc.status, tc.body)
		err := c.MarkPosted(context.Background(), "c1", "chan", "msg")
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: err = %v, want %v", tc.status, err, tc.want)
		}
		if req.Method != "POST" || req.URL.Path != "/admin/commissions/c1/posted" || *body != `{"channelId":"chan","messageId":"msg"}` {
			t.Errorf("status %d: %s %s %s", tc.status, req.Method, req.URL.Path, *body)
		}
	}
	var fail error
	c, _, _ := commissionSite(t, 500, `boom`)
	if fail = c.MarkPosted(context.Background(), "c1", "a", "b"); fail == nil {
		t.Error("500 should be an error")
	}
}

func TestSendEvent(t *testing.T) {
	user := "123456789012345678"
	c, req, body := commissionSite(t, 200, `{"id":"c1","status":"quoted","next":["deposit_paid"]}`)
	got, err := c.SendEvent(context.Background(), user, "c1", "quoted", "Rp1jt")
	if err != nil || got.Status != "quoted" {
		t.Fatalf("SendEvent = %+v, %v", got, err)
	}
	if req.Header.Get("X-Acting-Discord-Id") != user || *body != `{"note":"Rp1jt","status":"quoted"}` || req.URL.Path != "/admin/commissions/c1/events" {
		t.Errorf("request headers=%v body=%s path=%s", req.Header, *body, req.URL.Path)
	}

	c, _, body = commissionSite(t, 200, `{"id":"c1","status":"in_progress"}`)
	c.SendEvent(context.Background(), user, "c1", "in_progress", "")
	if *body != `{"status":"in_progress"}` {
		t.Errorf("empty note should be omitted, body = %s", *body)
	}

	c, _, _ = commissionSite(t, 400, `{"error":"cannot go from final to quoted"}`)
	_, err = c.SendEvent(context.Background(), user, "c1", "quoted", "")
	var rule *RuleError
	if !errors.As(err, &rule) || rule.Message != "cannot go from final to quoted" {
		t.Errorf("400 err = %v", err)
	}

	c, _, _ = commissionSite(t, 404, `{"error":"No such commission"}`)
	if _, err = c.SendEvent(context.Background(), user, "c1", "quoted", ""); !errors.Is(err, ErrCommissionNotFound) {
		t.Errorf("404 err = %v", err)
	}
}

func TestCommissionInputsAreChecked(t *testing.T) {
	c, req, _ := commissionSite(t, 200, `{}`)
	if _, err := c.SendEvent(context.Background(), "not-a-snowflake", "c1", "quoted", ""); !errors.Is(err, ErrInvalidUser) {
		t.Errorf("bad user err = %v", err)
	}
	if _, err := c.SendEvent(context.Background(), "123456789012345678", "../admin", "quoted", ""); !errors.Is(err, ErrInvalidCommission) {
		t.Errorf("bad id err = %v", err)
	}
	if err := c.MarkPosted(context.Background(), "a/b", "x", "y"); !errors.Is(err, ErrInvalidCommission) {
		t.Errorf("bad id err = %v", err)
	}
	if req.Method != "" {
		t.Errorf("a request left the client: %s %s", req.Method, req.URL.Path)
	}
}
