// VB-CABLE microphone.
//
// Windows has no equivalent of loading a null sink at runtime: a capture
// device is a driver, and drivers are installed, not created. VB-CABLE is that
// driver. It installs as a pair —
//
//   - CABLE Input    a playback device, which is where this writes PCM
//   - CABLE Output   a capture device, which other applications select
//
// — permanently wired together, so there is nothing for Open to provision and
// nothing for Close to tear down. Open's job is to find the playback half and
// fail with an install hint when it is absent.
package audio

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

// InstallHint is the one thing a Windows machine cannot be talked through
// installing from a package manager, so the URL travels with the error.
const InstallHint = "install VB-CABLE from https://vb-audio.com/Cable/ " +
	"(run the installer as administrator, then reboot)"

// cablePrefixes are the VB-Audio playback devices this can drive, best first.
// The plain VB-CABLE is what the README tells people to install; the A/B and
// Hi-Fi variants are the same driver under different names, and cost one line
// each to support for anyone who already has one.
//
// Matching is by prefix because waveOutGetDevCaps truncates names at 31
// characters: "CABLE Input (VB-Audio Virtual Cable)" arrives as
// "CABLE Input (VB-Audio Virtual C".
var cablePrefixes = []string{
	"CABLE Input",
	"CABLE-A Input",
	"CABLE-B Input",
	"Hi-Fi Cable Input",
}

// Buffering for one stream. Four 50 ms buffers is enough that a scheduling
// hiccup does not crackle, and short enough that Close drops only a fifth of a
// second of speech.
const (
	streamBuffers = 4
	streamChunkMS = 50
)

// cableMic is the playback half of an installed cable, plus the name of the
// capture half so the server can tell the user what to select.
type cableMic struct {
	deviceID uint32 // waveOut device index of the cable's playback half
	sink     string // its name, as the sound control panel spells it
	source   string // the capture half's name

	monitor        bool
	monitorLatency int
}

// Open locates the cable. opts.Keep is ignored: the devices belong to the
// driver and outlive this process either way.
func Open(ctx context.Context, opts Options) (VirtualMic, error) {
	id, sink, prefix, err := findCable()
	if err != nil {
		// Callers that are not the preflight report have nowhere else to
		// learn this, so the error carries the hint. Lookup deliberately
		// does not: the report prints hints in a column of their own.
		return nil, fmt.Errorf("%w; %s", err, InstallHint)
	}

	source, _ := captureHalf(prefix)
	m := &cableMic{
		deviceID:       id,
		sink:           sink,
		source:         source,
		monitor:        opts.Monitor,
		monitorLatency: opts.MonitorLatency,
	}
	return m, nil
}

// Lookup reports the devices Open would use, without opening anything: the
// playback device PCM is written to, and the capture device other applications
// select. It is what the preflight report is built from.
//
// A cable whose capture half is missing — a half-finished install, or one that
// wants the reboot it asked for — comes back with an empty source and no
// error, since the playback half alone is what Open needs.
func Lookup() (sink, source string, err error) {
	_, sink, prefix, err := findCable()
	if err != nil {
		return "", "", err
	}
	if name, found := captureHalf(prefix); found {
		source = name
	}
	return sink, source, nil
}

func (m *cableMic) Source() string { return m.source }

// Close has nothing to undo. It exists to satisfy VirtualMic, and to keep the
// caller's teardown identical on both platforms.
func (m *cableMic) Close() error { return nil }

// findCable picks the first installed cable, returning its device ID, its full
// name and the prefix it matched.
func findCable() (id uint32, name, prefix string, err error) {
	devices := playbackDevices()
	for _, p := range cablePrefixes {
		for i, dev := range devices {
			if strings.HasPrefix(dev, p) {
				return uint32(i), dev, p, nil
			}
		}
	}
	if len(devices) == 0 {
		return 0, "", "", fmt.Errorf("no playback devices at all on this machine")
	}
	// The names are the truncated ones the driver reports, which is worth
	// seeing: it is how you tell "not installed" from "installed under a name
	// this does not recognise".
	return 0, "", "", fmt.Errorf("no VB-CABLE playback device among %s",
		strings.Join(quoteAll(devices), ", "))
}

