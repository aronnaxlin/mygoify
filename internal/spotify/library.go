package spotify

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"mygoify/internal/library"
	"mygoify/internal/music"
)

// Library is the listener's music, as the Web API answers for it, within
// the limits of Development Mode (DESIGN.md §5.3).
type Library struct {
	API *Client
	// Cache, if set, is the directory where the library keeps what it
	// read (cache.go).
	Cache string

	mu sync.Mutex
	me *music.User
	// The liked songs, as read last, for a while: their playlist and the
	// hearts both read them.
	liked   *likedSongs
	likedAt time.Time
	// syncing is held while the liked songs are read: the hearts and
	// their playlist ask at once, and read them once.
	syncing sync.Mutex
}

var _ library.Library = (*Library)(nil)

// call bounds each call to the Web API in time: the app waits on it.
func (l *Library) do(method, path string, in, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return l.API.Do(ctx, method, path, in, out)
}

// The objects of the Web API, as far as the app reads them.

type image struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func images(im []image) music.Images {
	out := make(music.Images, 0, len(im))
	for _, m := range im {
		out = append(out, music.Image{URL: m.URL, Width: m.Width, Height: m.Height})
	}
	return out
}

type artist struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Images []image  `json:"images"`
	Genres []string `json:"genres"`
}

type album struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Images      []image  `json:"images"`
	Artists     []artist `json:"artists"`
	ReleaseDate string   `json:"release_date"`
	TotalTracks int      `json:"total_tracks"`
	Label       string   `json:"label"`
	Tracks      *page[track]
}

type track struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"`
	Name       string   `json:"name"`
	Artists    []artist `json:"artists"`
	Album      *album   `json:"album"`
	DurationMs int64    `json:"duration_ms"`
	Explicit   bool     `json:"explicit"`
	IsPlayable *bool    `json:"is_playable"`
	IsLocal    bool     `json:"is_local"`
}

