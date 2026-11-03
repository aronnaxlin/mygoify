package locales

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
)

// TestSupportedLanguages checks that the app offers the four languages, with
// English first so that unsupported system languages fall back to English.
func TestSupportedLanguages(t *testing.T) {
	langs := mygo.App.Languages()
	if len(langs) != 4 || langs[0] != "en" {
		t.Fatalf("unexpected app languages: %v", langs)
	}

	for _, tc := range []struct {
		system []string
		want   string
	}{
		{[]string{"zh-CN"}, "zh-Hans"},
		{[]string{"zh-TW"}, "zh-Hant"},
		{[]string{"ja-JP"}, "ja"},
		{[]string{"en-US"}, "en"},
		{[]string{"fr-FR"}, "en"},
		{[]string{"de-DE"}, "en"},
	} {
		match := i18n.Match(tc.system, langs)
		got := match
		if got == "" {
			got = langs[0] // fallback
		}
		if got != tc.want {
			t.Errorf("system %v: got %q, want %q", tc.system, got, tc.want)
		}
	}
}

// TestCatalogIn checks that messages can be accessed directly in each of the
// four languages through In(locale).
func TestCatalogIn(t *testing.T) {
	cases := []struct {
		locale    string
		cancel    string
		playlists string
	}{
		{"zh-Hans", "取消", "我的歌单"},
		{"zh-Hant", "取消", "我的播放清單"},
		{"en", "Cancel", "My Playlists"},
		{"ja", "キャンセル", "マイプレイリスト"},
	}

	for _, tc := range cases {
		msgs := In(tc.locale)
		if got := msgs.Cancel(); got != tc.cancel {
			t.Errorf("[%s] Cancel = %q, want %q", tc.locale, got, tc.cancel)
		}
		if got := msgs.MyPlaylists(); got != tc.playlists {
			t.Errorf("[%s] MyPlaylists = %q, want %q", tc.locale, got, tc.playlists)
		}
	}
}
