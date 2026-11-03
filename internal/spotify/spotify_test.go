package spotify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mygoify/internal/library"
)

func client(t *testing.T, h http.HandlerFunc) *Client {
	srv := httptest.NewServer(h)
	base := BaseURL
	BaseURL = srv.URL
	t.Cleanup(func() { BaseURL = base; srv.Close() })
	return &Client{Token: func(context.Context) (string, error) { return "tok", nil }}
}

// fakeClock advances waits without consulting the wall clock.
type fakeClock struct{ nanos atomic.Int64 }

func newClock(c *Client) *fakeClock {
	f := new(fakeClock)
	f.nanos.Store(time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	c.now = func() time.Time { return time.Unix(0, f.nanos.Load()) }
	c.after = func(d time.Duration) <-chan time.Time {
		f.nanos.Add(int64(d))
		tick := make(chan time.Time, 1)
		tick <- c.timeNow()
		return tick
	}
	return f
}

// observedContext signals when a call reaches a cancellable wait. For
// a queued call that is before Token; an admitted call reads Token first.
type observedContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *observedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

// deadlineContext lets a test end a deadline through cancellation,
// without waiting on the wall clock.
type deadlineContext struct {
	context.Context
	end time.Time
}

func (c deadlineContext) Deadline() (time.Time, bool) { return c.end, true }

func (c deadlineContext) Err() error {
	if c.Context.Err() != nil {
		return context.DeadlineExceeded
	}
	return nil
}

func gate(t *testing.T) (<-chan struct{}, func()) {
	ch := make(chan struct{})
	open := sync.OnceFunc(func() { close(ch) })
	t.Cleanup(open)
	return ch, open
}

func startCall(c *Client, ctx context.Context, method, path string) <-chan error {
	done := make(chan error, 1)
	go func() { done <- c.Do(ctx, method, path, nil, nil) }()
	return done
}

func await[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second): // Deadlock guard, not a timing assertion.
		t.Fatal("call did not reach its next step")
		var zero T
		return zero
	}
}

// Concurrent reads used to spend quota twice; they now share raw success
// or error answers, decode separately, and do not cache the next read.
// Mutations must still reach the server separately.
func TestRequestSharing(t *testing.T) {
	for _, tc := range []struct {
		method string
		status int
		calls  int32
	}{
		{"GET", http.StatusOK, 1},
		{"GET", http.StatusNotFound, 1},
		{"PUT", http.StatusOK, 2},
		{"POST", http.StatusOK, 2},
		{"DELETE", http.StatusOK, 2},
	} {
		t.Run(tc.method+"/"+http.StatusText(tc.status), func(t *testing.T) {
			var calls atomic.Int32
			entered := make(chan struct{}, 2)
			var release <-chan struct{}
			c := client(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				entered <- struct{}{}
				<-release
				w.WriteHeader(tc.status)
				w.Write([]byte(`{"id":"me","error":{"message":"missing"}}`))
			})
			var open func()
			release, open = gate(t)
			var first struct{ ID string }
			var second map[string]any
			done := make(chan error, 2)
			go func() { done <- c.Do(t.Context(), tc.method, "/me", nil, &first) }()
			await(t, entered)
			ctx := &observedContext{Context: t.Context(), waiting: make(chan struct{})}
			go func() { done <- c.Do(ctx, tc.method, "/me", nil, &second) }()
			await(t, ctx.waiting)
			open()
			for range 2 {
				err := await(t, done)
				if (tc.status == http.StatusOK && err != nil) || (tc.status == http.StatusNotFound && !IsNotFound(err)) {
					t.Fatalf("shared answer: %v", err)
				}
			}
			if calls.Load() != tc.calls {
				t.Fatalf("same %s sent %d requests, want %d", tc.method, calls.Load(), tc.calls)
			}
			if tc.status == http.StatusOK && (first.ID != "me" || second["id"] != "me") {
				t.Fatalf("separate destinations: %+v, %+v", first, second)
			}
			c.Do(t.Context(), tc.method, "/me", nil, nil)
			if calls.Load() != tc.calls+1 {
				t.Fatal("finished call was kept as a pending request")
			}
		})
	}
}

