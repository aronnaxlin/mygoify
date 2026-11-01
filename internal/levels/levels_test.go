package levels

import (
	"math"
	"testing"
	"time"
)

// meter is a Meter at 48 kHz on a clock the test moves.
func meter(latency time.Duration) (*Meter, *time.Time) {
	m := NewMeter()
	clock := time.Unix(1000, 0)
	m.now = func() time.Time { return clock }
	m.SetFormat(48000, latency)
	return m, &clock
}

// tone is n stereo frames of a sine of freq Hz at amplitude amp.
func tone(freq, amp float64, n int) []float32 {
	s := make([]float32, 2*n)
	for i := range n {
		v := float32(amp * math.Sin(2*math.Pi*freq*float64(i)/48000))
		s[2*i], s[2*i+1] = v, v
	}
	return s
}

func loudest(lv [Bands]float32) (int, float32) {
	at := 0
	for b, v := range lv {
		if v > lv[at] {
			at = b
		}
	}
	return at, lv[at]
}

// A tone lights the band its pitch falls in the most.
func TestToneLightsItsBand(t *testing.T) {
	m, _ := meter(0)
	m.Feed(tone(1000, 0.5, 4096), 2)
	lv, ok := m.Levels()
	if !ok {
		t.Fatal("nothing heard")
	}
	// 1 kHz, in bands evenly spaced in pitch from 40 Hz to 14 kHz.
	want := int(Bands * math.Log(1000.0/40) / math.Log(14000.0/40))
	if b, _ := loudest(lv); b != want {
		t.Fatalf("loudest band %d, want %d: %v", b, want, lv)
	}
}

// The bands rise as high for music turned down, as Soloist's volume
// turns down what the system hears.
func TestQuietShowsAsLoud(t *testing.T) {
	loud, _ := meter(0)
	loud.Feed(tone(440, 0.5, 4096), 2)
	quiet, _ := meter(0)
	quiet.Feed(tone(440, 0.005, 4096), 2)
	lv, _ := loud.Levels()
	qv, _ := quiet.Levels()
	_, l := loudest(lv)
	_, q := loudest(qv)
	if l < 0.8 || math.Abs(float64(l-q)) > 0.05 {
		t.Fatalf("loudest band %.2f loud and %.2f quiet", l, q)
	}
}

// Nothing shows once the system stops mixing Soloist, as it pauses, nor
// while it hears only silence, as without leave to record.
func TestNothingHeard(t *testing.T) {
	m, clock := meter(0)
	m.Feed(tone(440, 0.5, 4096), 2)
	*clock = clock.Add(stale + time.Millisecond)
	if _, ok := m.Levels(); ok {
		t.Fatal("levels after the sound stopped coming")
	}

	m, clock = meter(0)
	for range 30 {
		*clock = clock.Add(100 * time.Millisecond)
		m.Feed(make([]float32, 2*4800), 2)
	}
	if _, ok := m.Levels(); ok {
		t.Fatal("levels of silence alone")
	}
}

// The bands show the sound as it is heard, after the output's latency,
// not as it is mixed.
func TestLevelsWaitForTheOutput(t *testing.T) {
	m, _ := meter(100 * time.Millisecond) // 4800 frames
	m.Feed(tone(440, 0.5, 8192), 2)
	m.Feed(make([]float32, 2*2400), 2)
	lv, ok := m.Levels()
	if _, v := loudest(lv); !ok || v < 0.8 {
		t.Fatalf("the tone mixed 50ms ago shows %.2f, ok %v", v, ok)
	}
	m.Feed(make([]float32, 2*6000), 2)
	lv, _ = m.Levels()
	if _, v := loudest(lv); v > 0.1 {
		t.Fatalf("silence heard now shows %.2f", v)
	}
}
