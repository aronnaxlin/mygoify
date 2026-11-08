package app

import (
	"net/url"
	"strings"
	"time"

	"mygoify/internal/music"
	"mygoify/locales"
)

// spotifyLink reads what a link to Spotify names, as its apps share it,
// https://open.spotify.com/playlist/ID?si=…, or as a URI, spotify:album:ID:
// its kind, as track, album, artist or playlist, and its ID.
func spotifyLink(s string) (kind, id string, ok bool) {
	s = strings.TrimSpace(s)
	if rest, found := strings.CutPrefix(s, "spotify:"); found {
		kind, id, ok = strings.Cut(rest, ":")
		if !ok || id == "" || strings.Contains(id, ":") {
			return "", "", false
		}
		return kind, id, true
	}
	u, err := url.Parse(s)
	if err != nil || u.Host != "open.spotify.com" {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Links of a language, as /intl-ja/album/ID.
	if len(parts) == 3 && strings.HasPrefix(parts[0], "intl-") {
		parts = parts[1:]
	}
	if len(parts) != 2 || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// openLink opens what a link to Spotify, typed in the search field, names,
// and reports whether it was one it opens: an album's, an artist's or a
// playlist's page. A playlist Spotify made for the listener, which the Web
// API won't read, it keeps among the mixes the home page shows, and plays
// (DESIGN.md §5.4).
func (a *app) openLink(s string) bool {
	kind, id, ok := spotifyLink(s)
	if !ok {
		return false
	}
	switch kind {
	case "album":
		a.openPage(route{page: pageAlbum, id: id})
	case "artist":
		a.openArtist(id)
	case "playlist":
		if uri := "spotify:playlist:" + id; music.IsMix(uri) {
			a.seeMix(uri, "", time.Now())
			a.pl.PlayQueue(uri, nil, 0)
			a.toast = locales.MixKept()
			return true
		}
		a.openPage(route{page: pagePlaylist, id: id})
	default:
		return false
	}
	return true
}