// The first caller's cancellation or expired deadline used to fail every
// caller sharing its GET; a still-valid waiter now takes over the read.
func TestGETOwnerCancellation(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("view closed")} {
		t.Run(cause.Error(), func(t *testing.T) {
			var calls atomic.Int32
			entered := make(chan struct{}, 1)
			var release <-chan struct{}
			c := client(t, func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					entered <- struct{}{}
					select {
					case <-r.Context().Done():
					case <-release:
					}
					return
				}
				w.Write([]byte(`{"id":"me"}`))
			})
			release, _ = gate(t)
			clock := newClock(c)
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			var owner context.Context = ctx
			if cause == context.DeadlineExceeded {
				owner = deadlineContext{ctx, c.timeNow().Add(time.Second)}
			}
			first := startCall(c, owner, "GET", "/me")
			await(t, entered)
			waiter := &observedContext{Context: t.Context(), waiting: make(chan struct{})}
			var me struct{ ID string }
			second := make(chan error, 1)
			go func() { second <- c.Do(waiter, "GET", "/me", nil, &me) }()
			await(t, waiter.waiting)
			clock.nanos.Add(int64(time.Second))
			cancel(cause)
			if err := await(t, first); !errors.Is(err, cause) {
				t.Fatalf("first caller: %v, want %v", err, cause)
			}
			if err := await(t, second); err != nil || me.ID != "me" || calls.Load() != 2 {
				t.Fatalf("surviving waiter: %+v, %v after %d calls", me, err, calls.Load())
			}
		})
	}
}

// A shared GET used to give every caller its owner's limit decision;
// each now compares a newly received cooldown with its own deadline.
func TestGETCallerDeadlines(t *testing.T) {
	for _, shorter := range []string{"owner", "waiter"} {
		t.Run(shorter, func(t *testing.T) {
			var calls atomic.Int32
			entered := make(chan struct{}, 1)
			var release <-chan struct{}
			c := client(t, func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					entered <- struct{}{}
					<-release
					w.Header().Set("Retry-After", "10")
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				w.Write([]byte(`{"id":"me"}`))
			})
			var open func()
			release, open = gate(t)
			clock := newClock(c)
			tick := make(chan time.Time, 1)
			waits := make(chan time.Duration, 1)
			c.after = func(d time.Duration) <-chan time.Time { waits <- d; return tick }
			start := c.timeNow()
			ownerWait, waiterWait := time.Minute, 5*time.Second
			if shorter == "owner" {
				ownerWait, waiterWait = waiterWait, ownerWait
			}
			owner, cancelOwner := context.WithDeadline(t.Context(), start.Add(ownerWait))
			defer cancelOwner()
			waiter, cancelWaiter := context.WithDeadline(t.Context(), start.Add(waiterWait))
			defer cancelWaiter()
			var first, second struct{ ID string }
			firstDone, secondDone := make(chan error, 1), make(chan error, 1)
			go func() { firstDone <- c.Do(owner, "GET", "/me", nil, &first) }()
			await(t, entered)
			observed := &observedContext{Context: waiter, waiting: make(chan struct{})}
			go func() { secondDone <- c.Do(observed, "GET", "/me", nil, &second) }()
			await(t, observed.waiting)
			open()
			limited, remaining, result := secondDone, firstDone, &first
			if shorter == "owner" {
				limited, remaining, result = firstDone, secondDone, &second
			}
			err := await(t, limited)
			var lim *LimitedError
			if !errors.As(err, &lim) || !errors.Is(err, library.ErrLimited) || c.timeNow() != start {
				t.Fatalf("short deadline: %v at %v", err, c.timeNow())
			}
			if d := await(t, waits); d != lim.Until.Sub(start) {
				t.Fatalf("remaining caller waits %v, want %v", d, lim.Until.Sub(start))
			}
			clock.nanos.Store(lim.Until.UnixNano())
			tick <- c.timeNow()
			if err := await(t, remaining); err != nil || result.ID != "me" || calls.Load() != 2 {
				t.Fatalf("long deadline: %+v, %v after %d calls", result, err, calls.Load())
			}
		})
	}
}

