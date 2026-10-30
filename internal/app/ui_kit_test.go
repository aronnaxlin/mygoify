package app

import (
	"testing"
	"time"
)

func TestClock(t *testing.T) {
	if s := clock(185 * time.Second); s != "3:05" {
		t.Error(s)
	}
}
