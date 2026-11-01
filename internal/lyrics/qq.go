package lyrics

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// QQ finds lyrics on QQ Music (y.qq.com), which has timed lines and
// translations of many Chinese songs LRCLIB lacks. The endpoints are the
// web player's, not a public API (DESIGN.md §7). Its word-timed lyrics are
// encrypted, and are not used: what it finds is timed by line.
type QQ struct {
	HTTP *http.Client
	// Base is where tracks are searched, https://u6.y.qq.com unless a
	// test sets it.
	Base string
	// Lyric is where lyrics are asked of, https://c.y.qq.com unless a
	// test sets it.
	Lyric string
}

func (q *QQ) Name() string { return "qq" }

// qqSearch is the module and method searched with, as QQ names them:
// the web player's, without a login.
const qqSearch = "music.search.SearchCgiService.DoSearchForQQMusicMobile"

// qqHighlight is what QQ wraps the words searched for in, in its answers.
var qqHighlight = strings.NewReplacer("<em>", "", "</em>", "")

// A qqSong is a track QQ found: its length is in seconds.
type qqSong struct {
	Mid      string  `json:"mid"`
	Name     string  `json:"name"`
	Title    string  `json:"title"`
	Interval float64 `json:"interval"`
	Album    struct {
		Name string `json:"name"`
	} `json:"album"`
	Singer []struct {
		Name string `json:"name"`
	} `json:"singer"`
}

// Find searches QQ Music for q, as its name is and without what Spotify
// adds to it, and asks for the lyrics of the track of its album as long,
// or else the nearest of its name.
func (q *QQ) Find(ctx context.Context, query Query) (Found, error) {
	names := []string{query.Name}
	if bare := bareName(query.Name); bare != query.Name {
		names = append(names, bare)
	}
	for _, name := range names {
		songs, err := q.search(ctx, strings.TrimSpace(name+" "+query.Artist))
		if err != nil {
			return Found{}, err
		}
		picks := make([]pick, len(songs))
		for i, s := range songs {
			picks[i] = pick{
				Name:      qqHighlight.Replace(s.Name),
				Length:    time.Duration(s.Interval * float64(time.Second)),
				Album:     qqHighlight.Replace(s.Album.Name),
				HasLyrics: s.Mid != "",
			}
		}
		if i := nearestOf(picks, query, near); i >= 0 {
			return q.lyrics(ctx, songs[i].Mid)
		}
	}
	return Found{}, nil
}

func (q *QQ) search(ctx context.Context, keyword string) ([]qqSong, error) {
	body := map[string]any{
		"comm": map[string]string{"ct": "11", "v": "20080508", "tmeLoginType": "0", "tmeLoginMethod": "0"},
		qqSearch: map[string]any{
			"module": "music.search.SearchCgiService",
			"method": "DoSearchForQQMusicMobile",
			"param": map[string]any{
				"query":        keyword,
				"search_type":  0,
				"sin":          0,
				"ein":          30,
				"page_num":     1,
				"num_per_page": 10,
			},
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", q.base()+"/cgi-bin/musicu.fcg", strings.NewReader(string(b)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var res map[string]json.RawMessage
	if err := q.do(req, &res); err != nil {
		return nil, err
	}
	var answer struct {
		Code int `json:"code"`
		Data struct {
			Body struct {
				Songs []qqSong `json:"item_song"`
			} `json:"body"`
		} `json:"data"`
	}
	if b, ok := res[qqSearch]; !ok || json.Unmarshal(b, &answer) != nil || answer.Code != 0 {
		return nil, fmt.Errorf("lyrics: QQ: search answered %d", answer.Code)
	}
	return answer.Data.Body.Songs, nil
}

// lyrics asks QQ Music for a track's lyrics, which it serves in base64,
// and their translation too.
func (q *QQ) lyrics(ctx context.Context, mid string) (Found, error) {
	form := url.Values{
		"songmid":     {mid},
		"format":      {"json"},
		"g_tk":        {"5381"},
		"loginUin":    {"0"},
		"hostUin":     {"0"},
		"inCharset":   {"utf8"},
		"outCharset":  {"utf-8"},
		"notice":      {"0"},
		"platform":    {"yqq"},
		"needNewCode": {"0"},
		"pcachetime":  {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", q.lyricBase()+"/lyric/fcgi-bin/fcg_query_lyric_new.fcg", strings.NewReader(form.Encode()))
	if err != nil {
		return Found{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://c.y.qq.com/")
	var res struct {
		Retcode int    `json:"retcode"`
		Code    int    `json:"code"`
		Lyric   string `json:"lyric"`
		Trans   string `json:"trans"`
	}
	if err := q.do(req, &res); err != nil {
		return Found{}, err
	}
	// QQ answers -1901 for a track it has no lyrics of, as for songs it
	// may not serve: no lyrics, rather than not asked.
	if res.Retcode != 0 && res.Retcode != noLyricAnswer {
		return Found{}, fmt.Errorf("lyrics: QQ: lyric answered %d", res.Retcode)
	}
	return Found{LRC: qqText(res.Lyric), Translation: qqText(res.Trans)}, nil
}

// noLyricAnswer is what QQ Music answers for a track it has no lyrics of.
const noLyricAnswer = -1901

// qqText returns what QQ serves of lyrics in base64, its HTML entities
// written out, as its lyrics have "&#58;" for a colon.
func qqText(s string) string {
	if s == "" {
		return ""
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		s = string(b)
	}
	return html.UnescapeString(s)
}

func (q *QQ) base() string {
	if q.Base == "" {
		return "https://u6.y.qq.com"
	}
	return strings.TrimRight(q.Base, "/")
}

func (q *QQ) lyricBase() string {
	if q.Lyric == "" {
		return "https://c.y.qq.com"
	}
	return strings.TrimRight(q.Lyric, "/")
}

func (q *QQ) do(req *http.Request, v any) error {
	req.Header.Set("User-Agent", userAgent)
	client := q.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("lyrics: QQ: %s", res.Status)
	}
	return json.NewDecoder(res.Body).Decode(v)
}
