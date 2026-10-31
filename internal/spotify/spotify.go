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
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"
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

	mu    sync.Mutex
	until time.Time // calls wait until then: Spotify asked them to
}

// maxRetries is how many times a rate-limited call is tried again.
const maxRetries = 4

// Do calls method on path with in, if set, as JSON, and decodes the
// answer into out, if set and there is one. A call Spotify rate-limits
// waits as long as it says, a little longer, and is tried again; meanwhile
// other calls wait too, rather than add to what Spotify refuses.
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	for try := 0; ; try++ {
		if err := c.wait(ctx); err != nil {
			return err
		}
		res, err := c.once(ctx, method, path, body)
		if err != nil {
			return err
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		res.Body.Close()
		switch {
		case res.StatusCode == http.StatusTooManyRequests:
			var e apiError
			json.Unmarshal(b, &e)
			if e.Reason == "QUOTA_EXCEEDED" {
				return ErrQuota
			}
			if try == maxRetries {
				return &Error{res.StatusCode, "rate limited"}
			}
			c.holdOff(retryAfter(res.Header.Get("Retry-After")))
			continue
		case res.StatusCode >= 300:
			var e apiError
			json.Unmarshal(b, &e)
			return &Error{res.StatusCode, e.Error.Message}
		}
		if out != nil && len(b) > 0 {
			return json.Unmarshal(b, out)
		}
		return nil
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
	return h.Do(req)
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

func (c *Client) holdOff(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := time.Now().Add(d); t.After(c.until) {
		c.until = t
	}
}

func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	d := time.Until(c.until)
	c.mu.Unlock()
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
