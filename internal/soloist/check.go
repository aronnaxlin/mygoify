package soloist

import (
	"context"
	"errors"
)

// ErrNotPaired is what CheckKey returns when Soloist's data holds no
// pairing to try the key on, as before the first: the key shows itself
// wrong only as pairing fails.
var ErrNotPaired = errors.New("soloist: not paired yet")

// CheckKey tries key with Soloist's executable bin on a copy of dataDir,
// and returns nil once Soloist logs in with it, within 30 seconds. A
// wrong key makes Soloist delete the pairing in its data (DESIGN.md
// §4.2): a key not checked so never meets the real one.
func CheckKey(ctx context.Context, bin, key, dataDir string) error {
	return errors.New("soloist: checking a key is not done yet")
}
