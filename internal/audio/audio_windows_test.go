package audio

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// testFormat is what Piper's default voice produces.
var testFormat = Format{SampleRate: 22050, Channels: 1, Bits: 16}

// openTestStream opens the default output device, skipping the test on a
// machine with no sound card at all. It plays to WAVE_MAPPER rather than to a
// cable so the test runs without VB-CABLE installed, and everything it
// exercises — the buffer ring, the wait, the teardown — is the same code the
// cable goes through.
//
// Every test below writes silence. The point is that the driver accepts and
// returns the buffers, and zeroed samples prove that as well as a tone does
// without making the machine beep at whoever ran the tests.
func openTestStream(t *testing.T, chunkMS int) *waveStream {
	t.Helper()
	if len(playbackDevices()) == 0 {
		t.Skip("no playback devices on this machine")
	}
	s, err := openWave(waveMapper, testFormat, chunkMS)
	if err != nil {
		t.Fatalf("openWave: %v", err)
	}
	t.Cleanup(func() { s.close() })
	return s
}

// TestWaveStreamWrite pushes more audio through the ring than it holds, in
// chunk sizes that do not divide evenly into a buffer, so both the wrap and
// the partial-fill path are taken.
func TestWaveStreamWrite(t *testing.T) {
	s := openTestStream(t, 20)

	bytesPerSec := testFormat.SampleRate * testFormat.Channels * testFormat.Bits / 8
	audio := make([]byte, bytesPerSec/2) // half a second

	const chunk = 3001 // deliberately coprime with the buffer size
	for off := 0; off < len(audio); off += chunk {
		end := min(off+chunk, len(audio))
		n, err := s.write(audio[off:end])
		if err != nil {
			t.Fatalf("write at offset %d: %v", off, err)
		}
		if n != end-off {
			t.Fatalf("write at offset %d: wrote %d bytes, want %d", off, n, end-off)
		}
	}

	// Half a second of audio through four 20 ms buffers has to have wrapped.
	if s.idx == 0 && s.used == 0 {
		t.Error("ring never advanced; buffers were not queued")
	}
}

// TestMonitorDropsWhenStalled covers the promise that keeps the microphone
// safe: a monitor that has stopped draining must never hold up the sink.
//
// The goroutine is deliberately not started, which is what a wedged output
// device looks like from here — the queue fills and stays full.
func TestMonitorDropsWhenStalled(t *testing.T) {
	s := &cableStream{
		sink:      openTestStream(t, streamChunkMS),
		monitor:   openTestStream(t, DefaultMonitorLatency),
		monitorCh: make(chan []byte, monitorQueue),
		quit:      make(chan struct{}),
	}

	audio := make([]byte, 8*1024)
	for range monitorQueue + 3 {
		start := time.Now()
		n, err := s.Write(audio)
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		if n != len(audio) {
			t.Errorf("wrote %d bytes, want %d", n, len(audio))
		}
		// The sink's own ring holds far more than this, so nothing here
		// should have waited on anything.
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("write took %v; the stalled monitor held up the sink", elapsed)
		}
	}

	if dropped := s.dropped.Load(); dropped == 0 {
		t.Error("nothing was dropped; the queue accepted more than it can hold")
	}
}

// TestWaveStreamClose covers the teardown Close depends on: it releases the
// device, tolerates being called twice, and leaves writes failing rather than
// touching a closed handle.
func TestWaveStreamClose(t *testing.T) {
	s := openTestStream(t, 20)

	if err := s.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := s.close(); err != nil {
		t.Errorf("second close: %v, want nil", err)
	}

	if _, err := s.write([]byte{0, 0, 0, 0}); !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("write after close: %v, want %v", err, io.ErrClosedPipe)
	}
}

