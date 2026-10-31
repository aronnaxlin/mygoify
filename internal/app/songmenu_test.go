package app

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygoify/internal/music"
	"mygoify/internal/playback"
)

func TestSongMenuCopies(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	tr := music.Track{ID: "1859245776", Name: "宝船", Artists: []music.Artist{{Name: "DOUDOU"}, {Name: "小明"}}, Duration: 200000}
	a.pl.Cue([]music.Track{tr})
	a.setLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	for _, c := range []struct{ item, want string }{
		{"复制歌名", "宝船"},
		{"复制歌名和歌手", "宝船 - DOUDOU, 小明"},
		{"复制链接", "https://open.spotify.com/track/1859245776"},
	} {
		if err := tt.Click("更多"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(c.item); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		tt.Frame()
		if copied != c.want {
			t.Errorf("%s copied %q, want %q", c.item, copied, c.want)
		}
	}
}

func TestSongMenuOnRightClick(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists["42"] = &music.Playlist{ID: "42", Name: "飙升榜", Tracks: []music.Track{
		{ID: "1", Name: "一张相片", Artists: []music.Artist{{Name: "林宥嘉"}}, Duration: 200000},
		{ID: "2", Name: "宝船", Artists: []music.Artist{{Name: "DOUDOU"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, id: "42"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if got, want := tt.Menu(), []string{"加入队列", "-", "复制歌名", "复制歌名和歌手", "-", "复制链接"}; !slices.Equal(got, want) {
		t.Fatalf("the menu shows %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem("复制链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if want := "https://open.spotify.com/track/2"; copied != want {
		t.Errorf("copied %q, want %q", copied, want)
	}
	if a.pl.Current() != nil {
		t.Error("the right click played the song")
	}
}

// queueBackend plays as the fake does, but notes what it was asked to
// queue, and fails it when told to.
type queueBackend struct {
	*playback.Fake
	queued []string
	fail   error
}

func (b *queueBackend) AddToQueue(t music.Track) error {
	b.queued = append(b.queued, t.ID)
	return b.fail
}

// The song menu queues the song and says so, and a queue that fails says
// that, rather than the song playing failing.
func TestSongMenuQueues(t *testing.T) {
	for _, fail := range []error{nil, errors.New("offline")} {
		a := newTestApp()
		a.settings = defaultSettings()
		b := &queueBackend{Fake: playback.NewFake(), fail: fail}
		posted := make(chan func(), 8)
		a.pl = playback.New(func(f func()) { posted <- f }, b)
		tr := music.Track{ID: "1859245776", Name: "宝船", Duration: 200000}
		a.pl.Cue([]music.Track{{ID: "1", Name: "一张相片", Duration: 200000}})
		a.playlists["42"] = &music.Playlist{ID: "42", Name: "飙升榜", Tracks: []music.Track{tr}}
		a.route = route{page: pagePlaylist, id: "42"}
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		if err := tt.RightClick("宝船"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("加入队列"); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		select {
		case f := <-posted:
			f()
		case <-time.After(2 * time.Second):
			t.Fatal("the backend was never asked")
		}
		if !slices.Equal(b.queued, []string{"1859245776"}) {
			t.Errorf("queued %v", b.queued)
		}
		want := "已加入队列"
		if fail != nil {
			want = "加入队列失败，请稍后再试"
		}
		if a.toast != want {
			t.Errorf("failing %v, toast %q, want %q", fail, a.toast, want)
		}
		if a.pl.Err() != "" {
			t.Errorf("failing %v, the song playing failed too: %q", fail, a.pl.Err())
		}
	}
}
