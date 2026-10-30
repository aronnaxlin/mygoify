package library

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"slices"
	"strings"
	"sync"
	"time"

	"mygoify/internal/music"
)

// Fake is a library of made-up music, kept in memory, for tests and for
// running the app without Spotify. What the listener changes, it keeps.
type Fake struct {
	// Delay is how long each answer takes, to show what loads as it does.
	Delay time.Duration

	mu        sync.Mutex
	me        music.User
	artists   map[string]*music.ArtistProfile
	albums    map[string]*music.AlbumProfile
	byArtist  map[string][]string // album IDs, newest first
	playlists map[string]*music.Playlist
	made      []string // the listener's playlists, newest first
	saved     []string
	liked     []string // track IDs, the latest liked first
	followed  map[string]bool
	recent    []music.Track
	nextID    int
}

// NewFake returns a library with a few artists, albums and playlists.
func NewFake() *Fake {
	f := &Fake{
		me:        music.User{ID: "listener", Name: "听歌的人", Images: fakeImages("user:listener")},
		artists:   map[string]*music.ArtistProfile{},
		albums:    map[string]*music.AlbumProfile{},
		byArtist:  map[string][]string{},
		playlists: map[string]*music.Playlist{},
		followed:  map[string]bool{},
	}
	for _, ar := range fakeCatalog {
		artist := music.Artist{ID: fakeID("artist", ar.name), Name: ar.name}
		f.artists[artist.ID] = &music.ArtistProfile{Artist: artist, Images: fakeImages("artist:" + ar.name), Genres: ar.genres}
		for _, al := range ar.albums {
			album := music.Album{ID: fakeID("album", al.name), Name: al.name, Images: fakeImages("album:" + al.name)}
			profile := &music.AlbumProfile{Album: album, Artists: []music.Artist{artist}, Released: al.released, Label: ar.name + " Records"}
			for i, name := range al.songs {
				profile.Songs = append(profile.Songs, music.Track{
					ID: fakeID("track", al.name+"/"+name), Name: name, Artists: []music.Artist{artist}, Album: album,
					Duration: int64(180000 + 13000*((i*7+len(name))%9)),
				})
			}
			profile.TrackCount = len(profile.Songs)
			f.albums[album.ID] = profile
			f.byArtist[artist.ID] = append(f.byArtist[artist.ID], album.ID)
		}
	}
	for id, albums := range f.byArtist {
		slices.SortFunc(albums, func(a, b string) int { return strings.Compare(f.albums[b].Released, f.albums[a].Released) })
		// Their most popular songs: the first of each album, then the
		// second, and on.
		var top []music.Track
		for i := 0; len(top) < 10; i++ {
			added := false
			for _, al := range albums {
				if songs := f.albums[al].Songs; i < len(songs) {
					top, added = append(top, songs[i]), true
				}
			}
			if !added {
				break
			}
		}
		f.artists[id].Songs = top[:min(len(top), 10)]
	}

	all := f.allTracks()
	pick := func(from, step, n int) []music.Track {
		var out []music.Track
		for i := 0; i < n; i++ {
			out = append(out, all[(from+i*step)%len(all)])
		}
		return out
	}
	for i, tr := range pick(1, 3, 18) {
		if i%2 == 0 {
			f.liked = append(f.liked, tr.ID)
		}
	}
	f.recent = pick(2, 5, 12)
	f.addPlaylist(true, music.Playlist{Name: "通勤路上", Description: "早上八点的地铁。", Tracks: pick(0, 2, 24)})
	f.addPlaylist(true, music.Playlist{Name: "睡前", Tracks: pick(5, 7, 12)})
	f.addPlaylist(false, music.Playlist{Name: "周末的咖啡店", Owner: music.User{ID: "friend", Name: "夏日来信"}, Collaborative: true, Tracks: pick(3, 4, 20)})
	// Spotify's own: in Development Mode it plays, but lists no tracks.
	f.addPlaylist(false, music.Playlist{Name: "Daily Mix 1", Description: "为你挑选的歌曲。", Owner: music.User{ID: "spotify", Name: "Spotify"}, TrackCount: 50, Hidden: true})
	return f
}

