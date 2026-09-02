// OS-agnostic audio interface:
//
//   - Linux - PipeWire/PulseAudio modules loaded through pactl: a null sink named "voitts" and a remapped source reading its monitor.
//   - Windows - the VB-CABLE driver, whose playback half ("CABLE Input") is already wired to its capture half ("CABLE Output").
package audio

import (
	"context"
	"io"
)

// Introduces delay to audio monitoring loopback to remove crackling.
const DefaultMonitorLatency = 50

// Options for virtual microphone.
type Options struct {
	// Mirror virtual microphone output to default playback device.
	Monitor bool
	// Monitoring buffer in milliseconds.
	// Zero selects DefaultMonitorLatency.
	// Bluetooth outputs need more delay.
	MonitorLatency int
}

// PCM stream format.
type Format struct {
	SampleRate int
	Channels   int
	Bits       int
}

// Capture device other applications can select.
// Close does teardown and cleanup.
//   - On Linux - the pair of PipeWire modules.
//   - On Windows - nothing to release, VB Cable outlives the program.
type VirtualMic interface {
	io.Closer

	// Capture device (microphone) other applications see.
	Source() string

	// Connects a live PCM feed to the microphone. Open once for the program lifetime.
	OpenStream(ctx context.Context, f Format) (Stream, error)
}

// SLive PCM connection to the virtual microphone.
// Writes are headerless samples in the stream Format.
// Close drops the buffer, not draining it.
type Stream interface {
	io.WriteCloser
}
