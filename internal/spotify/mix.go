package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"mygoify/internal/music"
)

// OEmbedURL is Spotify's public oEmbed endpoint; tests replace it.
var OEmbedURL = "https://open.spotify.com/oembed"

// mixFresh is how long a mix's name and cover stay good: Spotify makes a
// new cover as it changes the mix (DESIGN.md §5.4).
const mixFresh = 24 * time.Hour

// Mix names a playlist Spotify made for the listener (music.IsMix), and
// finds its cover, through Spotify's oEmbed, which needs no token and
// costs the app none of its calls: the Web API won't read the playlist.
// Its tracks it leaves out; Soloist plays it whole.
func (l *Library) Mix(id string) (*music.Playlist, error) {
	return cached(l, "mixes/"+id, mixFresh, func() (*music.Playlist, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		q := url.Values{"url": {"https://open.spotify.com/playlist/" + id}}
		req, err := http.NewRequestWithContext(ctx, "GET", OEmbedURL+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		hc := http.DefaultClient
		if l.API != nil && l.API.HTTP != nil {
			hc = l.API.HTTP
		}
		res, err := hc.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("spotify: oEmbed of %s: %s", id, res.Status)
		}
		var e struct {
			Title     string `json:"title"`
			Thumbnail string `json:"thumbnail_url"`
		}
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			return nil, err
		}
		pl := &music.Playlist{ID: id, Name: e.Title, Hidden: true}
		if e.Thumbnail != "" {
			pl.Images = music.Images{{URL: e.Thumbnail}}
		}
		return pl, nil
	})
}