// allTracks lists every track, the artists' albums in order.
func (f *Fake) allTracks() []music.Track {
	var out []music.Track
	for _, ar := range fakeCatalog {
		for _, id := range f.byArtist[fakeID("artist", ar.name)] {
			out = append(out, f.albums[id].Songs...)
		}
	}
	return out
}

func (f *Fake) addPlaylist(mine bool, p music.Playlist) *music.Playlist {
	f.nextID++
	p.ID = fmt.Sprintf("fakepl%04d", f.nextID)
	if p.Owner.ID == "" {
		p.Owner = music.User{ID: f.me.ID, Name: f.me.Name}
	}
	if p.Images == nil {
		p.Images = fakeImages("playlist:" + p.Name)
	}
	if !p.Hidden {
		p.TrackCount = len(p.Tracks)
	}
	p.Public = mine
	p.Snapshot = "1"
	f.playlists[p.ID] = &p
	if mine {
		f.made = append([]string{p.ID}, f.made...)
	} else {
		f.saved = append(f.saved, p.ID)
	}
	return &p
}

func (f *Fake) wait() { time.Sleep(f.Delay) }

func (f *Fake) Me() (*music.User, error) {
	f.wait()
	me := f.me
	return &me, nil
}

func (f *Fake) Home() (music.Home, error) {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	h := music.Home{Recent: slices.Clone(f.recent)}
	for _, ar := range fakeCatalog {
		a := *f.artists[fakeID("artist", ar.name)]
		h.TopArtists = append(h.TopArtists, a)
		if len(a.Songs) > 0 {
			h.TopTracks = append(h.TopTracks, a.Songs[0])
		}
	}
	return h, nil
}

func (f *Fake) likedPlaylist() music.Playlist {
	p := music.Playlist{ID: "liked", Name: "喜欢的歌曲", Liked: true, Owner: f.me, Images: fakeImages("liked"), Snapshot: fmt.Sprint(len(f.liked))}
	for _, id := range f.liked {
		if tr, ok := f.track(id); ok {
			p.Tracks = append(p.Tracks, tr)
		}
	}
	p.TrackCount = len(p.Tracks)
	return p
}

func (f *Fake) track(id string) (music.Track, bool) {
	for _, al := range f.albums {
		for _, tr := range al.Songs {
			if tr.ID == id {
				return tr, true
			}
		}
	}
	return music.Track{}, false
}

func (f *Fake) MyPlaylists() (made, saved []music.Playlist, err error) {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	made = []music.Playlist{f.likedPlaylist()}
	for _, id := range f.made {
		p := *f.playlists[id]
		p.Tracks = nil
		made = append(made, p)
	}
	for _, id := range f.saved {
		p := *f.playlists[id]
		p.Tracks = nil
		saved = append(saved, p)
	}
	return made, saved, nil
}

var errNotFound = errors.New("library: not found")

func (f *Fake) Playlist(id string) (*music.Playlist, error) {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	if id == "liked" {
		p := f.likedPlaylist()
		return &p, nil
	}
	p, ok := f.playlists[id]
	if !ok {
		return nil, errNotFound
	}
	c := *p
	c.Tracks = slices.Clone(p.Tracks)
	return &c, nil
}

func (f *Fake) Album(id string) (*music.AlbumProfile, error) {
	f.wait()
	al, ok := f.albums[id]
	if !ok {
		return nil, errNotFound
	}
	c := *al
	return &c, nil
}

func (f *Fake) Artist(id string) (*music.ArtistProfile, error) {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	ar, ok := f.artists[id]
	if !ok {
		return nil, errNotFound
	}
	c := *ar
	c.Followed = f.followed[id]
	return &c, nil
}

func (f *Fake) ArtistAlbums(id string, limit, offset int) ([]music.AlbumProfile, bool, error) {
	f.wait()
	ids := f.byArtist[id]
	var out []music.AlbumProfile
	for _, al := range ids[min(offset, len(ids)):min(offset+limit, len(ids))] {
		c := *f.albums[al]
		c.Songs = nil
		out = append(out, c)
	}
	return out, offset+limit < len(ids), nil
}