// captureHalf names the recording device paired with a playback prefix. The
// pairing is by name — "CABLE Input" feeds "CABLE Output" — so the answer is
// the prefix with that one word swapped, expanded to the full device name when
// the driver is really there. When it is not, the derived name is returned
// anyway with found false: it is still the right thing to tell a user to look
// for once the install finishes.
func captureHalf(prefix string) (name string, found bool) {
	want := strings.Replace(prefix, "Input", "Output", 1)
	for _, dev := range captureDevices() {
		if strings.HasPrefix(dev, want) {
			return dev, true
		}
	}
	return want, false
}

func quoteAll(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, fmt.Sprintf("%q", n))
	}
	return out
}

// OpenStream opens the cable for playback, and the default output alongside it
// when monitoring is on.
func (m *cableMic) OpenStream(ctx context.Context, f Format) (Stream, error) {
	sink, err := openWave(uintptr(m.deviceID), f, streamChunkMS)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", m.sink, err)
	}

	s := &cableStream{sink: sink}

	if m.monitor {
		latency := m.monitorLatency
		if latency <= 0 {
			latency = DefaultMonitorLatency
		}
		// Monitoring plays to WAVE_MAPPER rather than a named device, for the
		// same reason the PipeWire loopback names no sink: it should follow
		// whatever output is currently the default.
		mon, err := openWave(waveMapper, f, latency)
		if err != nil {
			sink.close()
			return nil, fmt.Errorf("enable monitoring: %w", err)
		}
		s.startMonitor(mon)
	}

	// The player process on Linux dies with the context; this is the same
	// promise, kept by hand. The registration is dropped with the context,
	// and there is one stream per run, so it is not worth unregistering.
	context.AfterFunc(ctx, func() { s.Close() })
	return s, nil
}

// monitorQueue is how many pieces of audio may be waiting for the monitor.
//
// Two, because the queue is there to decouple two devices running off
// independent clocks, not to buffer speech: in the steady state it holds
// nothing, since the monitor drains at the same rate the sink does. Making it
// deeper would only add latency to what is meant to be heard as it is sent.
const monitorQueue = 2

// cableStream writes each buffer to the cable, and hands a copy to the monitor
// goroutine when one is running.
type cableStream struct {
	sink *waveStream

	// The monitor plays on its own goroutine so it can block on its device the
	// way the sink does. Writing to both in turn from here would serialise
	// them and halve the rate; writing to the monitor without blocking loses
	// most of the audio, because Piper delivers a sentence far faster than
	// real time and only the blocking write paces it back down.
	monitor   *waveStream
	monitorCh chan []byte
	quit      chan struct{}
	monitorWG sync.WaitGroup
	dropped   atomic.Int64 // bytes the monitor queue refused

	closeOnce sync.Once
}

// startMonitor attaches an open device and the goroutine that feeds it.
func (s *cableStream) startMonitor(mon *waveStream) {
	s.monitor = mon
	s.monitorCh = make(chan []byte, monitorQueue)
	s.quit = make(chan struct{})

	s.monitorWG.Add(1)
	go func() {
		defer s.monitorWG.Done()
		for {
			select {
			case buf := <-s.monitorCh:
				// An error here is the device going away, or Close. Either
				// way the monitor is done and the microphone carries on.
				if _, err := s.monitor.write(buf); err != nil {
					return
				}
			case <-s.quit:
				return
			}
		}
	}()
}

