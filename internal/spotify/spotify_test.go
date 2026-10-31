package spotify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func client(t *testing.T, h http.HandlerFunc) *Client {
	srv := httptest.NewServer(h)
	base := BaseURL
	BaseURL = srv.URL
	t.Cleanup(func() { BaseURL = base; srv.Close() })
	return &Client{Token: func(context.Context) (string, error) { return "tok", nil }}
}

// A rate-limited call waits as long as Spotify says, and longer, then
// succeeds; a spent quota is not tried again.
func TestRateLimit(t *testing.T) {
	var calls atomic.Int32
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("authorization %q", r.Header.Get("Authorization"))
		}
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"id":"me"}`))
	})
	start := time.Now()
	var me struct{ ID string }
	if err := c.Do(context.Background(), "GET", "/me", nil, &me); err != nil || me.ID != "me" {
		t.Fatalf("got %+v, %v", me, err)
	}
	if waited := time.Since(start); waited < time.Second {
		t.Fatalf("tried again after %v, before Retry-After", waited)
	}

	calls.Store(0)
	c = client(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"status":429,"message":"quota"},"reason":"QUOTA_EXCEEDED"}`))
	})
	if err := c.Do(context.Background(), "GET", "/me", nil, nil); !errors.Is(err, ErrQuota) || calls.Load() != 1 {
		t.Fatalf("spent quota: %v after %d calls", err, calls.Load())
	}
}

// Another error says its status, as a 404 does when a device is gone.
func TestError(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"status":404,"message":"Device not found"}}`))
	})
	err := c.Do(context.Background(), "PUT", "/me/player/play", map[string]string{"context_uri": "x"}, nil)
	if !IsNotFound(err) {
		t.Fatalf("got %v", err)
	}
}