// Search finds what has query in its name, or its artist's, ignoring case.
func (f *Fake) Search(query string, kind music.SearchType, limit, offset int) (music.SearchResult, error) {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	q := strings.ToLower(strings.TrimSpace(query))
	has := func(s ...string) bool {
		for _, x := range s {
			if strings.Contains(strings.ToLower(x), q) {
				return true
			}
		}
		return false
	}
	var res music.SearchResult
	n := 0
	take := func() bool {
		n++
		return n > offset && n <= offset+limit
	}
	switch kind {
	case music.SearchTracks:
		for _, tr := range f.allTracks() {
			if has(tr.Name, tr.ArtistNames(), tr.Album.Name) && take() {
				res.Tracks = append(res.Tracks, tr)
			}
		}
	case music.SearchArtists:
		for _, ar := range fakeCatalog {
			if a := f.artists[fakeID("artist", ar.name)]; has(a.Name) && take() {
				res.Artists = append(res.Artists, *a)
			}
		}
	case music.SearchAlbums:
		for _, ar := range fakeCatalog {
			for _, id := range f.byArtist[fakeID("artist", ar.name)] {
				if al := f.albums[id]; has(al.Name, al.ArtistNames()) && take() {
					c := *al
					c.Songs = nil
					res.Albums = append(res.Albums, c)
				}
			}
		}
	case music.SearchPlaylists:
		for _, id := range append(slices.Clone(f.made), f.saved...) {
			if p := f.playlists[id]; has(p.Name, p.Description) && take() {
				c := *p
				c.Tracks = nil
				res.Playlists = append(res.Playlists, c)
			}
		}
	}
	res.More = n > offset+limit
	return res, nil
}

func (f *Fake) LikedIDs() ([]string, error) {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.liked), nil
}

func (f *Fake) Like(trackID string, like bool) error {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.liked = slices.DeleteFunc(f.liked, func(id string) bool { return id == trackID })
	if like {
		f.liked = append([]string{trackID}, f.liked...)
	}
	return nil
}

func (f *Fake) FollowArtist(id string, follow bool) error {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.followed[id] = follow
	return nil
}

// own returns a playlist of the listener's, which they may change.
func (f *Fake) own(id string) (*music.Playlist, error) {
	p, ok := f.playlists[id]
	if !ok {
		return nil, errNotFound
	}
	if p.Owner.ID != f.me.ID && !p.Collaborative {
		return nil, errors.New("library: not the listener's playlist")
	}
	return p, nil
}

func (f *Fake) CreatePlaylist(name string) (*music.Playlist, error) {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	p := *f.addPlaylist(true, music.Playlist{Name: name})
	return &p, nil
}

func (f *Fake) RenamePlaylist(id, name string) error {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	p, err := f.own(id)
	if err != nil {
		return err
	}
	p.Name = name
	return nil
}

func (f *Fake) changed(p *music.Playlist) {
	p.TrackCount = len(p.Tracks)
	var n int
	fmt.Sscan(p.Snapshot, &n)
	p.Snapshot = fmt.Sprint(n + 1)
}

func (f *Fake) AddToPlaylist(id string, trackIDs ...string) error {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	p, err := f.own(id)
	if err != nil {
		return err
	}
	for _, tid := range trackIDs {
		if slices.ContainsFunc(p.Tracks, func(tr music.Track) bool { return tr.ID == tid }) {
			return ErrInPlaylist
		}
	}
	for _, tid := range trackIDs {
		if tr, ok := f.track(tid); ok {
			p.Tracks = append(p.Tracks, tr)
		}
	}
	f.changed(p)
	return nil
}

func (f *Fake) RemoveFromPlaylist(id string, trackIDs ...string) error {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	p, err := f.own(id)
	if err != nil {
		return err
	}
	p.Tracks = slices.DeleteFunc(p.Tracks, func(tr music.Track) bool { return slices.Contains(trackIDs, tr.ID) })
	f.changed(p)
	return nil
}

