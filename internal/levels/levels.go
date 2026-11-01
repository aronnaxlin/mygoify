// Package levels measures the spectrum of what Soloist plays, for the
// visualizers, from its sound as the system mixes it (DESIGN.md §4.5):
// Soloist gives none of its own. Only the last moment of it is kept, in
// memory, as it is measured; none is written or sent anywhere.
package levels

import (
	"errors"
	"math"
	"math/cmplx"
	"sync"
	"time"
)

const (
	// Bands is how many bands Levels splits what plays into.
	Bands   = 24
	fftSize = 1024
	// ringSize is how many frames of sound Meter keeps: enough to look as
	// far back as a Bluetooth output lags, a third of a second at 96 kHz.
	ringSize = 1 << 15
)

// ErrUnsupported is Tap's error where it can't hear a process: before
// macOS 14.2, on Linux without PipeWire's tools, and on other systems.
var ErrUnsupported = errors.New("levels: can't hear another process here")

// Meter takes in sound as it is mixed and measures the spectrum of what
// is heard now. Feed runs on the audio thread, Levels on the main one.
type Meter struct {
	mu      sync.Mutex
	ring    [ringSize]float32 // mono
	written int64             // frames fed, all told
	fedAt   time.Time
	soundAt time.Time // the last that wasn't silence
	rate    int
	delay   int64 // frames between being mixed and being heard

	// What only Levels touches.
	amu    sync.Mutex
	edges  [Bands + 1]int
	edgeOf int // the rate edges were found for
	window [fftSize]float32
	buf    [fftSize]complex128
	end    int64 // where last was measured to
	last   [Bands]float32
	ref    float64 // the loudest band lately, in dB
	refAt  time.Time

	now func() time.Time
}

const (
	// stale is how long Levels shows what it last heard once no more
	// comes: the system stops mixing a process that is paused.
	stale = 150 * time.Millisecond
	// hush is how long Levels hears silence before it says there is
	// none: so it does with no permission to record the system's audio.
	hush = 2 * time.Second
	// span is how many dB below the loudest band lately a band shows
	// empty; bands follow how loud it is lately, as Soloist's volume and
	// its loudness normalization turn down what the system hears.
	span = 46.0
	// fall is how many dB a second the loudest band lately falls by.
	fall = 4.0
	// floor is how low that goes, so silence doesn't fill the bands.
	floor = -70.0
)

func NewMeter() *Meter {
	m := &Meter{now: time.Now, ref: floor, end: -1}
	for i := range m.window {
		m.window[i] = float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(fftSize-1)))
	}
	return m
}

// SetFormat says the sample rate of what Feed takes, and how long the
// system takes from mixing it to playing it.
func (m *Meter) SetFormat(rate int, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rate = rate
	m.delay = min(ringSize-fftSize, int64(latency.Seconds()*float64(rate)))
}

// Feed takes interleaved samples of channels channels.
func (m *Meter) Feed(samples []float32, channels int) {
	if channels < 1 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	loud := false
	for i := 0; i+channels <= len(samples); i += channels {
		var v float32
		for _, s := range samples[i : i+channels] {
			v += s
		}
		v /= float32(channels)
		loud = loud || v > 1e-5 || v < -1e-5
		m.ring[m.written%ringSize] = v
		m.written++
	}
	m.fedAt = m.now()
	if loud {
		m.soundAt = m.fedAt
	}
}

// Levels returns the spectrum of what is heard now, each band from 0 to
// 1, and false when nothing is.
func (m *Meter) Levels() ([Bands]float32, bool) {
	m.amu.Lock()
	defer m.amu.Unlock()
	m.mu.Lock()
	now := m.now()
	end, rate := m.written-m.delay, m.rate
	if rate == 0 || end < fftSize || now.Sub(m.fedAt) > stale || now.Sub(m.soundAt) > hush {
		m.mu.Unlock()
		return [Bands]float32{}, false
	}
	if end == m.end {
		m.mu.Unlock()
		return m.last, true
	}
	for i := range fftSize {
		v := m.ring[(end-fftSize+int64(i))%ringSize]
		m.buf[i] = complex(float64(v*m.window[i]), 0)
	}
	m.mu.Unlock()

	if m.edgeOf != rate {
		m.findEdges(rate)
	}
	fft(m.buf[:])
	var db [Bands]float64
	peak := math.Inf(-1)
	for b := range Bands {
		var sum float64
		for k := m.edges[b]; k < m.edges[b+1]; k++ {
			sum += cmplx.Abs(m.buf[k])
		}
		mag := sum / float64(m.edges[b+1]-m.edges[b]) / (fftSize / 4)
		// Higher bands carry less energy: tilt them up, as the ear hears.
		mag *= 1 + float64(b)/6
		db[b] = 20 * math.Log10(mag+1e-9)
		peak = max(peak, db[b])
	}
	if !m.refAt.IsZero() {
		m.ref -= fall * now.Sub(m.refAt).Seconds()
	}
	m.ref = max(m.ref, peak, floor)
	m.refAt = now
	for b := range Bands {
		m.last[b] = float32(math.Max(0, math.Min(1, (db[b]-m.ref+span-2)/span)))
	}
	m.end = end
	return m.last, true
}

// findEdges spaces the bands evenly in pitch, from 40 Hz to 14 kHz.
func (m *Meter) findEdges(rate int) {
	lo, hi := math.Log(40), math.Log(14000)
	for i := range m.edges {
		f := math.Exp(lo + (hi-lo)*float64(i)/Bands)
		m.edges[i] = max(1, min(fftSize/2-1, int(f*fftSize/float64(rate))))
	}
	for i := 1; i < len(m.edges); i++ {
		m.edges[i] = max(m.edges[i], m.edges[i-1]+1)
	}
	m.edgeOf = rate
}

// fft transforms x in place; len(x) is a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := range size / 2 {
				a, b := x[start+k], x[start+k+size/2]*wk
				x[start+k], x[start+k+size/2] = a+b, a-b
				wk *= w
			}
		}
	}
}
