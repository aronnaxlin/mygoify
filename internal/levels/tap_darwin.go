package levels

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// On macOS, a Core Audio process tap hears what one process plays, as
// the system mixes it, and a private aggregate device of the tap alone
// hands it over: since macOS 14.2, with the listener's leave to record
// the system's audio (NSAudioCaptureUsageDescription).

type propAddr struct{ sel, scope, elem uint32 }

func fourcc(s string) uint32 {
	return uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3])
}

var (
	glob   = fourcc("glob")
	output = fourcc("outp")
)

var (
	loadOnce sync.Once
	loadErr  error

	getProp       func(obj uint32, a *propAddr, qsize uint32, q unsafe.Pointer, size *uint32, out unsafe.Pointer) int32
	createTap     func(desc objc.ID, out *uint32) int32
	destroyTap    func(tap uint32) int32
	createAgg     func(desc objc.ID, out *uint32) int32
	destroyAgg    func(dev uint32) int32
	createIOProc  func(dev uint32, proc, client uintptr, out *uintptr) int32
	destroyIOProc func(dev uint32, proc uintptr) int32
	startDevice   func(dev uint32, proc uintptr) int32
	stopDevice    func(dev uint32, proc uintptr) int32

	// ioProc is the one callback, for purego makes only so many: it feeds
	// the meter of the tap running, as only one runs at a time.
	ioProc  uintptr
	feeding atomic.Pointer[Meter]
)

func load() error {
	loadOnce.Do(func() {
		ca, err := purego.Dlopen("/System/Library/Frameworks/CoreAudio.framework/CoreAudio", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = err
			return
		}
		if _, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			loadErr = err
			return
		}
		if objc.GetClass("CATapDescription") == 0 {
			loadErr = ErrUnsupported
			return
		}
		purego.RegisterLibFunc(&getProp, ca, "AudioObjectGetPropertyData")
		purego.RegisterLibFunc(&createTap, ca, "AudioHardwareCreateProcessTap")
		purego.RegisterLibFunc(&destroyTap, ca, "AudioHardwareDestroyProcessTap")
		purego.RegisterLibFunc(&createAgg, ca, "AudioHardwareCreateAggregateDevice")
		purego.RegisterLibFunc(&destroyAgg, ca, "AudioHardwareDestroyAggregateDevice")
		purego.RegisterLibFunc(&createIOProc, ca, "AudioDeviceCreateIOProcID")
		purego.RegisterLibFunc(&destroyIOProc, ca, "AudioDeviceDestroyIOProcID")
		purego.RegisterLibFunc(&startDevice, ca, "AudioDeviceStart")
		purego.RegisterLibFunc(&stopDevice, ca, "AudioDeviceStop")
		ioProc = purego.NewCallback(feed)
	})
	return loadErr
}

// feed is the aggregate device's IO proc, on Core Audio's thread: in is
// an AudioBufferList of float samples.
func feed(dev uint32, now, in, inTime, out, outTime, client unsafe.Pointer) int32 {
	m := feeding.Load()
	if m == nil || in == nil {
		return 0
	}
	n := *(*uint32)(in)
	for i := range int(n) {
		// mNumberChannels, mDataByteSize and mData, after the count and
		// its padding.
		buf := unsafe.Add(in, 8+i*16)
		channels := *(*uint32)(buf)
		size := *(*uint32)(unsafe.Add(buf, 4))
		data := *(*unsafe.Pointer)(unsafe.Add(buf, 8))
		if data != nil && size >= 4 {
			m.Feed(unsafe.Slice((*float32)(data), size/4), int(channels))
		}
	}
	return 0
}

func get[T any](obj, sel, scope uint32) (T, bool) {
	var v T
	size := uint32(unsafe.Sizeof(v))
	ok := getProp(obj, &propAddr{sel, scope, 0}, 0, nil, &size, unsafe.Pointer(&v)) == 0
	return v, ok
}

