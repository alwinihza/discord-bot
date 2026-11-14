package site

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const secret = "s3cret-value"

func fakeSite(t *testing.T) (*Client, *[]string) {
	t.Helper()
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer "+secret {
			http.Error(w, `{"error":"Forbidden"}`, http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/admin/playlists/Rainy123":
			deleted := r.Method == http.MethodDelete
			w.Write([]byte(`{"id":"Rainy123","name":"Rainy","entryCount":12,"createdAt":"a","updatedAt":"b","deleted":` + map[bool]string{true: "true", false: "false"}[deleted] + `}`))
		case "/admin/playlists/Broken1x":
			http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
		default:
			http.Error(w, `{"error":"No such playlist"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL+"/", secret, "https://fio.example/"), &calls
}

func TestInfoAndDelete(t *testing.T) {
	c, calls := fakeSite(t)
	info, err := c.Info(context.Background(), "Rainy123")
	if err != nil || info.Name != "Rainy" || info.EntryCount != 12 || info.Deleted {
		t.Fatalf("Info = %+v, %v", info, err)
	}
	del, err := c.Delete(context.Background(), "Rainy123")
	if err != nil || !del.Deleted {
		t.Fatalf("Delete = %+v, %v", del, err)
	}
	if got := strings.Join(*calls, ","); got != "GET /admin/playlists/Rainy123,DELETE /admin/playlists/Rainy123" {
		t.Errorf("calls = %s", got)
	}
}

func TestStatusErrors(t *testing.T) {
	c, _ := fakeSite(t)
	if _, err := c.Info(context.Background(), "Missing1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
	if _, err := c.Delete(context.Background(), "Missing1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete unknown id: %v", err)
	}
	wrong := NewClient(c.base, "wrong", "")
	if _, err := wrong.Info(context.Background(), "Rainy123"); !errors.Is(err, ErrForbidden) {
		t.Errorf("wrong secret: %v", err)
	}
	if _, err := c.Info(context.Background(), "Broken1x"); err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrForbidden) {
		t.Errorf("server error: %v", err)
	}
}

func TestUnreachableSiteErrorKeepsSecretAndURLOut(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()
	_, err := NewClient(base, secret, "").Info(context.Background(), "Rainy123")
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), base) {
		t.Errorf("error leaks request details: %v", err)
	}
}

func TestPlaylistID(t *testing.T) {
	cases := []struct {
		in   string
		id   string
		want bool
	}{
		{"https://fionastation.example/p/Rainy123", "Rainy123", true},
		{"https://fionastation.example/p/Rainy123/", "Rainy123", true},
		{"https://fionastation.example/p/Rainy123#edit=abcdefTOKEN", "Rainy123", true},
		{"  http://localhost:3000/p/Rainy123?x=1#edit=t  ", "Rainy123", true},
		{"Rainy123", "Rainy123", true},
		{"https://fionastation.example/", "", false},
		{"https://fionastation.example/p/short", "", false},
		{"https://fionastation.example/p/Rainy123/extra", "", false},
		{"https://fionastation.example/songs/Rainy123", "", false},
		{"javascript:/p/Rainy123", "", false},
		{"Rainy12", "", false},
		{"Rainy1234", "", false},
		{"Rain y12", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		id, ok := PlaylistID(c.in)
		if id != c.id || ok != c.want {
			t.Errorf("PlaylistID(%q) = %q, %v; want %q, %v", c.in, id, ok, c.id, c.want)
		}
	}
}

func TestClientRejectsMalformedIDsBeforeAnyRequest(t *testing.T) {
	c, calls := fakeSite(t)
	for _, id := range []string{"../../x", "a?all=1", "a#frag", "a/../../b", "", "short", "waytoolong1"} {
		if _, err := c.Info(context.Background(), id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Info(%q) err = %v, want ErrInvalidID", id, err)
		}
		if _, err := c.Delete(context.Background(), id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Delete(%q) err = %v, want ErrInvalidID", id, err)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("site saw %v", *calls)
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	var otherHits int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits++
		w.Write([]byte(`{"id":"Rainy123","name":"Impostor","entryCount":1}`))
	}))
	t.Cleanup(other.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/anything", http.StatusFound)
	}))
	t.Cleanup(redirector.Close)
	c := NewClient(redirector.URL, secret, "")
	if s, err := c.Info(context.Background(), "Rainy123"); err == nil {
		t.Errorf("Info followed a redirect and returned %+v", s)
	}
	if _, err := c.Delete(context.Background(), "Rainy123"); err == nil {
		t.Error("Delete accepted a redirect")
	}
	if otherHits != 0 {
		t.Errorf("redirect target was called %d times", otherHits)
	}
}

const alice = "900000000000000011"

type seen struct {
	Method, Path, User, Auth, Body string
}

func ownerSite(t *testing.T, status int, body string) (*Client, *[]seen) {
	t.Helper()
	var got []seen
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got = append(got, seen{r.Method, r.URL.EscapedPath(), r.Header.Get("X-Acting-Discord-Id"), r.Header.Get("Authorization"), string(raw)})
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, secret, "https://fio.example/"), &got
}

func TestOwnerRoutesSendTheActingUserAndContractShape(t *testing.T) {
	const playlist = `{"id":"Rainy123","name":"Rainy","entries":[{"songId":"s1","title":"Halu"}],"version":3}`
	c, got := ownerSite(t, 200, playlist)
	ctx := context.Background()
	if _, err := c.Create(ctx, alice, "Rainy", "s1"); err != nil {
		t.Fatal(err)
	}
	p, err := c.Get(ctx, alice, "Rainy123")
	if err != nil || p.Name != "Rainy" || len(p.Entries) != 1 || p.Entries[0].Title != "Halu" {
		t.Fatalf("Get = %+v, %v", p, err)
	}
	c.Rename(ctx, alice, "Rainy123", "New")
	c.AddSong(ctx, alice, "Rainy123", "s2")
	c.RemoveSong(ctx, alice, "Rainy123", "s/1 ?")
	c.Remove(ctx, alice, "Rainy123")
	want := []seen{
		{"POST", "/admin/owner/playlists", alice, "Bearer " + secret, `{"name":"Rainy","songIds":["s1"]}`},
		{"GET", "/admin/owner/playlists/Rainy123", alice, "Bearer " + secret, ""},
		{"PUT", "/admin/owner/playlists/Rainy123/name", alice, "Bearer " + secret, `{"name":"New"}`},
		{"POST", "/admin/owner/playlists/Rainy123/songs", alice, "Bearer " + secret, `{"songId":"s2"}`},
		{"DELETE", "/admin/owner/playlists/Rainy123/songs/s%2F1%20%3F", alice, "Bearer " + secret, ""},
		{"DELETE", "/admin/owner/playlists/Rainy123", alice, "Bearer " + secret, ""},
	}
	if len(*got) != len(want) {
		t.Fatalf("requests = %+v", *got)
	}
	for n := range want {
		if (*got)[n] != want[n] {
			t.Errorf("request %d = %+v, want %+v", n, (*got)[n], want[n])
		}
	}
}

func TestMineListsSummaries(t *testing.T) {
	c, got := ownerSite(t, 200, `{"playlists":[{"id":"Rainy123","name":"Rainy","entryCount":2}]}`)
	list, err := c.Mine(context.Background(), alice)
	if err != nil || len(list) != 1 || list[0].EntryCount != 2 {
		t.Fatalf("Mine = %+v, %v", list, err)
	}
	if (*got)[0].Path != "/admin/owner/playlists" || (*got)[0].User != alice {
		t.Errorf("request = %+v", (*got)[0])
	}
}

func TestOwnerStatusesMapToTypedErrors(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   error
	}{
		{403, `{"error":"This playlist belongs to someone else"}`, ErrNotOwner},
		{403, `{"error":"Forbidden"}`, ErrForbidden},
		{404, `{"error":"No such playlist"}`, ErrNotFound},
		{404, `{"error":"Not Found"}`, ErrNoOwnerAPI},
		{404, `{"error":"That song is not in the playlist"}`, ErrSongNotInPlaylist},
		{409, `{"error":"The playlist changed. Reload it."}`, ErrConflict},
		{410, `{"error":"This playlist was deleted"}`, ErrDeleted},
		{429, `{"error":"Too many playlists from this member. Try later."}`, ErrRateLimited},
		{503, `{"error":"Too many playlists were saved today. Try tomorrow."}`, ErrDailyCap},
	}
	for _, tc := range cases {
		c, _ := ownerSite(t, tc.status, tc.body)
		if _, err := c.Get(context.Background(), alice, "Rainy123"); !errors.Is(err, tc.want) {
			t.Errorf("%d %s: err = %v, want %v", tc.status, tc.body, err, tc.want)
		}
	}
	c, _ := ownerSite(t, 400, `{"error":"Each song may appear once"}`)
	_, err := c.AddSong(context.Background(), alice, "Rainy123", "s1")
	var rule *RuleError
	if !errors.As(err, &rule) || rule.Message != "Each song may appear once" {
		t.Errorf("400: err = %v", err)
	}
	c, _ = ownerSite(t, 500, `{"error":"boom"}`)
	if _, err := c.Get(context.Background(), alice, "Rainy123"); err == nil || errors.Is(err, ErrNotOwner) {
		t.Errorf("500: err = %v", err)
	}
}

func TestOwnerCallsRejectBadInputsBeforeAnyRequest(t *testing.T) {
	c, got := ownerSite(t, 200, `{}`)
	ctx := context.Background()
	for _, user := range []string{"", "abc", "9000000000000000111111", "9000000000000000 1", "900000000000000011\r\nX-Evil: 1"} {
		if _, err := c.Mine(ctx, user); !errors.Is(err, ErrInvalidUser) {
			t.Errorf("Mine(%q) err = %v, want ErrInvalidUser", user, err)
		}
	}
	for _, id := range []string{"../../x", "a?all=1", "short", ""} {
		if _, err := c.Get(ctx, alice, id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Get(%q) err = %v", id, err)
		}
		if err := c.Remove(ctx, alice, id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Remove(%q) err = %v", id, err)
		}
	}
	if _, err := c.RemoveSong(ctx, alice, "Rainy123", ""); !errors.Is(err, ErrInvalidID) {
		t.Errorf("RemoveSong with empty song: %v", err)
	}
	if len(*got) != 0 {
		t.Errorf("site saw %+v", *got)
	}
}

func TestOwnerRoutesDoNotFollowRedirects(t *testing.T) {
	var hits int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	t.Cleanup(other.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)
	if _, err := NewClient(redirector.URL, secret, "").Mine(context.Background(), alice); err == nil || hits != 0 {
		t.Errorf("err = %v, redirect target hits = %d", err, hits)
	}
}

func TestViewLinkTrimsTheBase(t *testing.T) {
	c := NewClient("http://x", secret, "https://fio.example/")
	if got := c.ViewLink("Rainy123"); got != "https://fio.example/p/Rainy123" {
		t.Errorf("ViewLink = %s", got)
	}
}
