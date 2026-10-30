package app

import (
	"mygoify/internal/library"
	"mygoify/internal/music"
	"mygoify/internal/playback"
)

func newTestApp() *app {
	a := &app{
		playlists: map[string]*music.Playlist{}, loadingPl: map[string]bool{}, plErr: map[string]string{},
		artists: map[string]*artistData{}, albums: map[string]*albumData{}, searches: map[route]*searchData{},
		lyricAt: -1, queueAt: -1,
		lib: &stubLib{Fake: library.NewFake()},
	}
	a.art = newArtCache(func(func()) {})
	a.pl = playback.New(func(func()) {}, playback.NewFake())
	a.pl.SetVolume(0.5)
	return a
}

// stubLib answers as the fake library does, but for what a test sets
// here, to see what the app asks or to answer otherwise.
type stubLib struct {
	*library.Fake
	like   func(id string, like bool) error
	album  func(id string) (*music.AlbumProfile, error)
	artist func(id string) (*music.ArtistProfile, error)
	follow func(id string, follow bool) error
	search func(q string, kind music.SearchType, limit, offset int) (music.SearchResult, error)
	create func(name string) (*music.Playlist, error)
	rename func(id, name string) error
	add    func(id string, tracks ...string) error
	remove func(id string, tracks ...string) error
	delete func(id string) error
}

// stub returns the stub library of a test's app.
func stub(a *app) *stubLib { return a.lib.(*stubLib) }

func (s *stubLib) Like(id string, like bool) error {
	if s.like != nil {
		return s.like(id, like)
	}
	return s.Fake.Like(id, like)
}

func (s *stubLib) Album(id string) (*music.AlbumProfile, error) {
	if s.album != nil {
		return s.album(id)
	}
	return s.Fake.Album(id)
}

func (s *stubLib) Artist(id string) (*music.ArtistProfile, error) {
	if s.artist != nil {
		return s.artist(id)
	}
	return s.Fake.Artist(id)
}

func (s *stubLib) FollowArtist(id string, follow bool) error {
	if s.follow != nil {
		return s.follow(id, follow)
	}
	return s.Fake.FollowArtist(id, follow)
}

func (s *stubLib) Search(q string, kind music.SearchType, limit, offset int) (music.SearchResult, error) {
	if s.search != nil {
		return s.search(q, kind, limit, offset)
	}
	return s.Fake.Search(q, kind, limit, offset)
}

func (s *stubLib) CreatePlaylist(name string) (*music.Playlist, error) {
	if s.create != nil {
		return s.create(name)
	}
	return s.Fake.CreatePlaylist(name)
}

func (s *stubLib) RenamePlaylist(id, name string) error {
	if s.rename != nil {
		return s.rename(id, name)
	}
	return s.Fake.RenamePlaylist(id, name)
}

func (s *stubLib) AddToPlaylist(id string, tracks ...string) error {
	if s.add != nil {
		return s.add(id, tracks...)
	}
	return s.Fake.AddToPlaylist(id, tracks...)
}

func (s *stubLib) RemoveFromPlaylist(id string, tracks ...string) error {
	if s.remove != nil {
		return s.remove(id, tracks...)
	}
	return s.Fake.RemoveFromPlaylist(id, tracks...)
}

func (s *stubLib) DeletePlaylist(id string) error {
	if s.delete != nil {
		return s.delete(id)
	}
	return s.Fake.DeletePlaylist(id)
}

// searchFake finds tracks in the fake library, for the tests that look at
// covers: what it lacks, it stands in for with a track of its own.
func searchFake(q string, n int) ([]music.Track, error) {
	f := library.NewFake()
	res, err := f.Search(q, music.SearchTracks, n, 0)
	if err != nil || len(res.Tracks) > 0 {
		return res.Tracks, err
	}
	all, err := f.Search("", music.SearchTracks, 50, 0)
	if err != nil || len(all.Tracks) == 0 {
		return nil, err
	}
	return all.Tracks[len(q)%len(all.Tracks):][:1], nil
}