func (f *Fake) DeletePlaylist(id string) error {
	f.wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.own(id); err != nil {
		return err
	}
	delete(f.playlists, id)
	f.made = slices.DeleteFunc(f.made, func(m string) bool { return m == id })
	return nil
}

// fakeID makes a Spotify-like ID of 22 letters and digits from a name.
func fakeID(kind, name string) string {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	h := fnv.New64a()
	h.Write([]byte(kind + ":" + name))
	x := h.Sum64()
	b := make([]byte, 22)
	for i := range b {
		b[i] = alphabet[x%62]
		x = x*6364136223846793005 + 1442695040888963407
	}
	return string(b)
}

// FakeImageScheme starts the URLs of Fake's pictures, which FakeImage
// draws.
const FakeImageScheme = "fake:"

func fakeImages(name string) music.Images {
	var im music.Images
	for _, px := range []int{640, 300, 64} {
		im = append(im, music.Image{URL: fmt.Sprintf("%s%s?%d", FakeImageScheme, name, px), Width: px, Height: px})
	}
	return im
}

// FakeImage draws the picture at a URL of Fake's: a gradient between two
// colors the name picks, as a PNG of the size the URL asks for.
func FakeImage(url string) ([]byte, error) {
	name, size, ok := strings.Cut(strings.TrimPrefix(url, FakeImageScheme), "?")
	px := 300
	if ok {
		fmt.Sscan(size, &px)
	}
	px = min(max(px, 16), 640)
	h := fnv.New32a()
	h.Write([]byte(name))
	sum := h.Sum32()
	hue := float64(sum%360) / 360
	from, to := hsl(hue, 0.55, 0.42), hsl(hue+0.12, 0.6, 0.62)
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	for y := range px {
		for x := range px {
			t := float64(x+y) / float64(2*px)
			img.Set(x, y, color.RGBA{
				uint8(float64(from.R)*(1-t) + float64(to.R)*t),
				uint8(float64(from.G)*(1-t) + float64(to.G)*t),
				uint8(float64(from.B)*(1-t) + float64(to.B)*t),
				255,
			})
		}
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	return buf.Bytes(), err
}

func hsl(h, s, l float64) color.RGBA {
	h -= float64(int(h))
	f := func(n float64) uint8 {
		k := n + h*12
		for k >= 12 {
			k -= 12
		}
		a := s * min(l, 1-l)
		v := l - a*max(-1, min(k-3, 9-k, 1))
		return uint8(v * 255)
	}
	return color.RGBA{f(0), f(8), f(4), 255}
}

// fakeCatalog is the music Fake has: bands that aren't, though the first
// borrows the song titles of MyGO!!!!!, whose name the app's does.
var fakeCatalog = []struct {
	name   string
	genres []string
	albums []struct {
		name, released string
		songs          []string
	}
}{
	{"迷途之子", []string{"j-rock", "anime"}, []struct {
		name, released string
		songs          []string
	}{
		{"迷跡波", "2023-12-13", []string{"春日影", "迷星叫", "壱雫空", "碧天伴走", "音一会", "栞"}},
		{"跡暖空", "2025-06-18", []string{"潜在表明", "名無声", "回層浮", "砂寸奏", "処救生"}},
	}},
	{"夜航电台", []string{"city pop"}, []struct {
		name, released string
		songs          []string
	}{
		{"城市漫游", "2021-04-02", []string{"晚风", "第九区", "雨后的站台", "霓虹", "末班车", "海边公路", "凌晨四点"}},
		{"慢慢说", "2024-11-08", []string{"慢慢说", "旧唱片", "周末早餐", "通勤十分钟"}},
	}},
	{"Paper Lanterns", []string{"indie pop", "dream pop"}, []struct {
		name, released string
		songs          []string
	}{
		{"Low Tide", "2019-09-20", []string{"Low Tide", "Glasshouse", "Summer Static", "Northbound", "Orchard"}},
		{"Halflight", "2022-03-11", []string{"Halflight", "Paper Boats", "Kite Season", "Window Seat", "Afterglow", "Lanterns"}},
	}},
}
