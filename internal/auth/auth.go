// Package auth logs the listener in to Spotify's Web API: the
// authorization code flow with PKCE, in the browser, with the app's own
// client ID and no secret. A Session keeps the token, refreshing it before
// it runs out, in a Store.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Scopes are what the app asks to do, and no more (DESIGN.md §5.1): no
// email nor profile.
var Scopes = []string{
	"user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing",
	"user-read-recently-played", "user-top-read", "user-library-read", "user-library-modify",
	"playlist-read-private", "playlist-read-collaborative", "playlist-modify-private", "playlist-modify-public",
	// Following artists, and whether the listener does.
	"user-follow-read", "user-follow-modify",
}

// These are Spotify's; tests replace them.
var (
	authorizeURL = "https://accounts.spotify.com/authorize"
	tokenURL     = "https://accounts.spotify.com/api/token"
)

// Config is the app registered with Spotify: its client ID, and the
// loopback address its dashboard lists to send the listener back to.
type Config struct {
	ClientID    string
	RedirectURI string
}

type Token struct {
	Access  string    `json:"access_token"`
	Refresh string    `json:"refresh_token"`
	Scope   string    `json:"scope"`
	Expiry  time.Time `json:"expiry"`
}

// ErrDenied is what Login returns when the listener refuses.
var ErrDenied = errors.New("auth: the listener refused")

func random(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Login has the listener log in in the browser, which open shows the
// page of, and returns the token Spotify gives once they have, or when
// ctx is done, its error.
func Login(ctx context.Context, cfg Config, open func(url string) error) (*Token, error) {
	redirect, err := url.Parse(cfg.RedirectURI)
	if err != nil || redirect.Scheme != "http" || redirect.Hostname() != "127.0.0.1" {
		return nil, fmt.Errorf("auth: the redirect URI %q is not of the loopback address 127.0.0.1", cfg.RedirectURI)
	}
	verifier := random(64)
	sum := sha256.Sum256([]byte(verifier))
	state := random(16)
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {cfg.RedirectURI},
		"scope":                 {strings.Join(Scopes, " ")},
		"state":                 {state},
		"code_challenge_method": {"S256"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
	}

	ln, err := net.Listen("tcp4", redirect.Host)
	if err != nil {
		return nil, fmt.Errorf("auth: listening for the browser: %w", err)
	}
	type answer struct {
		code string
		err  error
	}
	answers := make(chan answer, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != redirect.Path {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		var a answer
		switch {
		case q.Get("state") != state:
			// Not the login asked for: a page of another, or of someone
			// else's making.
			http.Error(w, "MyGoify：登录请求不匹配，请回到 MyGoify 重新登录。", http.StatusBadRequest)
			return
		case q.Get("error") == "access_denied":
			a.err = ErrDenied
		case q.Get("error") != "":
			a.err = fmt.Errorf("auth: Spotify answered %s", q.Get("error"))
		default:
			a.code = q.Get("code")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if a.err != nil {
			fmt.Fprint(w, page("登录未完成", "请回到 MyGoify 再试一次。"))
		} else {
			fmt.Fprint(w, page("已登录", "可以关闭这个页面，回到 MyGoify。"))
		}
		select {
		case answers <- a:
		default:
		}
	})}
	go srv.Serve(ln)
	defer srv.Close()

	if err := open(authorizeURL + "?" + q.Encode()); err != nil {
		return nil, fmt.Errorf("auth: opening the browser: %w", err)
	}
	var a answer
	select {
	case a = <-answers:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if a.err != nil {
		return nil, a.err
	}
	return requestToken(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {a.code},
		"redirect_uri":  {cfg.RedirectURI},
		"client_id":     {cfg.ClientID},
		"code_verifier": {verifier},
	})
}

func page(title, body string) string {
	return `<!doctype html><meta charset="utf-8"><title>MyGoify</title>` +
		`<body style="font:16px system-ui,sans-serif;display:grid;place-items:center;height:90vh;margin:0">` +
		`<div style="text-align:center"><h1 style="font-size:22px">` + title + `</h1><p style="color:#666">` + body + `</p></div>`
}

// Refresh trades a refresh token for a new token.
func Refresh(ctx context.Context, cfg Config, refresh string) (*Token, error) {
	tok, err := requestToken(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh},
		"client_id":     {cfg.ClientID},
	})
	if err == nil && tok.Refresh == "" {
		// Spotify may keep the refresh token as it was.
		tok.Refresh = refresh
	}
	return tok, err
}

// ErrRevoked is what Refresh returns when Spotify no longer takes the
// refresh token: the listener must log in again.
var ErrRevoked = errors.New("auth: the login was revoked")

var client = &http.Client{Timeout: 30 * time.Second}

func requestToken(ctx context.Context, form url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		var e struct{ Error string }
		json.Unmarshal(body, &e)
		if res.StatusCode == http.StatusBadRequest && e.Error == "invalid_grant" {
			return nil, ErrRevoked
		}
		return nil, fmt.Errorf("auth: the token endpoint answered %s: %s", res.Status, e.Error)
	}
	var t struct {
		Token
		ExpiresIn int `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, err
	}
	t.Expiry = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	return &t.Token, nil
}

// Store keeps the token, across launches.
type Store interface {
	Load() ([]byte, error)
	Save([]byte) error
	Delete() error
}

// Session is the listener's login: the token, kept in a Store, and
// refreshed as it nears its end. Its methods are safe from any goroutine.
type Session struct {
	cfg   Config
	store Store

	mu  sync.Mutex
	tok *Token
}

func NewSession(cfg Config, store Store) *Session {
	return &Session{cfg: cfg, store: store}
}

// Restore takes up the login kept, and reports whether there was one.
func (s *Session) Restore() bool {
	b, err := s.store.Load()
	if err != nil {
		return false
	}
	var tok Token
	if json.Unmarshal(b, &tok) != nil || tok.Refresh == "" {
		return false
	}
	s.mu.Lock()
	s.tok = &tok
	s.mu.Unlock()
	return true
}

// Login has the listener log in, as Login does, and keeps the token.
func (s *Session) Login(ctx context.Context, open func(url string) error) error {
	tok, err := Login(ctx, s.cfg, open)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keep(tok)
}

// keep makes tok the token, and stores it. s.mu is held.
func (s *Session) keep(tok *Token) error {
	s.tok = tok
	b, _ := json.Marshal(tok)
	return s.store.Save(b)
}

// refreshBefore is how long before the token runs out it is refreshed.
const refreshBefore = time.Minute

// AccessToken returns a token good for a minute at least, refreshing the
// one kept if it isn't. Without a login, or with one Spotify revoked, it
// returns ErrRevoked.
func (s *Session) AccessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tok == nil {
		return "", ErrRevoked
	}
	if time.Until(s.tok.Expiry) > refreshBefore {
		return s.tok.Access, nil
	}
	tok, err := Refresh(ctx, s.cfg, s.tok.Refresh)
	if errors.Is(err, ErrRevoked) {
		s.tok = nil
		s.store.Delete()
	}
	if err != nil {
		return "", err
	}
	if err := s.keep(tok); err != nil {
		return "", err
	}
	return tok.Access, nil
}

// LoggedIn reports whether there is a login, which may still turn out
// revoked.
func (s *Session) LoggedIn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tok != nil
}

// Logout forgets the login, here: Spotify keeps it until the listener
// removes the app from their account.
func (s *Session) Logout() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tok = nil
	s.store.Delete()
}
