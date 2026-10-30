package library

import (
	"bytes"
	"errors"
	"image/png"
	"testing"

	"mygoify/internal/music"
)

// A search pages through what matches, saying whether more follows, as
// Spotify's does with its total gone.
func TestFakeSearchPages(t *testing.T) {
	f := NewFake()
	var all []music.Track
	for offset := 0; ; offset += 3 {
		res, err := f.Search("夜航", music.SearchTracks, 3, offset)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, res.Tracks...)
		if !res.More {
			break
		}
	}
	if len(all) != 11 {
		t.Fatalf("found %d of 夜航电台's 11 songs", len(all))
	}
	seen := map[string]bool{}
	for _, tr := range all {
		if seen[tr.ID] {
			t.Fatalf("%s found twice", tr.Name)
		}
		seen[tr.ID] = true
	}
}

// Liking a song puts it first among the liked songs, which show as a
// playlist; unliking takes it out.
func TestFakeLike(t *testing.T) {
	f := NewFake()
	tr := f.allTracks()[0]
	f.Like(tr.ID, false)
	if err := f.Like(tr.ID, true); err != nil {
		t.Fatal(err)
	}
	p, _ := f.Playlist("liked")
	if !p.Liked || len(p.Tracks) == 0 || p.Tracks[0].ID != tr.ID {
		t.Fatalf("liked songs: %+v", p)
	}
	f.Like(tr.ID, false)
	ids, _ := f.LikedIDs()
	for _, id := range ids {
		if id == tr.ID {
			t.Fatal("still liked")
		}
	}
}

// A song added to a playlist again is refused, and the listener changes
// only their own playlists or those shared with them.
func TestFakePlaylistEdits(t *testing.T) {
	f := NewFake()
	p, _ := f.CreatePlaylist("新歌单")
	tr := f.allTracks()[2]
	if err := f.AddToPlaylist(p.ID, tr.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.AddToPlaylist(p.ID, tr.ID); !errors.Is(err, ErrInPlaylist) {
		t.Fatalf("added twice: %v", err)
	}
	got, _ := f.Playlist(p.ID)
	if got.TrackCount != 1 || got.Snapshot == p.Snapshot {
		t.Fatalf("after adding: %d tracks, snapshot %q", got.TrackCount, got.Snapshot)
	}
	_, saved, _ := f.MyPlaylists()
	var spotify *music.Playlist
	for i := range saved {
		if saved[i].Hidden {
			spotify = &saved[i]
		}
	}
	if spotify == nil {
		t.Fatal("no hidden playlist among the saved")
	}
	if err := f.RenamePlaylist(spotify.ID, "我的"); err == nil {
		t.Fatal("renamed Spotify's playlist")
	}
}

// Fake's pictures decode, at the size their URL asks for.
func TestFakeImage(t *testing.T) {
	url := fakeImages("album:迷跡波").URL(64)
	b, err := FakeImage(url)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if w := img.Bounds().Dx(); w != 64 {
		t.Fatalf("%d px across", w)
	}
}
