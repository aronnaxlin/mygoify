//go:build !darwin && !linux

package levels

// Tap hears no other process here.
func Tap(pid int, m *Meter) (stop func(), err error) { return nil, ErrUnsupported }