type user struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"display_name"`
	Images      []image `json:"images"`
}

type playlist struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Images        []image `json:"images"`
	Owner         user    `json:"owner"`
	Public        bool    `json:"public"`
	Collaborative bool    `json:"collaborative"`
	SnapshotID    string  `json:"snapshot_id"`
	// Items says how many it has, and is absent from playlists the
	// listener neither owns nor collaborates on. Tracks is its old name.
	Items  *struct{ Total int } `json:"items"`
	Tracks *struct{ Total int } `json:"tracks"`
}

type page[T any] struct {
	Items []T    `json:"items"`
	Next  string `json:"next"`
	Total int    `json:"total"`
}

func artists(as []artist) []music.Artist {
	out := make([]music.Artist, len(as))
	for i, a := range as {
		out[i] = music.Artist{ID: a.ID, Name: a.Name}
	}
	return out
}

// music returns what the app shows of t, on al when t, as an album's,
// names none.
func (t *track) music(al *album) music.Track {
	mt := music.Track{ID: t.ID, Name: t.Name, Artists: artists(t.Artists), Duration: t.DurationMs, Explicit: t.Explicit}
	if t.IsPlayable != nil && !*t.IsPlayable {
		mt.Unplayable = true
	}
	if t.Album != nil {
		al = t.Album
	}
	if al != nil {
		mt.Album = music.Album{ID: al.ID, Name: al.Name, Images: images(al.Images)}
	}
	return mt
}

// playable reports whether t is a track the app plays: not an episode,
// nor a local file, which Spotify streams not.
func (t *track) playable() bool {
	return t != nil && t.ID != "" && !t.IsLocal && (t.Type == "" || t.Type == "track")
}

func (a *album) profile() music.AlbumProfile {
	p := music.AlbumProfile{
		Album:      music.Album{ID: a.ID, Name: a.Name, Images: images(a.Images)},
		Artists:    artists(a.Artists),
		Released:   a.ReleaseDate,
		TrackCount: a.TotalTracks,
		Label:      a.Label,
	}
	if a.Tracks != nil {
		for i := range a.Tracks.Items {
			if t := &a.Tracks.Items[i]; t.playable() {
				p.Songs = append(p.Songs, t.music(a))
			}
		}
	}
	return p
}

func (p *playlist) music() music.Playlist {
	mp := music.Playlist{
		ID: p.ID, Name: p.Name, Description: p.Description, Images: images(p.Images),
		Owner:  music.User{ID: p.Owner.ID, Name: p.Owner.DisplayName},
		Public: p.Public, Collaborative: p.Collaborative, Snapshot: p.SnapshotID,
	}
	switch {
	case p.Items != nil:
		mp.TrackCount = p.Items.Total
	case p.Tracks != nil:
		mp.TrackCount = p.Tracks.Total
	}
	return mp
}

// pages calls path and the next of its pages until there are no more, or
// it has max of their items.
func pages[T any](l *Library, path string, max int) ([]T, error) {
	var out []T
	for path != "" && len(out) < max {
		var p page[T]
		if err := l.do("GET", path, nil, &p); err != nil {
			return out, err
		}
		out = append(out, p.Items...)
		path = strings.TrimPrefix(p.Next, BaseURL)
	}
	return out[:min(len(out), max)], nil
}

func (l *Library) Me() (*music.User, error) {
	l.mu.Lock()
	me := l.me
	l.mu.Unlock()
	if me != nil {
		c := *me
		return &c, nil
	}
	var u user
	if err := l.do("GET", "/me", nil, &u); err != nil {
		// Read before, while Spotify answered.
		kept, _, ok := load[music.User](l, "me")
		if !ok {
			return nil, err
		}
		me = &kept
	} else {
		me = &music.User{ID: u.ID, Name: u.DisplayName, Images: images(u.Images)}
		if me.Name == "" {
			me.Name = u.ID
		}
		l.store("me", me)
	}
	l.mu.Lock()
	l.me = me
	l.mu.Unlock()
	c := *me
	return &c, nil
}

// homeFresh is how long the home page read stays good: the app reads it
// again after as long (homeStale), and what was played lately changes.
const homeFresh = 5 * time.Minute

func (l *Library) Home() (music.Home, error) {
	name, err := l.mine("home")
	if err != nil {
		return music.Home{}, err
	}
	return cached(l, name, homeFresh, l.home)
}

func (l *Library) home() (music.Home, error) {
	var h music.Home
	var recent struct {
		Items []struct {
			Track track `json:"track"`
		} `json:"items"`
	}
	err := l.do("GET", "/me/player/recently-played?limit=30", nil, &recent)
	seen := map[string]bool{}
	for _, it := range recent.Items {
		if t := it.Track; t.playable() && !seen[t.ID] {
			seen[t.ID] = true
			h.Recent = append(h.Recent, t.music(nil))
		}
	}
	var top page[track]
	if e := l.do("GET", "/me/top/tracks?limit=10", nil, &top); e == nil {
		for i := range top.Items {
			if t := &top.Items[i]; t.playable() {
				h.TopTracks = append(h.TopTracks, t.music(nil))
			}
		}
	} else if err == nil {
		err = e
	}
	var topArtists page[artist]
	if e := l.do("GET", "/me/top/artists?limit=10", nil, &topArtists); e == nil {
		for _, a := range topArtists.Items {
			h.TopArtists = append(h.TopArtists, music.ArtistProfile{Artist: music.Artist{ID: a.ID, Name: a.Name}, Images: images(a.Images), Genres: a.Genres})
		}
	} else if err == nil {
		err = e
	}
	// What loaded shows: one part failing leaves the others.
	if len(h.Recent)+len(h.TopTracks)+len(h.TopArtists) > 0 {
		err = nil
	}
	return h, err
}

// likedID is the ID of the playlist the liked songs show as: Spotify
// lists them apart from the playlists.
const likedID = "liked"

// maxLiked is how many liked songs the app reads, the latest liked first.
const maxLiked = 10000

// likedFresh is how long the liked songs read stay good.
const likedFresh = 2 * time.Minute

// likedWhole is how long the liked songs are only read since the latest
// read, before they are read whole again: songs unliked in another app,
// as many as were liked, go unnoticed until then.
const likedWhole = 24 * time.Hour

// likedSongs are the liked songs read, the latest liked first: those the
// app plays, and how many Spotify counted besides, as local files.
type likedSongs struct {
	Tracks  []music.Track
	Skipped int
	Whole   time.Time // when they were last read whole
}

func (s *likedSongs) count() int { return len(s.Tracks) + s.Skipped }

func (s *likedSongs) add(t *track) {
	if t.playable() {
		s.Tracks = append(s.Tracks, t.music(nil))
	} else {
		s.Skipped++
	}
}

type savedPage = page[struct {
	Track track `json:"track"`
}]

// likedTracks returns the liked songs, the latest liked first, as read
// last if lately. Kept from before, they are read only since then, which
// takes a call; when Spotify limits the app, they answer as they are.
func (l *Library) likedTracks() ([]music.Track, error) {
	l.syncing.Lock()
	defer l.syncing.Unlock()
	l.mu.Lock()
	have := l.liked
	if have != nil && time.Since(l.likedAt) < likedFresh {
		l.mu.Unlock()
		return have.Tracks, nil
	}
	l.mu.Unlock()
	name, err := l.mine("liked")
	if err != nil {
		return nil, err
	}
	if have == nil {
		if kept, _, ok := load[likedSongs](l, name); ok {
			have = &kept
		}
	}
	got, err := l.readLiked(have)
	if err != nil {
		if have != nil {
			return have.Tracks, nil
		}
		return nil, err
	}
	l.store(name, got)
	l.mu.Lock()
	l.liked, l.likedAt = got, time.Now()
	l.mu.Unlock()
	return got.Tracks, nil
}

// readLiked reads the songs liked since have, ahead of have, or all of
// them, when have is old or no longer adds up to what Spotify counts.
func (l *Library) readLiked(have *likedSongs) (*likedSongs, error) {
	var first savedPage
	if err := l.do("GET", "/me/tracks?limit=50", nil, &first); err != nil {
		return nil, err
	}
	if have != nil && time.Since(have.Whole) < likedWhole && len(have.Tracks) > 0 && first.Total <= maxLiked {
		got, ok, err := l.likedSince(have, first)
		if ok || err != nil {
			return got, err
		}
	}
	return l.readAllLiked(first)
}

// likedSince reads the songs liked since have, from the first page on,
// until the latest of have. Past a few pages, reading them all costs as
// much.
func (l *Library) likedSince(have *likedSongs, first savedPage) (*likedSongs, bool, error) {
	latest := have.Tracks[0].ID
	got := &likedSongs{Whole: have.Whole}
	p := first
	for range 4 {
		for i := range p.Items {
			t := &p.Items[i].Track
			if t.playable() && t.ID == latest {
				got.Tracks = append(got.Tracks, have.Tracks...)
				got.Skipped += have.Skipped
				return got, got.count() == first.Total, nil
			}
			got.add(t)
		}
		if p.Next == "" {
			break
		}
		next := strings.TrimPrefix(p.Next, BaseURL)
		p = savedPage{}
		if err := l.do("GET", next, nil, &p); err != nil {
			return nil, false, err
		}
	}
	return nil, false, nil
}

// readAllLiked reads all the liked songs. The first page says how many
// there are, and the others are read four at a time: a page of 50 takes
// a third of a second, and one after another 2000 songs took more than
// ten.
func (l *Library) readAllLiked(first savedPage) (*likedSongs, error) {
	total := min(first.Total, maxLiked)
	pages := make([]savedPage, (total+49)/50)
	if len(pages) > 0 {
		pages[0] = first
	}
	errs := make([]error, len(pages))
	var wg sync.WaitGroup
	next := make(chan int)
	for range 4 {
		wg.Go(func() {
			for i := range next {
				errs[i] = l.do("GET", fmt.Sprintf("/me/tracks?limit=50&offset=%d", i*50), nil, &pages[i])
			}
		})
	}
	for i := 1; i < len(pages); i++ {
		next <- i
	}
	close(next)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	got := &likedSongs{Whole: time.Now()}
	for _, p := range pages {
		for i := range p.Items {
			got.add(&p.Items[i].Track)
		}
	}
	return got, nil
}

// playlistsFresh is how long the listener's playlists read stay good.
// The app's own changes drop them at once; another app's show after this.
const playlistsFresh = 10 * time.Minute

type myPlaylists struct{ Made, Saved []music.Playlist }

func (l *Library) MyPlaylists() (made, saved []music.Playlist, err error) {
	name, err := l.mine("playlists")
	if err != nil {
		return nil, nil, err
	}
	m, err := cached(l, name, playlistsFresh, func() (m myPlaylists, err error) {
		m.Made, m.Saved, err = l.myPlaylists()
		return m, err
	})
	return m.Made, m.Saved, err
}

// playlistsChanged drops the listener's playlists read: one of them
// changed, as its count or its name.
func (l *Library) playlistsChanged() {
	if name, err := l.mine("playlists"); err == nil {
		l.forget(name)
	}
}

func (l *Library) myPlaylists() (made, saved []music.Playlist, err error) {
	me, err := l.Me()
	if err != nil {
		return nil, nil, err
	}
	var liked page[struct{}]
	if err := l.do("GET", "/me/tracks?limit=1", nil, &liked); err != nil {
		return nil, nil, err
	}
	made = append(made, music.Playlist{ID: likedID, Name: "喜欢的歌曲", Liked: true, Owner: *me, TrackCount: liked.Total, Images: music.Images{{URL: music.LikedCover}}})
	lists, err := pages[playlist](l, "/me/playlists?limit=50", 500)
	for i := range lists {
		p := lists[i].music()
		if p.Owner.ID == me.ID {
			made = append(made, p)
		} else {
			saved = append(saved, p)
		}
	}
	return made, saved, err
}

func (l *Library) Playlist(id string) (*music.Playlist, error) {
	if id == likedID {
		return l.likedPlaylist()
	}
	name, err := l.mine("playlist-" + id)
	if err != nil {
		return nil, err
	}
	kept, _, ok := load[music.Playlist](l, name)
	var p playlist
	if err := l.do("GET", "/playlists/"+url.PathEscape(id)+"?fields="+url.QueryEscape("id,name,description,images,owner(id,display_name),public,collaborative,snapshot_id,items(total),tracks(total)"), nil, &p); err != nil {
		if ok {
			return &kept, nil
		}
		return nil, err
	}
	mp := p.music()
	if p.Items == nil && p.Tracks == nil {
		// Someone else's: Spotify lists none of its tracks, in
		// Development Mode.
		mp.Hidden = true
		return &mp, nil
	}
	// Its tracks are as read last while its snapshot is: one call
	// rather than one a page.
	if ok && mp.Snapshot != "" && kept.Snapshot == mp.Snapshot {
		mp.Tracks = kept.Tracks
		return &mp, nil
	}
	items, err := pages[struct {
		Item  *track `json:"item"`
		Track *track `json:"track"`
	}](l, "/playlists/"+url.PathEscape(id)+"/items?limit=50", 2000)
	var e *Error
	if errors.As(err, &e) && e.Status == 403 {
		mp.Hidden = true
		return &mp, nil
	}
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		t := it.Item
		if t == nil {
			t = it.Track
		}
		if t.playable() {
			mp.Tracks = append(mp.Tracks, t.music(nil))
		}
	}
	l.store(name, mp)
	return &mp, nil
}

func (l *Library) likedPlaylist() (*music.Playlist, error) {
	me, err := l.Me()
	if err != nil {
		return nil, err
	}
	liked, err := l.likedTracks()
	if err != nil {
		return nil, err
	}
	p := &music.Playlist{ID: likedID, Name: "喜欢的歌曲", Liked: true, Owner: *me, Images: music.Images{{URL: music.LikedCover}}, Tracks: liked}
	p.TrackCount = len(p.Tracks)
	p.Snapshot = fmt.Sprint(len(p.Tracks))
	return p, nil
}

// albumFresh is how long an album read stays good: one rarely changes.
const albumFresh = 7 * 24 * time.Hour

func (l *Library) Album(id string) (*music.AlbumProfile, error) {
	name, err := l.mine("album-" + id)
	if err != nil {
		return nil, err
	}
	return cached(l, name, albumFresh, func() (*music.AlbumProfile, error) { return l.album(id) })
}

func (l *Library) album(id string) (*music.AlbumProfile, error) {
	var a album
	if err := l.do("GET", "/albums/"+url.PathEscape(id), nil, &a); err != nil {
		return nil, err
	}
	// Past the first page of its tracks, the rest.
	if a.Tracks != nil && a.Tracks.Next != "" {
		rest, err := pages[track](l, strings.TrimPrefix(a.Tracks.Next, BaseURL), 500)
		if err != nil {
			return nil, err
		}
		a.Tracks.Items = append(a.Tracks.Items, rest...)
	}
	p := a.profile()
	return &p, nil
}

// topSongs is how many of an artist's songs their page lists.
const topSongs = 10

// Artist returns an artist with the songs a search for them finds first:
// Spotify lists no top tracks in Development Mode.
func (l *Library) Artist(id string) (*music.ArtistProfile, error) {
	var a artist
	if err := l.do("GET", "/artists/"+url.PathEscape(id), nil, &a); err != nil {
		return nil, err
	}
	p := &music.ArtistProfile{Artist: music.Artist{ID: a.ID, Name: a.Name}, Images: images(a.Images), Genres: a.Genres}
	res, err := l.Search(`artist:"`+a.Name+`"`, music.SearchTracks, topSongs, 0)
	if err == nil {
		for _, t := range res.Tracks {
			if slices.ContainsFunc(t.Artists, func(x music.Artist) bool { return x.ID == id }) {
				p.Songs = append(p.Songs, t)
			}
		}
	}
	var follows []bool
	if l.do("GET", "/me/library/contains?uris="+url.QueryEscape(p.URI()), nil, &follows) == nil && len(follows) == 1 {
		p.Followed = follows[0]
	}
	return p, nil
}

// pageLimit is the most the app asks for of a page, as Spotify gives in
// Development Mode.
const pageLimit = 10

func (l *Library) ArtistAlbums(id string, limit, offset int) ([]music.AlbumProfile, bool, error) {
	var p page[album]
	path := fmt.Sprintf("/artists/%s/albums?include_groups=album,single&limit=%d&offset=%d", url.PathEscape(id), min(limit, pageLimit), offset)
	if err := l.do("GET", path, nil, &p); err != nil {
		return nil, false, err
	}
	out := make([]music.AlbumProfile, len(p.Items))
	for i := range p.Items {
		out[i] = p.Items[i].profile()
	}
	return out, p.Next != "", nil
}

var searchTypes = map[music.SearchType]string{
	music.SearchTracks: "track", music.SearchArtists: "artist", music.SearchAlbums: "album", music.SearchPlaylists: "playlist",
}

func (l *Library) Search(query string, kind music.SearchType, limit, offset int) (music.SearchResult, error) {
	q := url.Values{"q": {query}, "type": {searchTypes[kind]}, "limit": {fmt.Sprint(min(limit, pageLimit))}, "offset": {fmt.Sprint(offset)}}
	var res struct {
		Tracks    *page[track]     `json:"tracks"`
		Artists   *page[artist]    `json:"artists"`
		Albums    *page[album]     `json:"albums"`
		Playlists *page[*playlist] `json:"playlists"`
	}
	var out music.SearchResult
	if err := l.do("GET", "/search?"+q.Encode(), nil, &res); err != nil {
		return out, err
	}
	switch {
	case res.Tracks != nil:
		for i := range res.Tracks.Items {
			if t := &res.Tracks.Items[i]; t.playable() {
				out.Tracks = append(out.Tracks, t.music(nil))
			}
		}
		out.More = res.Tracks.Next != ""
	case res.Artists != nil:
		for _, a := range res.Artists.Items {
			out.Artists = append(out.Artists, music.ArtistProfile{Artist: music.Artist{ID: a.ID, Name: a.Name}, Images: images(a.Images), Genres: a.Genres})
		}
		out.More = res.Artists.Next != ""
	case res.Albums != nil:
		for i := range res.Albums.Items {
			out.Albums = append(out.Albums, res.Albums.Items[i].profile())
		}
		out.More = res.Albums.Next != ""
	case res.Playlists != nil:
		// A playlist taken down shows as null among them.
		for _, p := range res.Playlists.Items {
			if p != nil {
				out.Playlists = append(out.Playlists, p.music())
			}
		}
		out.More = res.Playlists.Next != ""
	}
	return out, nil
}

func (l *Library) LikedIDs() ([]string, error) {
	liked, err := l.likedTracks()
	ids := make([]string, 0, len(liked))
	for _, t := range liked {
		ids = append(ids, t.ID)
	}
	return ids, err
}

// keep saves uri in the listener's library, or takes it out.
func (l *Library) keep(uri string, keep bool) error {
	method := "PUT"
	if !keep {
		method = "DELETE"
	}
	return l.do(method, "/me/library?uris="+url.QueryEscape(uri), nil, nil)
}

func (l *Library) Like(trackID string, like bool) error {
	if err := l.keep("spotify:track:"+trackID, like); err != nil {
		return err
	}
	name, err := l.mine("liked")
	if err != nil {
		return nil
	}
	l.syncing.Lock()
	defer l.syncing.Unlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	if like || l.liked == nil {
		// Read next since the latest read, which finds it.
		l.likedAt = time.Time{}
		return nil
	}
	// Unliked, it leaves what was read, which stays good.
	got := *l.liked
	got.Tracks = slices.DeleteFunc(slices.Clone(got.Tracks), func(t music.Track) bool { return t.ID == trackID })
	l.liked = &got
	l.store(name, &got)
	return nil
}

func (l *Library) FollowArtist(id string, follow bool) error {
	return l.keep("spotify:artist:"+id, follow)
}

func (l *Library) CreatePlaylist(name string) (*music.Playlist, error) {
	var p playlist
	if err := l.do("POST", "/me/playlists", map[string]any{"name": name, "public": false}, &p); err != nil {
		return nil, err
	}
	l.playlistsChanged()
	mp := p.music()
	return &mp, nil
}

func (l *Library) RenamePlaylist(id, name string) error {
	return l.changed(l.do("PUT", "/playlists/"+url.PathEscape(id), map[string]any{"name": name}, nil))
}

// changed drops the listener's playlists read, unless err says nothing
// changed, and returns err.
func (l *Library) changed(err error) error {
	if err == nil {
		l.playlistsChanged()
	}
	return err
}

// AddToPlaylist adds tracks to a playlist, unless one is in it already:
// Spotify would add it again.
func (l *Library) AddToPlaylist(id string, trackIDs ...string) error {
	p, err := l.Playlist(id)
	if err != nil {
		return err
	}
	var uris []string
	for _, tid := range trackIDs {
		if slices.ContainsFunc(p.Tracks, func(t music.Track) bool { return t.ID == tid }) {
			return library.ErrInPlaylist
		}
		uris = append(uris, "spotify:track:"+tid)
	}
	return l.changed(l.do("POST", "/playlists/"+url.PathEscape(id)+"/items", map[string]any{"uris": uris}, nil))
}

func (l *Library) RemoveFromPlaylist(id string, trackIDs ...string) error {
	items := make([]map[string]string, len(trackIDs))
	for i, tid := range trackIDs {
		items[i] = map[string]string{"uri": "spotify:track:" + tid}
	}
	return l.changed(l.do("DELETE", "/playlists/"+url.PathEscape(id)+"/items", map[string]any{"items": items}, nil))
}

// DeletePlaylist takes the playlist out of the listener's library, as
// Spotify's apps delete one: it unfollows it.
func (l *Library) DeletePlaylist(id string) error {
	return l.changed(l.keep("spotify:playlist:"+id, false))
}
