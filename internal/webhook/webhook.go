// Package webhook receives the OBS start and stop streaming events.
package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
)

// Event is a streaming state change reported by OBS.
type Event string

const (
	Started Event = "started"
	Stopped Event = "stopped"
)

const maxBodyBytes = 1 << 10

// Handler serves POST /hooks/obs and GET /healthz. A request to /hooks/obs
// needs the header "Authorization: Bearer <secret>". An empty secret rejects
// every request. onEvent runs on the
// request goroutine before the 202 goes out, so it must not block: start a
// goroutine for slow work.
func Handler(secret string, onEvent func(Event)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /hooks/obs", func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || secret == "" || subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
			log.Printf("go-live webhook: rejected request from %s: missing or wrong secret", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var body struct {
			Event Event `json:"event"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if body.Event != Started && body.Event != Stopped {
			http.Error(w, "unknown event", http.StatusBadRequest)
			return
		}

		onEvent(body.Event)
		w.WriteHeader(http.StatusAccepted)
	})
	return mux
}

// Serve listens on addr until ctx is done, then drains in-flight requests.
// It returns nil after a clean shutdown.
func Serve(ctx context.Context, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
