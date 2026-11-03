package soloist

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Expiry is how long a build of Soloist lasts: 90 days after it was
// built it exits with 10, which neither launchd nor systemd starts again
// (DESIGN.md §4.2).
const Expiry = 90 * 24 * time.Hour

// Remind is how long before Expiry the app says so.
const Remind = 14 * 24 * time.Hour

// Version is what `soloist --version` says of itself, such as
//
//	soloist 1.3.10.31 build 1793361706 (20261030) (abcdef) (linux/x86_64)
//
// and so when it was built, and when it expires.
type Version struct {
	Number string
	Build  time.Time
}

// Expires is when this version stops working.
func (v Version) Expires() time.Time { return v.Build.Add(Expiry) }

// Expiring says whether v expires at now within Remind: the app says so
// once as it starts, and past Expiry has the listener update Soloist.
func (v Version) Expiring(now time.Time) bool {
	e := v.Expires()
	return now.Before(e) && !now.Before(e.Add(-Remind))
}

// built is the build's timestamp in what --version says.
var built = regexp.MustCompile(`\bbuild (\d{9,11})\b`)

// ParseVersion reads what `soloist --version` says.
func ParseVersion(out string) (Version, error) {
	m := built.FindStringSubmatch(out)
	if m == nil {
		return Version{}, errors.New("soloist: --version says nothing of its build")
	}
	secs, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return Version{}, err
	}
	v := Version{Build: time.Unix(secs, 0).UTC()}
	if f := strings.Fields(out); len(f) > 1 && strings.HasSuffix(f[0], "soloist") {
		v.Number = f[1]
	}
	return v, nil
}

// ReadVersion runs bin with --version, and reads what it says.
func ReadVersion(ctx context.Context, bin string) (Version, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return Version{}, err
	}
	return ParseVersion(string(out))
}
