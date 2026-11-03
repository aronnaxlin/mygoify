package locales

import "github.com/egoist/mygo"

func init() {
	// The app's supported languages, with English first so that system
	// languages other than the four fall back to English.
	mygo.App.SetLanguages("en", "zh-Hans", "zh-Hant", "ja")
}