// TestOpenWaveRejectsFormat checks the guards in front of waveOutOpen, which
// otherwise fails with a WAVERR_BADFORMAT that says nothing about which field
// was wrong.
func TestOpenWaveRejectsFormat(t *testing.T) {
	for _, f := range []Format{
		{SampleRate: 22050, Channels: 1, Bits: 24},
		{SampleRate: 0, Channels: 1, Bits: 16},
		{SampleRate: 22050, Channels: 0, Bits: 16},
	} {
		if s, err := openWave(waveMapper, f, 20); err == nil {
			s.close()
			t.Errorf("openWave(%+v) succeeded, want an error", f)
		}
	}
}

// TestCaptureHalf checks the name the report tells users to select. The
// derived name has to come back even when the device is absent, since that is
// what a half-installed cable is told to look for.
func TestCaptureHalf(t *testing.T) {
	name, found := captureHalf("CABLE Input")
	if found {
		t.Logf("cable installed; capture half is %q", name)
		return
	}
	if name != "CABLE Output" {
		t.Errorf("captureHalf(%q) = %q, want %q", "CABLE Input", name, "CABLE Output")
	}
}

// TestMonitorKeepsUp reproduces what the monitor actually has to survive.
//
// Piper does not produce audio in real time: it synthesises a whole sentence
// in a burst, and io.Copy hands that over in 32 KiB pieces — about 740 ms of
// speech each. The sink absorbs one ring's worth and then blocks, which is
// what paces the run. The monitor sees the same pieces, and has to end up
// playing the whole sentence rather than the first fragment of it.
func TestMonitorKeepsUp(t *testing.T) {
	s := &cableStream{sink: openTestStream(t, streamChunkMS)}
	s.startMonitor(openTestStream(t, DefaultMonitorLatency))

	bytesPerSec := testFormat.SampleRate * testFormat.Channels * testFormat.Bits / 8
	audio := make([]byte, 3*bytesPerSec) // three seconds of speech

	const chunk = 32 * 1024 // what io.Copy uses
	for off := 0; off < len(audio); off += chunk {
		end := min(off+chunk, len(audio))
		if _, err := s.Write(audio[off:end]); err != nil {
			t.Fatalf("write at offset %d: %v", off, err)
		}
	}

	dropped := s.dropped.Load()
	pct := float64(dropped) / float64(len(audio)) * 100
	t.Logf("monitor dropped %d of %d bytes (%.1f%%)", dropped, len(audio), pct)

	// A stalled output device may cost a buffer here and there. Losing a
	// tenth of the speech means the policy is wrong, not the device.
	if pct > 10 {
		t.Errorf("monitor dropped %.1f%% of the audio; it plays the start of a sentence and then falls silent", pct)
	}
}

// TestMonitorKeepsUpOnRealCable is TestMonitorKeepsUp against the devices the
// server actually opens, which TestMonitorKeepsUp cannot be: it plays to
// WAVE_MAPPER on both ends, so both run off one clock. Here the sink is the
// cable and the monitor is the default output — two devices, two clocks — and
// a drift between them shows up as drops that the single-device test cannot
// produce. It also exercises Close, goroutine and all.
func TestMonitorKeepsUpOnRealCable(t *testing.T) {
	if _, _, err := Lookup(); err != nil {
		t.Skip("no VB-CABLE installed:", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mic, err := Open(ctx, Options{Monitor: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer mic.Close()

	stream, err := mic.OpenStream(ctx, testFormat)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	cs, ok := stream.(*cableStream)
	if !ok {
		t.Fatalf("OpenStream returned %T, want *cableStream", stream)
	}

	bytesPerSec := testFormat.SampleRate * testFormat.Channels * testFormat.Bits / 8
	audio := make([]byte, 3*bytesPerSec)

	const chunk = 32 * 1024
	for off := 0; off < len(audio); off += chunk {
		end := min(off+chunk, len(audio))
		if _, err := stream.Write(audio[off:end]); err != nil {
			t.Fatalf("write at offset %d: %v", off, err)
		}
	}

	dropped := cs.dropped.Load()
	pct := float64(dropped) / float64(len(audio)) * 100
	t.Logf("monitor dropped %d of %d bytes (%.1f%%)", dropped, len(audio), pct)
	if pct > 10 {
		t.Errorf("monitor dropped %.1f%% against the real cable", pct)
	}

	if err := stream.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
