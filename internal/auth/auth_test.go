package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// spotify stands in for Spotify's accounts service: it checks the PKCE
// verifier against the challenge the browser brought, and refreshes as
// told to.
type spotify struct {
	mu        sync.Mutex
	challenge string // of the login under way
	refreshes int
	revoked   bool
}

func (s *spotify) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	s.mu.Lock()
	defer s.mu.Unlock()
	reply := map[string]any{"access_token": "access", "expires_in": 3600, "scope": "playlist-read-private"}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if r.Form.Get("code") != "the-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != s.challenge {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		reply["refresh_token"] = "refresh"
	case "refresh_token":
		if s.revoked {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		s.refreshes++
		reply["access_token"] = fmt.Sprint("access", s.refreshes)
	}
	json.NewEncoder(w).Encode(reply)
}

// fakeSpotify points the package at a spotify of the test's.
func fakeSpotify(t *testing.T) *spotify {
	s := &spotify{}
	srv := httptest.NewServer(s)
	tokenURL0 := tokenURL
	tokenURL = srv.URL
	t.Cleanup(func() { tokenURL = tokenURL0; srv.Close() })
	return s
}

func config(t *testing.T) Config {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return Config{ClientID: "client", RedirectURI: fmt.Sprintf("http://127.0.0.1:%d/callback", port)}
}

// browser is what open does in the tests: it goes back to the app as
// Spotify would send it, with code, state and error taken from answer.
func browser(s *spotify, answer func(state string) url.Values) func(string) error {
	return func(page string) error {
		u, _ := url.Parse(page)
		q := u.Query()
		s.mu.Lock()
		s.challenge = q.Get("code_challenge")
		s.mu.Unlock()
		go func() {
			res, err := http.Get(q.Get("redirect_uri") + "?" + answer(q.Get("state")).Encode())
			if err == nil {
				res.Body.Close()
			}
		}()
		return nil
	}
}

// Logging in trades the code the browser brings back, with the verifier of
// the challenge it took, for a token.
func TestLogin(t *testing.T) {
	s := fakeSpotify(t)
	tok, err := Login(context.Background(), config(t), browser(s, func(state string) url.Values {
		return url.Values{"code": {"the-code"}, "state": {state}}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if tok.Access != "access" || tok.Refresh != "refresh" || time.Until(tok.Expiry) < 59*time.Minute {
		t.Fatalf("token %+v", tok)
	}
}

// A visit to the callback with another state, as of a page that isn't the
// login's, is turned away, and the login waits on for its own.
func TestLoginIgnoresOtherState(t *testing.T) {
	s := fakeSpotify(t)
	cfg := config(t)
	tok, err := Login(context.Background(), cfg, func(page string) error {
		res, err := http.Get(cfg.RedirectURI + "?code=forged&state=other")
		if err != nil {
			return err
		}
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("a forged callback answered %s", res.Status)
		}
		return browser(s, func(state string) url.Values {
			return url.Values{"code": {"the-code"}, "state": {state}}
		})(page)
	})
	if err != nil || tok.Access != "access" {
		t.Fatalf("token %+v, %v", tok, err)
	}
}

// The listener refusing ends the login with ErrDenied, and a context done
// ends it with the context's error.
func TestLoginEnds(t *testing.T) {
	s := fakeSpotify(t)
	_, err := Login(context.Background(), config(t), browser(s, func(state string) url.Values {
		return url.Values{"error": {"access_denied"}, "state": {state}}
	}))
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("refused: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = Login(ctx, config(t), func(string) error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed out: %v", err)
	}
	if _, err := Login(context.Background(), Config{RedirectURI: "http://localhost:8000/callback"}, nil); err == nil {
		t.Fatal("logged in with a redirect to localhost, which Spotify refuses")
	}
}

type memStore struct{ b []byte }

func (m *memStore) Load() ([]byte, error) {
	if m.b == nil {
		return nil, errors.New("none")
	}
	return m.b, nil
}
func (m *memStore) Save(b []byte) error { m.b = b; return nil }
func (m *memStore) Delete() error       { m.b = nil; return nil }

// A session refreshes a token near its end and keeps the new one, with
// the refresh token Spotify kept if it gave no other; a refresh token
// Spotify no longer takes logs the listener out.
func TestSessionRefreshes(t *testing.T) {
	s := fakeSpotify(t)
	store := &memStore{}
	old, _ := json.Marshal(Token{Access: "old", Refresh: "refresh", Expiry: time.Now().Add(30 * time.Second)})
	store.Save(old)
	sess := NewSession(config(t), store)
	if !sess.Restore() {
		t.Fatal("no login restored")
	}
	tok, err := sess.AccessToken(context.Background())
	if err != nil || tok != "access1" {
		t.Fatalf("token %q, %v", tok, err)
	}
	var kept Token
	json.Unmarshal(store.b, &kept)
	if kept.Access != "access1" || kept.Refresh != "refresh" {
		t.Fatalf("kept %+v", kept)
	}
	// Good for an hour: no refresh.
	if tok, _ := sess.AccessToken(context.Background()); tok != "access1" || s.refreshes != 1 {
		t.Fatalf("token %q after %d refreshes", tok, s.refreshes)
	}

	s.revoked = true
	sess.tok.Expiry = time.Now()
	if _, err := sess.AccessToken(context.Background()); !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked: %v", err)
	}
	if sess.LoggedIn() || store.b != nil {
		t.Fatal("still logged in after Spotify revoked the login")
	}
}