// A caller leaving a shared GET does not cancel the first caller's read.
func TestGETWaiterCancellation(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{}, 2)
	var release <-chan struct{}
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
	})
	var open func()
	release, open = gate(t)
	first := startCall(c, t.Context(), "GET", "/me")
	await(t, entered)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	observed := &observedContext{Context: ctx, waiting: make(chan struct{})}
	second := startCall(c, observed, "GET", "/me")
	await(t, observed.waiting)
	cancel()
	if err := await(t, second); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter: %v", err)
	}
	open()
	if err := await(t, first); err != nil || calls.Load() != 1 {
		t.Fatalf("original read: %v after %d requests", err, calls.Load())
	}
}

// A third request used to leave immediately; now it waits for one of the
// first two bodies, including when their headers have already arrived.
func TestConcurrencyLimit(t *testing.T) {
	entered := make(chan string, 3)
	releases := make(map[string]<-chan struct{})
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		entered <- r.URL.Path
		<-releases[r.URL.Path]
	})
	one, openOne := gate(t)
	two, openTwo := gate(t)
	three, openThree := gate(t)
	releases["/one"], releases["/two"], releases["/three"] = one, two, three
	var admitted atomic.Int32
	c.Token = func(context.Context) (string, error) { admitted.Add(1); return "tok", nil }
	first := startCall(c, t.Context(), "GET", "/one")
	await(t, entered)
	second := startCall(c, t.Context(), "PUT", "/two")
	await(t, entered)
	ctx := &observedContext{Context: t.Context(), waiting: make(chan struct{})}
	third := startCall(c, ctx, "DELETE", "/three")
	await(t, ctx.waiting)
	if got := admitted.Load(); got != 2 {
		t.Fatalf("admitted %d calls before a response finished", got)
	}
	openOne()
	if path := await(t, entered); path != "/three" {
		t.Fatalf("next request: %s", path)
	}
	openTwo()
	openThree()
	for _, done := range []<-chan error{first, second, third} {
		if err := await(t, done); err != nil {
			t.Fatal(err)
		}
	}
}

// A 429 used to leave every caller free to retry together; capacity now
// stays at one until five quiet minutes after the cooldown has passed.
func TestConcurrencyAfterRateLimit(t *testing.T) {
	entered := make(chan string, 2)
	releases := make(map[string]<-chan struct{})
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/limited" {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		entered <- r.URL.Path
		<-releases[r.URL.Path]
	})
	one, openOne := gate(t)
	two, openTwo := gate(t)
	releases["/one"], releases["/two"] = one, two
	clock := newClock(c)
	var admitted atomic.Int32
	c.Token = func(context.Context) (string, error) { admitted.Add(1); return "tok", nil }
	err := c.Do(t.Context(), "PUT", "/limited", nil, nil)
	var lim *LimitedError
	if !errors.As(err, &lim) {
		t.Fatalf("rate limit: %v", err)
	}
	clock.nanos.Store(lim.Until.UnixNano())
	type timer struct {
		d    time.Duration
		fire chan time.Time
	}
	timers := make(chan timer, 4)
	c.after = func(d time.Duration) <-chan time.Time {
		tick := make(chan time.Time, 1)
		timers <- timer{d, tick}
		return tick
	}
	first := startCall(c, t.Context(), "GET", "/one")
	await(t, entered)
	ctx := &observedContext{Context: t.Context(), waiting: make(chan struct{})}
	second := startCall(c, ctx, "GET", "/two")
	await(t, ctx.waiting)
	if got := admitted.Load(); got != 2 { // The 429 and one read.
		t.Fatalf("admitted %d calls after a 429", got)
	}
	tick := await(t, timers)
	if tick.d != 5*time.Minute {
		t.Fatalf("recovery window: %v", tick.d)
	}
	clock.nanos.Store(lim.Until.Add(5*time.Minute - time.Nanosecond).UnixNano())
	tick.fire <- c.timeNow()
	tick = await(t, timers)
	if tick.d != time.Nanosecond || admitted.Load() != 2 {
		t.Fatal("restored capacity before the quiet window ended")
	}
	clock.nanos.Add(1)
	tick.fire <- c.timeNow()
	if path := await(t, entered); path != "/two" {
		t.Fatalf("request after recovery: %s", path)
	}
	openOne()
	openTwo()
	for _, done := range []<-chan error{first, second} {
		if err := await(t, done); err != nil {
			t.Fatal(err)
		}
	}
}