// Write blocks until the cable has room, which is what paces Piper: the
// synthesizer outruns playback and its stdout has to fill somewhere.
//
// The monitor gets a copy, handed over without waiting. It is a convenience,
// and a default output that stalls — a Bluetooth headset reconnecting, say —
// must not hold up the microphone everyone else is listening to. A full queue
// means exactly that, and the audio is dropped rather than waited on.
func (s *cableStream) Write(p []byte) (int, error) {
	if s.monitorCh != nil {
		// p belongs to the caller and is reused the moment this returns, so
		// the monitor cannot be handed the same memory.
		buf := make([]byte, len(p))
		copy(buf, p)

		select {
		case s.monitorCh <- buf:
		default:
			s.dropped.Add(int64(len(p)))
		}
	}
	return s.sink.write(p)
}

// Close stops playback immediately, dropping whatever is still queued, and
// releases the devices.
//
// The monitor device is closed before the goroutine is joined: closing is what
// unblocks a write already in progress, so the wait cannot outlast the audio
// still queued in the driver.
func (s *cableStream) Close() error {
	var err error
	s.closeOnce.Do(func() {
		if s.monitor != nil {
			err = s.monitor.close()
			close(s.quit)
			s.monitorWG.Wait()
		}
		if sinkErr := s.sink.close(); err == nil {
			err = sinkErr
		}
	})
	return err
}

// waveStream is one open waveOut device with a ring of buffers cycling through
// it. Buffers are filled here and handed to the driver, which sets WHDR_DONE
// and signals the event when it has played one back.
type waveStream struct {
	handle uintptr        // HWAVEOUT
	event  windows.Handle // signalled by the driver on every completed buffer

	bufs [][]byte
	hdrs []waveHdr
	idx  int // buffer being filled
	used int // bytes already in it

	closed atomic.Bool
	mu     sync.Mutex // serialises write against itself
}

// openWave opens a device and prepares its buffers. chunkMS sizes each of the
// streamBuffers buffers, so the device holds up to streamBuffers*chunkMS of
// audio.
func openWave(deviceID uintptr, f Format, chunkMS int) (*waveStream, error) {
	if f.Bits != 16 && f.Bits != 32 {
		return nil, fmt.Errorf("waveOut: unsupported sample size %d", f.Bits)
	}
	if f.SampleRate <= 0 || f.Channels <= 0 {
		return nil, fmt.Errorf("waveOut: invalid format %d Hz, %d channels", f.SampleRate, f.Channels)
	}

	blockAlign := f.Channels * f.Bits / 8
	wfx := waveFormatEx{
		wFormatTag:      waveFormatPCM,
		nChannels:       uint16(f.Channels),
		nSamplesPerSec:  uint32(f.SampleRate),
		nAvgBytesPerSec: uint32(f.SampleRate * blockAlign),
		nBlockAlign:     uint16(blockAlign),
		wBitsPerSample:  uint16(f.Bits),
	}

	// Auto-reset, initially unsignalled. Waiters re-check the buffer's own
	// flags after every wake, so a signal consumed on another buffer's behalf
	// costs a lap of the loop rather than a lost wakeup.
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("waveOut: create event: %w", err)
	}

	s := &waveStream{event: event}
	ret, _, _ := procWaveOutOpen.Call(
		uintptr(unsafe.Pointer(&s.handle)),
		deviceID,
		uintptr(unsafe.Pointer(&wfx)),
		uintptr(event),
		0,
		callbackEvent,
	)
	if err := mmError("waveOutOpen", ret); err != nil {
		windows.CloseHandle(event)
		return nil, err
	}

	// Round the chunk down to whole frames: a buffer cut mid-frame swaps the
	// channels of everything after it.
	size := f.SampleRate * blockAlign * chunkMS / 1000
	size -= size % blockAlign
	size = max(size, blockAlign)

	s.bufs = make([][]byte, streamBuffers)
	s.hdrs = make([]waveHdr, streamBuffers)
	for i := range s.bufs {
		s.bufs[i] = make([]byte, size)
		if err := s.prepare(i); err != nil {
			s.close()
			return nil, err
		}
	}
	return s, nil
}