// Tap feeds m what process pid plays until stop is called, which the
// caller does before tapping again.
func Tap(pid int, m *Meter) (stop func(), err error) {
	if err := load(); err != nil {
		return nil, err
	}
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))

	p := int32(pid)
	var proc uint32
	size := uint32(4)
	getProp(1, &propAddr{fourcc("id2p"), glob, 0}, 4, unsafe.Pointer(&p), &size, unsafe.Pointer(&proc))
	if proc == 0 {
		// Core Audio knows a process once it opens an output.
		return nil, fmt.Errorf("levels: process %d plays nothing yet", pid)
	}

	cls := func(name string) objc.ID { return objc.ID(objc.GetClass(name)) }
	sel := objc.RegisterName
	str := func(s string) objc.ID { return cls("NSString").Send(sel("stringWithUTF8String:"), s) }
	one := cls("NSNumber").Send(sel("numberWithInt:"), int32(1))
	list := func(o objc.ID) objc.ID { return cls("NSArray").Send(sel("arrayWithObject:"), o) }
	dict := func(kv ...objc.ID) objc.ID {
		d := cls("NSMutableDictionary").Send(sel("dictionary"))
		for i := 0; i+1 < len(kv); i += 2 {
			d.Send(sel("setObject:forKey:"), kv[i+1], kv[i])
		}
		return d
	}

	desc := cls("CATapDescription").Send(sel("alloc")).
		Send(sel("initStereoMixdownOfProcesses:"), list(cls("NSNumber").Send(sel("numberWithUnsignedInt:"), proc)))
	defer desc.Send(sel("release"))
	// Heard by the app alone, and still played as before.
	desc.Send(sel("setPrivate:"), true)
	desc.Send(sel("setMuteBehavior:"), 0)
	var tap uint32
	if st := createTap(desc, &tap); st != 0 {
		return nil, fmt.Errorf("levels: tapping process %d: %d", pid, st)
	}
	tapUID := desc.Send(sel("UUID")).Send(sel("UUIDString"))
	var dev uint32
	st := createAgg(dict(
		str("name"), str("MyGoify levels"),
		str("uid"), cls("NSUUID").Send(sel("UUID")).Send(sel("UUIDString")),
		str("private"), one,
		str("tapautostart"), one,
		str("taps"), list(dict(str("uid"), tapUID, str("drift"), one)),
	), &dev)
	if st != 0 {
		destroyTap(tap)
		return nil, fmt.Errorf("levels: aggregate device: %d", st)
	}
	rate, _ := get[float64](dev, fourcc("nsrt"), glob)
	m.SetFormat(int(rate), outputLatency())

	var id uintptr
	if st := createIOProc(dev, ioProc, 0, &id); st != 0 {
		destroyAgg(dev)
		destroyTap(tap)
		return nil, fmt.Errorf("levels: io proc: %d", st)
	}
	feeding.Store(m)
	if st := startDevice(dev, id); st != 0 {
		feeding.Store(nil)
		destroyIOProc(dev, id)
		destroyAgg(dev)
		destroyTap(tap)
		return nil, fmt.Errorf("levels: starting: %d", st)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			stopDevice(dev, id)
			feeding.CompareAndSwap(m, nil)
			destroyIOProc(dev, id)
			destroyAgg(dev)
			destroyTap(tap)
		})
	}, nil
}

// outputLatency is how long the default output takes to play what is
// mixed: a few milliseconds through the speakers, a fifth of a second or
// so over Bluetooth.
func outputLatency() time.Duration {
	dev, ok := get[uint32](1, fourcc("dOut"), glob)
	if !ok || dev == 0 {
		return 0
	}
	rate, _ := get[float64](dev, fourcc("nsrt"), glob)
	if rate <= 0 {
		return 0
	}
	latency, _ := get[uint32](dev, fourcc("ltnc"), output)
	safety, _ := get[uint32](dev, fourcc("saft"), output)
	buffer, _ := get[uint32](dev, fourcc("fsiz"), output)
	frames := latency + safety + buffer
	if stream, ok := get[uint32](dev, fourcc("stm#"), output); ok && stream != 0 {
		l, _ := get[uint32](stream, fourcc("ltnc"), glob)
		frames += l
	}
	return time.Duration(float64(frames) / rate * float64(time.Second))
}
