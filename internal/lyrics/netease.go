package lyrics

import (
	"context"
	"crypto/aes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// NetEase finds lyrics on NetEase Cloud Music (music.163.com), which has
// word-timed lyrics (YRC) and translations for many Chinese and Japanese
// songs that LRCLIB lacks. The endpoints are the desktop client's, not a
// public API, and their lyrics are the users' (DESIGN.md §7): they may
// change, or answer differently elsewhere.
type NetEase struct {
	HTTP *http.Client
	// Base is where tracks are searched, https://music.163.com unless a
	// test sets it.
	Base string
	// Eapi is where the desktop client asks for lyrics,
	// https://interface3.music.163.com unless a test sets it.
	Eapi string
}

func (n *NetEase) Name() string { return "netease" }

// A neSong is a track NetEase found: its length is in milliseconds.
type neSong struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Duration float64 `json:"duration"`
	Album    struct {
		Name string `json:"name"`
	} `json:"album"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
}

// neLyrics is NetEase's answer for a track: its lines, their translation,
// and their word times, if any.
type neLyrics struct {
	Lrc struct {
		Lyric string `json:"lyric"`
	} `json:"lrc"`
	Tlyric struct {
		Lyric string `json:"lyric"`
	} `json:"tlyric"`
	Yrc struct {
		Lyric string `json:"lyric"`
	} `json:"yrc"`
	Nolyric     bool `json:"nolyric"`
	Uncollected bool `json:"uncollected"`
}

func (r neLyrics) found() Found {
	if r.Nolyric || r.Uncollected {
		return Found{}
	}
	return Found{LRC: r.Lrc.Lyric, Translation: r.Tlyric.Lyric, YRC: r.Yrc.Lyric}
}

// Find searches NetEase for q, as its name is and without what Spotify
// adds to it, and asks for the lyrics of the track of its album as long,
// or else the longest of its name.
func (n *NetEase) Find(ctx context.Context, q Query) (Found, error) {
	names := []string{q.Name}
	if bare := bareName(q.Name); bare != q.Name {
		names = append(names, bare)
	}
	for _, name := range names {
		songs, err := n.search(ctx, strings.TrimSpace(name+" "+q.Artist))
		if err != nil {
			return Found{}, err
		}
		picks := make([]pick, len(songs))
		for i, s := range songs {
			picks[i] = pick{
				Name:      s.Name,
				Length:    time.Duration(s.Duration * float64(time.Millisecond)),
				Album:     s.Album.Name,
				HasLyrics: true,
			}
		}
		if i := nearestOf(picks, q, near); i >= 0 {
			return n.lyrics(ctx, songs[i].ID)
		}
	}
	return Found{}, nil
}

func (n *NetEase) search(ctx context.Context, keyword string) ([]neSong, error) {
	var res struct {
		Code   int `json:"code"`
		Result struct {
			Songs []neSong `json:"songs"`
		} `json:"result"`
	}
	u := n.base() + "/api/search/get/web?" + url.Values{
		"csrf_token": {""},
		"s":          {keyword},
		"type":       {"1"},
		"offset":     {"0"},
		"total":      {"true"},
		"limit":      {"20"},
	}.Encode()
	if err := n.get(ctx, u, &res); err != nil {
		return nil, err
	}
	// Behind some addresses NetEase answers that it is cheating, rather
	// than finding anything.
	if res.Code != 200 {
		return nil, fmt.Errorf("lyrics: NetEase: search code %d", res.Code)
	}
	return res.Result.Songs, nil
}

// lyrics asks the desktop client's API for a track's lyrics, whose answer
// has them word-timed where NetEase has them so.
func (n *NetEase) lyrics(ctx context.Context, id int64) (Found, error) {
	path := "/api/song/lyric/v1"
	data := map[string]any{
		"id":         strconv.FormatInt(id, 10),
		"cp":         false,
		"lv":         0,
		"kv":         0,
		"tv":         0,
		"rv":         0,
		"yv":         0,
		"ytv":        0,
		"yrv":        0,
		"csrf_token": "",
		"header":     eapiHeader(),
	}
	params, err := eapiSign(path, data)
	if err != nil {
		return Found{}, err
	}
	var res neLyrics
	if err := n.post(ctx, n.eapi()+"/eapi/song/lyric/v1", url.Values{"params": {params}}, &res); err != nil {
		return Found{}, err
	}
	return res.found(), nil
}

func (n *NetEase) base() string {
	if n.Base == "" {
		return "https://music.163.com"
	}
	return strings.TrimRight(n.Base, "/")
}

func (n *NetEase) eapi() string {
	if n.Eapi == "" {
		return "https://interface3.music.163.com"
	}
	return strings.TrimRight(n.Eapi, "/")
}

func (n *NetEase) client() *http.Client {
	if n.HTTP == nil {
		return http.DefaultClient
	}
	return n.HTTP
}

func (n *NetEase) get(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	return n.do(req, v)
}

func (n *NetEase) post(ctx context.Context, u string, form url.Values, v any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return n.do(req, v)
}

func (n *NetEase) do(req *http.Request, v any) error {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://music.163.com/")
	res, err := n.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("lyrics: NetEase: %s", res.Status)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// eapiKey encrypts the requests of every desktop client, so it is no
// listener's secret.
const eapiKey = "e82ckenh8dichen8"

// eapiSign signs and encrypts data for the desktop client's API at path,
// as it does: the path and data with their MD5, encrypted with AES-128 in
// ECB mode.
//
// From Lyricify Lyrics Helper (WXRIW/Lyricify-Lyrics-Helper,
// Apache-2.0), EapiHelper.cs.
func eapiSign(path string, data any) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	sum := md5.Sum([]byte("nobody" + path + "use" + string(b) + "md5forencrypt"))
	plain := path + "-36cd479b6b5-" + string(b) + "-36cd479b6b5-" + hex.EncodeToString(sum[:])
	block, err := aes.NewCipher([]byte(eapiKey))
	if err != nil {
		return "", err
	}
	// Padded as PKCS #7 does, to whole blocks.
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	buf := make([]byte, len(plain)+pad)
	copy(buf, plain)
	for i := len(plain); i < len(buf); i++ {
		buf[i] = byte(pad)
	}
	for i := 0; i < len(buf); i += aes.BlockSize {
		block.Encrypt(buf[i:], buf[i:])
	}
	return strings.ToUpper(hex.EncodeToString(buf)), nil
}

// eapiHeader is what the desktop client sends of itself, as its requests
// say.
func eapiHeader() map[string]string {
	return map[string]string{
		"__csrf":      "",
		"appver":      "8.0.0",
		"buildver":    strconv.FormatInt(time.Now().Unix(), 10),
		"channel":     "",
		"deviceId":    "",
		"mobilename":  "",
		"resolution":  "1920x1080",
		"os":          "android",
		"osver":       "",
		"requestId":   strconv.FormatInt(time.Now().UnixMilli(), 10) + "_0001",
		"versioncode": "140",
		"MUSIC_U":     "",
	}
}