// prepare hands one buffer to the driver to lock down for DMA. The header
// keeps the buffer's address as a bare uintptr, which the garbage collector
// cannot see through; s.bufs is what keeps the memory alive, and the Go heap
// does not move it.
func (s *waveStream) prepare(i int) error {
	s.hdrs[i] = waveHdr{
		lpData:         uintptr(unsafe.Pointer(&s.bufs[i][0])),
		dwBufferLength: uint32(len(s.bufs[i])),
	}
	ret, _, _ := procWaveOutPrepareHeader.Call(s.handle,
		uintptr(unsafe.Pointer(&s.hdrs[i])), unsafe.Sizeof(s.hdrs[i]))
	return mmError("waveOutPrepareHeader", ret)
}

// write copies p into the ring, queueing each buffer as it fills and waiting
// whenever the ring is full. That wait is the point: it is what turns Piper's
// faster-than-real-time output into something played at the rate it is spoken.
//
// Callers that must not be held up run write on a goroutine of their own; the
// monitor does. Dropping audio is a decision made in front of this, not here.
func (s *waveStream) write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	total := len(p)
	for len(p) > 0 {
		if s.closed.Load() {
			return total - len(p), io.ErrClosedPipe
		}
		if err := s.awaitFree(); err != nil {
			return total - len(p), err
		}

		buf := s.bufs[s.idx]
		n := copy(buf[s.used:], p)
		s.used += n
		p = p[n:]

		if s.used == len(buf) {
			if err := s.queue(len(buf)); err != nil {
				return total - len(p), err
			}
		}
	}
	return total, nil
}

// awaitFree waits until the buffer at s.idx is out of the driver's queue.
//
// It is called with s.mu held, which close is careful not to take until after
// waveOutReset has emptied the queue: that is what releases a writer parked
// here, so the two cannot deadlock against each other.
func (s *waveStream) awaitFree() error {
	for s.hdrs[s.idx].flags()&whdrInQueue != 0 {
		if s.closed.Load() {
			return io.ErrClosedPipe
		}
		// A bounded wait rather than INFINITE: the event is auto-reset and
		// shared by every buffer, so this re-checks rather than trusting it.
		if _, err := windows.WaitForSingleObject(s.event, 250); err != nil {
			return fmt.Errorf("waveOut: wait: %w", err)
		}
	}
	return nil
}

// queue hands the current buffer to the driver and moves on to the next.
func (s *waveStream) queue(length int) error {
	// WHDR_DONE is left as the driver last set it: waveOutWrite raises
	// WHDR_INQUEUE, and that is the only flag awaitFree consults.
	s.hdrs[s.idx].dwBufferLength = uint32(length)

	ret, _, _ := procWaveOutWrite.Call(s.handle,
		uintptr(unsafe.Pointer(&s.hdrs[s.idx])), unsafe.Sizeof(s.hdrs[s.idx]))
	if err := mmError("waveOutWrite", ret); err != nil {
		return err
	}

	s.idx = (s.idx + 1) % len(s.bufs)
	s.used = 0
	return nil
}

// close stops the device and releases everything openWave took.
//
// waveOutReset comes first and deliberately: it drops the queued buffers,
// which both matches the Linux stream's Close and unparks any writer waiting
// on one of them, so taking s.mu below cannot deadlock against it.
func (s *waveStream) close() error {
	if s == nil || s.handle == 0 {
		return nil
	}
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	procWaveOutReset.Call(s.handle)

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.hdrs {
		procWaveOutUnprepareHeader.Call(s.handle,
			uintptr(unsafe.Pointer(&s.hdrs[i])), unsafe.Sizeof(s.hdrs[i]))
	}

	ret, _, _ := procWaveOutClose.Call(s.handle)
	s.handle = 0

	if s.event != 0 {
		windows.CloseHandle(s.event)
		s.event = 0
	}
	return mmError("waveOutClose", ret)
}
