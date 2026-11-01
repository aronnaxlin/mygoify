//go:build !darwin

package levels

// Tap hears no other process here yet: on Linux, it would read Soloist's
// stream from PipeWire.
func Tap(pid int, m *Meter) (stop func(), err error) { return nil, ErrUnsupported }
