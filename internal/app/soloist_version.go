package app

import (
	"context"
	"time"

	"mygoify/internal/soloist"
)

// readSoloistVersion asks Soloist of itself, off the main thread: which
// build it is, and so when it expires (DESIGN.md §4.2).
func (a *app) readSoloistVersion() {
	bin := a.settings.Spotify.withEnv().SoloistBin
	if bin == "" {
		return
	}
	go func() {
		v, err := soloist.ReadVersion(context.Background(), expandHome(bin))
		// Without what it says, nothing of its expiry is known: Soloist
		// may not run at all, which the onboarding tells of.
		if err != nil {
			return
		}
		a.post(func() {
			a.soloistVer = v
			a.noteSoloistExpiry(time.Now())
		})
	}()
}

// noteSoloistExpiry has the listener update Soloist once its build is
// past its expiry, and says once, as the app starts, that it is near.
func (a *app) noteSoloistExpiry(now time.Time) {
	v := a.soloistVer
	switch {
	case v.Build.IsZero():
		return
	case now.After(v.Expires()):
		// Soloist exits with 10 and stays stopped: updating it, alone, is
		// what is left to do.
		a.look()
		a.ob.binErr = "Soloist 已于 " + dateText(v.Expires()) + " 过期，请更新"
		a.ob.open = true
		a.goTo(stepSoloist)
		a.redraw()
	case v.Expiring(now):
		a.toast = "Soloist 将在 " + dateText(v.Expires()) + " 过期，请到设置里更新"
	}
}

// soloistAbout is what Soloist is and when its version expires, for the
// foot of the account's card, or "" while the version isn't known.
func (a *app) soloistAbout() string {
	v := a.soloistVer
	if v.Build.IsZero() {
		return ""
	}
	about := "Soloist"
	if v.Number != "" {
		about += " " + v.Number
	}
	return about + "，到期日 " + dateText(v.Expires()) + "。"
}
