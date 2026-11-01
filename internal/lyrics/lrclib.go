package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// LRCLIB finds timed lyrics on LRCLIB (lrclib.net), an open library of
// lyrics that needs no key (DESIGN.md §7).
type LRCLIB struct {
	HTTP *http.Client
	// Base is LRCLIB's address, https://lrclib.net unless a test sets it.
	Base string

	pause time.Duration // before asking again a server too busy
}

func (l *LRCLIB) Name() string { return "lrclib" }

// A Query is the track to find lyrics of.
type Query struct {
	ID       string // what the cache keeps it as
	Name     string
	Artist   string
	Album    string
	Duration time.Duration
}

const (
	// near is how far apart a track and its lyrics' track can be in
	// length: as LRCLIB's own matching allows, and a little more.
	near = 3 * time.Second
	// nearest is as far apart they can be of tracks found by name alone.
	nearest = time.Second
	// tries is how many times a request is made of a busy LRCLIB.
	tries = 5
)

// userAgent is how LRCLIB knows the app, as it asks.
const userAgent = "MyGoify (https://github.com/aronnaxlin/mygoify)"

// record is LRCLIB's answer for a track.
type record struct {
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"` // seconds
	Instrumental bool    `json:"instrumental"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

// Find returns the timed lines of q, or none if LRCLIB has none, and an
// error only if it couldn't be asked.
func (l *LRCLIB) Find(ctx context.Context, q Query) (Found, error) {
	lrc, err := l.lookup(ctx, q)
	// What LRCLIB refuses it refuses again: kept as none, not asked again
	// and again as if it were down.
	if errors.Is(err, errRefused) {
		lrc, err = "", nil
	}
	if err != nil {
		return Found{}, err
	}
	return Found{LRC: lrc}, nil
}

// lookup asks LRCLIB for q's track as it is named, then searches for it,
// then for it named without what Spotify adds, as "- Remastered 2011",
// and last for any artist's: Spotify may name an artist in a script
// LRCLIB doesn't, as "Rene Liu" for 刘若英.
// Asked for a track it doesn't have, LRCLIB looks further afield and
// mostly gives up saying it is busy (2026-11-01): then it is searched for
// at once.
func (l *LRCLIB) lookup(ctx context.Context, q Query) (string, error) {
	var r record
	err := l.get(ctx, 1, "/api/get", url.Values{
		"track_name":  {q.Name},
		"artist_name": {q.Artist},
		"album_name":  {q.Album},
		"duration":    {fmt.Sprint(int(math.Round(q.Duration.Seconds())))},
	}, &r)
	switch {
	case err == nil && r.Instrumental:
		return "", nil
	case err == nil && r.SyncedLyrics != "":
		return r.SyncedLyrics, nil
	case err != nil && !errors.Is(err, errNotFound) && !errors.Is(err, errBusy):
		return "", err
	}
	names := []string{q.Name}
	if bare := bareName(q.Name); bare != q.Name {
		names = append(names, bare)
	}
	for _, name := range names {
		var found []record
		if err := l.get(ctx, tries, "/api/search", url.Values{"track_name": {name}, "artist_name": {q.Artist}}, &found); err != nil {
			return "", err
		}
		if r, ok := best(found, q, near); ok {
			return r.SyncedLyrics, nil
		}
	}
	// Of any artist, the same name and length is mostly the same song, and
	// otherwise a cover of it.
	var found []record
	if err := l.get(ctx, tries, "/api/search", url.Values{"track_name": {names[len(names)-1]}}, &found); err != nil {
		return "", err
	}
	if r, ok := best(found, q, nearest); ok {
		return r.SyncedLyrics, nil
	}
	return "", nil
}

// best is the one of found nearest q in length with timed lyrics, of q's
// album if there are several as near.
func best(found []record, q Query, near time.Duration) (record, bool) {
	picks := make([]pick, len(found))
	for i, r := range found {
		picks[i] = pick{
			Length:    time.Duration(r.Duration * float64(time.Second)),
			Album:     r.AlbumName,
			HasLyrics: r.SyncedLyrics != "" && !r.Instrumental,
		}
	}
	if i := nearestOf(picks, q, near); i >= 0 {
		return found[i], true
	}
	return record{}, false
}

// added is what Spotify adds to a name that LRCLIB's track names mostly
// leave out: " - Remastered 2011", " - Live", " (feat. Someone)".
var added = regexp.MustCompile(`(?i)\s+(-\s.*|\((feat|with|ft)\.?\s[^)]*\))$`)

func bareName(name string) string {
	return strings.TrimSpace(added.ReplaceAllString(name, ""))
}

var (
	errNotFound = errors.New("lyrics: not on LRCLIB")
	errBusy     = errors.New("lyrics: LRCLIB is busy")
	errRefused  = errors.New("lyrics: LRCLIB refused the query")
)

// get asks LRCLIB at path, decoding its answer into v, up to tries times
// while it says it is too busy, a little later each time.
func (l *LRCLIB) get(ctx context.Context, tries int, path string, query url.Values, v any) error {
	base := l.Base
	if base == "" {
		base = "https://lrclib.net"
	}
	client := l.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	pause := l.pause
	if pause == 0 {
		pause = time.Second
	}
	var err error
	for try := range tries {
		if try > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pause * time.Duration(try)):
			}
		}
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, "GET", base+path+"?"+query.Encode(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", userAgent)
		var res *http.Response
		res, err = client.Do(req)
		if err != nil {
			continue
		}
		switch {
		case res.StatusCode == http.StatusNotFound:
			res.Body.Close()
			return errNotFound
		case res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests:
			res.Body.Close()
			err = fmt.Errorf("%w: %s", errBusy, res.Status)
			continue
		case res.StatusCode == http.StatusBadRequest:
			res.Body.Close()
			return errRefused
		case res.StatusCode != http.StatusOK:
			res.Body.Close()
			return fmt.Errorf("lyrics: LRCLIB: %s", res.Status)
		}
		err = json.NewDecoder(res.Body).Decode(v)
		res.Body.Close()
		return err
	}
	return err
}
