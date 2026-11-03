// Package spotify calls Spotify's Web API for the listener: it brings the
// token, waits out rate limits as Spotify asks, and tells a quota spent
// from a rate limit (DESIGN.md §5.2).
package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"mygoify/internal/library"
)

// BaseURL is the Web API's; tests replace it.
var BaseURL = "https://api.spotify.com/v1"

// ErrQuota is what a call returns when the developer account spent its
// quota: waiting a little does not help, so it is not tried again.
var ErrQuota = errors.New("spotify: the app's quota is spent")

// Error is an answer of the Web API other than a success.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("spotify: %d %s", e.Status, e.Message) }

// IsNotFound reports whether err is a 404 of the Web API.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// Client calls the Web API with the tokens Token gives.
type Client struct {
	Token func(ctx context.Context) (string, error)
	HTTP  *http.Client

	// HoldFile, if set, keeps how long Spotify asked calls to wait
	// across restarts.
	HoldFile string

	mu           sync.Mutex
	until        time.Time // calls wait until then: Spotify asked them to
	holdRead     bool
	active       int
	changed      chan struct{}
	reducedUntil time.Time
	pending      map[string]*pending

	now   func() time.Time
	after func(time.Duration) <-chan time.Time
}

// pending keeps the raw answer: callers decode into their own values.
// The first caller's context bounds the shared request; others may leave
// without cancelling it.
type pending struct {
	done chan struct{}
	body []byte
	err  error
}

// maxRetries is how many times a rate-limited call is tried again.
const maxRetries = 4

// Do calls method on path with in, if set, as JSON, and decodes the
// answer into out, if set and there is one. Concurrent GETs of the same
// path share an answer. Only GETs retry a rate limit; mutations fail with
// a LimitedError, so an old command cannot run later without the listener.
// A wait past ctx's deadline also fails at once with a LimitedError.
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	var b []byte
	var err error
	if method == http.MethodGet {
		b, err = c.get(ctx, path, body)
	} else {
		b, err = c.request(ctx, method, path, body)
	}
	if err != nil {
		return err
	}
	if out != nil && len(b) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, body []byte) ([]byte, error) {
	c.mu.Lock()
	if p := c.pending[path]; p != nil {
		c.mu.Unlock()
		select {
		case <-p.done:
			return p.body, p.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	p := &pending{done: make(chan struct{})}
	if c.pending == nil {
		c.pending = make(map[string]*pending)
	}
	c.pending[path] = p
	c.mu.Unlock()

	p.body, p.err = c.request(ctx, http.MethodGet, path, body)
	c.mu.Lock()
	delete(c.pending, path)
	close(p.done)
	c.mu.Unlock()
	return p.body, p.err
}

func (c *Client) request(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	for try := 0; ; try++ {
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
		res, err := c.once(ctx, method, path, body)
		if err != nil {
			c.release()
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		res.Body.Close()
		var until time.Time
		if res.StatusCode == http.StatusTooManyRequests {
			// Publish the cooldown before giving another call the slot.
			until = c.holdOff(retryAfter(res.Header.Get("Retry-After")))
		}
		c.release()
		if err != nil {
			return nil, err
		}
		switch {
		case res.StatusCode == http.StatusTooManyRequests:
			var e apiError
			json.Unmarshal(b, &e)
			if e.Reason == "QUOTA_EXCEEDED" {
				return nil, ErrQuota
			}
			if method != http.MethodGet || try == maxRetries {
				return nil, &LimitedError{until}
			}
			continue
		case res.StatusCode >= 300:
			var e apiError
			json.Unmarshal(b, &e)
			return nil, &Error{res.StatusCode, e.Error.Message}
		}
		return b, nil
	}
}

func (c *Client) once(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	tok, err := c.Token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	h := c.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	debugf("web api: %s %s %s", method, path, body)
	return h.Do(req)
}

// debug, with MYGOIFY_DEBUG set, logs the calls to the Web API, to line
// them up with what the listener heard.
var debug = os.Getenv("MYGOIFY_DEBUG") != ""

func debugf(format string, args ...any) {
	if debug {
		log.Printf(format, args...)
	}
}

// apiError is how the Web API says what went wrong. A spent quota says
// so in reason (DESIGN.md §5.3).
type apiError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
	Reason string `json:"reason"`
}

// retryAfter reads Retry-After, in seconds, and adds up to a third more:
// Spotify often refuses again a call tried again just when it said.
func retryAfter(h string) time.Duration {
	s, err := strconv.Atoi(h)
	if err != nil || s < 1 {
		s = 1
	}
	d := time.Duration(s) * time.Second
	return d + time.Duration(rand.Int64N(int64(d)/3+1))
}

// LimitedError is what a call returns when Spotify asked for a longer
// wait than the call may take: it may ask for hours.
type LimitedError struct {
	Until time.Time
}

func (e *LimitedError) Error() string {
	return "spotify: rate limited until " + e.Until.Format(time.DateTime)
}

func (e *LimitedError) Is(target error) bool { return target == library.ErrLimited }

func (c *Client) timeNow() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// Stay at one request for five quiet minutes after the cooldown ends:
// a full clean window lets Spotify's rolling limit settle before trying
// two again, even when Retry-After itself lasted hours.
const recoveryWindow = 5 * time.Minute

func (c *Client) holdOff(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadHold()
	if t := c.timeNow().Add(d); t.After(c.until) {
		c.until = t
		if c.HoldFile != "" {
			os.MkdirAll(filepath.Dir(c.HoldFile), 0o700)
			os.WriteFile(c.HoldFile, []byte(t.Format(time.RFC3339)), 0o600)
		}
	}
	c.reducedUntil = c.until.Add(recoveryWindow)
	c.notify()
	return c.until
}

// loadHold reads the wait Spotify asked for before the app last quit,
// once: calling again at once would only lengthen it.
func (c *Client) loadHold() {
	if c.HoldFile == "" || c.holdRead {
		return
	}
	c.holdRead = true
	if b, err := os.ReadFile(c.HoldFile); err == nil {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b))); err == nil && t.After(c.until) {
			c.until = t
			c.reducedUntil = t.Add(recoveryWindow)
		}
	}
}

// notify wakes queued calls to recheck both the cooldown and capacity.
// The caller holds mu.
func (c *Client) notify() {
	if c.changed != nil {
		close(c.changed)
		c.changed = nil
	}
}

func (c *Client) release() {
	c.mu.Lock()
	c.active--
	c.notify()
	c.mu.Unlock()
}

// wait takes a slot only when the global cooldown and capacity allow it.
// A slot lasts through reading the response body, not just its headers.
func (c *Client) wait(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.mu.Lock()
		c.loadHold()
		now := c.timeNow()
		until := c.until
		d := until.Sub(now)
		limit := 2
		if now.Before(c.reducedUntil) {
			limit = 1
		}
		if d <= 0 && c.active < limit {
			c.active++
			c.mu.Unlock()
			return nil
		}
		if end, ok := ctx.Deadline(); d > 0 && ok && until.After(end) {
			c.mu.Unlock()
			return &LimitedError{until}
		}
		if d <= 0 && limit == 1 {
			d = c.reducedUntil.Sub(now)
		}
		if c.changed == nil {
			c.changed = make(chan struct{})
		}
		changed := c.changed
		c.mu.Unlock()

		var timer <-chan time.Time
		if d > 0 {
			after := c.after
			if after == nil {
				after = time.After
			}
			timer = after(d)
		}
		select {
		case <-timer:
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