// Mutations used to retry 429s, so an old command could take effect later;
// each now returns its cooldown after exactly one attempt.
func TestMutationRateLimit(t *testing.T) {
	for _, method := range []string{"PUT", "POST", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			var calls atomic.Int32
			c := client(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != method {
					t.Errorf("sent %s, want %s", r.Method, method)
				}
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(http.StatusTooManyRequests)
			})
			newClock(c)
			start := c.timeNow()
			err := c.Do(t.Context(), method, "/me/library", nil, nil)
			var lim *LimitedError
			if !errors.As(err, &lim) || !errors.Is(err, library.ErrLimited) || calls.Load() != 1 {
				t.Fatalf("mutation: %v after %d attempts", err, calls.Load())
			}
			if lim.Until.Before(start.Add(2*time.Second)) || lim.Until.After(start.Add(3*time.Second)) || c.timeNow() != start {
				t.Fatalf("cooldown %v, now %v", lim.Until, c.timeNow())
			}
		})
	}
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
	newClock(c)
	start := c.timeNow()
	var me struct{ ID string }
	if err := c.Do(context.Background(), "GET", "/me", nil, &me); err != nil || me.ID != "me" {
		t.Fatalf("got %+v, %v", me, err)
	}
	if waited := c.timeNow().Sub(start); waited < time.Second {
		t.Fatalf("tried again after %v, before Retry-After", waited)
	}
	if calls.Load() != 2 {
		t.Fatalf("got %d calls", calls.Load())
	}

	calls.Store(0)
	c = client(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"status":429,"message":"quota"},"reason":"QUOTA_EXCEEDED"}`))
	})
	newClock(c)
	if err := c.Do(context.Background(), "GET", "/me", nil, nil); !errors.Is(err, ErrQuota) || calls.Load() != 1 {
		t.Fatalf("spent quota: %v after %d calls", err, calls.Load())
	}

	calls.Store(0)
	c = client(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	})
	newClock(c)
	err := c.Do(context.Background(), "GET", "/me", nil, nil)
	var lim *LimitedError
	if !errors.As(err, &lim) || calls.Load() != 5 || !lim.Until.After(c.timeNow()) {
		t.Fatalf("retry budget: %v after %d calls at %v", err, calls.Load(), c.timeNow())
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

// A call Spotify asks to wait for longer than it may take fails at once,
// saying so; the app started again waits on, rather than call and be
// held off longer. Before, each call waited until its deadline, and the
// pages loaded for nothing while Spotify held the app off for hours.
func TestLongRateLimit(t *testing.T) {
	var calls atomic.Int32
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "20000")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	c.HoldFile = filepath.Join(t.TempDir(), "hold")
	clock := newClock(c)
	start := c.timeNow()
	ctx, cancel := context.WithDeadline(context.Background(), start.Add(5*time.Second))
	defer cancel()
	err := c.Do(ctx, "GET", "/me", nil, nil)
	var lim *LimitedError
	if !errors.As(err, &lim) || !errors.Is(err, library.ErrLimited) || c.timeNow() != start {
		t.Fatalf("got %v after %v", err, c.timeNow().Sub(start))
	}
	if lim.Until.Sub(start) < 5*time.Hour {
		t.Errorf("limited until %v", lim.Until)
	}
	again := &Client{Token: c.Token, HoldFile: c.HoldFile, now: c.now, after: c.after}
	if err := again.Do(ctx, "GET", "/me", nil, nil); !errors.Is(err, library.ErrLimited) || calls.Load() != 1 {
		t.Fatalf("started again: %v after %d calls", err, calls.Load())
	}
	if clock.nanos.Load() != start.UnixNano() {
		t.Fatal("waited instead of returning the cooldown")
	}
}
