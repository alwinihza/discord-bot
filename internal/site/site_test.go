package site

import (
	"context"
	"errors"
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
	return NewClient(srv.URL+"/", secret), &calls
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
	wrong := NewClient(c.base, "wrong")
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
	_, err := NewClient(base, secret).Info(context.Background(), "Rainy123")
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
	c := NewClient(redirector.URL, secret)
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
