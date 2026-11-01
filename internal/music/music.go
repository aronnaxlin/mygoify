// Package music holds what the app shows of music: tracks, albums,
// artists, playlists and the people who listen, as Spotify describes them,
// whatever answers for them (package library).
package music

import (
	"strings"
	"time"
)

// Image is a picture Spotify serves at one size.
type Image struct {
	URL           string
	Width, Height int
}

// LikedCover is the URL of the cover of the liked songs, which Spotify
// gives none: the app draws it.
const LikedCover = "mygoify:liked"

// Images are the sizes Spotify serves a picture at: a cover's are 640, 300
// and 64 pixels across.
type Images []Image

// URL returns the smallest picture at least px across, else the largest,
// or "" when there are none. A picture of unknown size, as a playlist's
// mosaic, counts as large enough.
func (im Images) URL(px int) string {
	best := -1
	for i, m := range im {
		switch {
		case best == -1:
			best = i
		case m.Width == 0 || im[best].Width == 0:
			// Unknown sizes: keep the first.
		case im[best].Width < px && m.Width > im[best].Width:
			best = i
		case m.Width >= px && m.Width < im[best].Width:
			best = i
		}
	}
	if best == -1 {
		return ""
	}
	return im[best].URL
}

type Artist struct {
	ID   string
	Name string
}

type Album struct {
	ID     string
	Name   string
	Images Images
}

type Track struct {
	ID       string
	Name     string
	Artists  []Artist
	Album    Album
	Duration int64 // milliseconds
	Explicit bool
	// Unplayable is set when Spotify won't play the track to the listener,
	// as in a market it isn't in.
	Unplayable bool
}

func (t *Track) URI() string { return "spotify:track:" + t.ID }

// URL is the track's page on Spotify's website.
func (t *Track) URL() string { return "https://open.spotify.com/track/" + t.ID }

// ArtistNames joins the track's artists as the credits read.
func (t *Track) ArtistNames() string { return names(t.Artists) }

func (t *Track) Playable() bool { return !t.Unplayable }

func (t *Track) Length() time.Duration { return time.Duration(t.Duration) * time.Millisecond }

func names(artists []Artist) string {
	s := make([]string, len(artists))
	for i, a := range artists {
		s[i] = a.Name
	}
	return strings.Join(s, ", ")
}

// User is someone on Spotify: the listener, or who owns a playlist.
type User struct {
	ID     string
	Name   string
	Images Images
}

type Playlist struct {
	ID          string
	Name        string
	Description string
	Images      Images
	Owner       User
	TrackCount  int
	Public      bool
	// Collaborative playlists are changed by those they are shared with,
	// besides their owner.
	Collaborative bool
	// Snapshot names the playlist's version: it changes as it does.
	Snapshot string
	// Liked is the listener's liked songs, shown as a playlist.
	Liked bool
	// Hidden is set when Spotify won't list the playlist's tracks: in
	// Development Mode, those of playlists the listener neither owns nor
	// collaborates on. It still plays, as a whole.
	Hidden bool
	Tracks []Track
}

// URI names the playlist to play it whole, or is "" for the liked songs,
// which the Web API plays not as a context: as a list of their tracks.
func (p *Playlist) URI() string {
	if p.Liked {
		return ""
	}
	return "spotify:playlist:" + p.ID
}

func (p *Playlist) URL() string { return "https://open.spotify.com/playlist/" + p.ID }

type AlbumProfile struct {
	Album
	Artists []Artist
	// Released is when it came out, as precisely as Spotify knows: a year,
	// a month or a day ("2023", "2023-06" or "2023-06-14").
	Released   string
	TrackCount int
	Label      string
	Songs      []Track
}

func (al *AlbumProfile) URI() string         { return "spotify:album:" + al.ID }
func (al *AlbumProfile) URL() string         { return "https://open.spotify.com/album/" + al.ID }
func (al *AlbumProfile) ArtistNames() string { return names(al.Artists) }

// Year is the year it came out, or "".
func (al *AlbumProfile) Year() string {
	if len(al.Released) < 4 {
		return ""
	}
	return al.Released[:4]
}

type ArtistProfile struct {
	Artist
	Images Images
	Genres []string
	// Followed is set when the listener follows them.
	Followed bool
	// Songs are their most popular songs. Spotify gives no list of them in
	// Development Mode: a search for the artist's songs stands in.
	Songs []Track
}

func (ar *ArtistProfile) URI() string { return "spotify:artist:" + ar.ID }
func (ar *ArtistProfile) URL() string { return "https://open.spotify.com/artist/" + ar.ID }

// SearchType is what a search looks for.
type SearchType int

const (
	SearchTracks SearchType = iota
	SearchArtists
	SearchAlbums
	SearchPlaylists
)

// SearchResult is a page of what a search found, of the one type it
// looked for. Spotify says how many it found in all as 0, so More says
// whether another page follows.
type SearchResult struct {
	Tracks    []Track
	Artists   []ArtistProfile
	Albums    []AlbumProfile
	Playlists []Playlist
	More      bool
	// Skipped is how many Spotify found that the page leaves out, as
	// tracks that won't play and playlists taken down: the next page
	// starts past them too.
	Skipped int
}

// Len is how many the page has.
func (r *SearchResult) Len() int {
	return len(r.Tracks) + len(r.Artists) + len(r.Albums) + len(r.Playlists)
}

// Append adds the next page to r.
func (r *SearchResult) Append(next SearchResult) {
	r.Tracks = append(r.Tracks, next.Tracks...)
	r.Artists = append(r.Artists, next.Artists...)
	r.Albums = append(r.Albums, next.Albums...)
	r.Playlists = append(r.Playlists, next.Playlists...)
	r.Skipped += next.Skipped
	r.More = next.More
}

// Home is what the home page shows: what the listener played lately and
// listens to most. Spotify browses no new releases or categories in
// Development Mode.
type Home struct {
	Recent     []Track
	TopTracks  []Track
	TopArtists []ArtistProfile
}
