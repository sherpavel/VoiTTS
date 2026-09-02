// PipeWire/PulseAudio microphone, made out of two pactl modules:
//
// - module-null-sink      a sink that discards its output, named "voitts"
// - module-remap-source   a capture device fed from that sink's monitor
package audio

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Names of the devices this file creates. Descriptions avoid spaces so they
// need no quoting inside pactl's property syntax.
const (
	SinkName   = "voitts"
	SourceName = "voitts_mic"

	sinkDesc   = "VoiTTS_Sink"
	sourceDesc = "VoiTTS_Microphone"
)

// pulseMic is a null sink paired with the capture device that reads from it.
type pulseMic struct {
	sink   string // the name to play audio into
	source string // the capture device other applications should select

	modules    []string // pactl module IDs this process loaded
	playerBin  string   // resolved playback binary
	playerKind string   // "pw-cat" or "paplay"
}

// Open provisions the virtual microphone, reusing any device that already
// exists under the expected name.
func Open(ctx context.Context, opts Options) (VirtualMic, error) {
	if _, err := exec.LookPath("pactl"); err != nil {
		return nil, fmt.Errorf("pactl not found in PATH; install pipewire-pulse or pulseaudio-utils: %w", err)
	}

	m := &pulseMic{sink: SinkName, source: SourceName}

	bin, kind, err := resolvePlayer()
	if err != nil {
		return nil, err
	}
	m.playerBin, m.playerKind = bin, kind

	if err := m.provision(ctx, opts); err != nil {
		// Roll back whatever was loaded before the failure.
		m.Close()
		return nil, err
	}
	return m, nil
}

func (m *pulseMic) Source() string { return m.source }

func (m *pulseMic) provision(ctx context.Context, opts Options) error {
	found, err := deviceExists(ctx, "sinks", m.sink)
	if err != nil {
		return err
	}
	if !found {
		id, err := loadModule(ctx,
			"module-null-sink",
			"sink_name="+m.sink,
			"sink_properties=device.description="+sinkDesc,
		)
		if err != nil {
			return fmt.Errorf("create sink %q: %w", m.sink, err)
		}
		m.modules = append(m.modules, id)
	}

	found, err = deviceExists(ctx, "sources", m.source)
	if err != nil {
		return err
	}
	if !found {
		id, err := loadModule(ctx,
			"module-remap-source",
			"master="+m.sink+".monitor",
			"source_name="+m.source,
			"source_properties=device.description="+sourceDesc,
		)
		if err != nil {
			return fmt.Errorf("create source %q: %w", m.source, err)
		}
		m.modules = append(m.modules, id)
	}

	if opts.Monitor {
		if err := m.enableMonitor(ctx, opts.MonitorLatency); err != nil {
			return err
		}
	}
	return nil
}

// enableMonitor mirrors the sink to the default output so you hear what is
// being sent to the microphone.
//
// The loopback is given no sink argument on purpose: it then attaches to
// whatever output is currently the default, rather than pinning a device name
// that may not be the one in use.
func (m *pulseMic) enableMonitor(ctx context.Context, latencyMS int) error {
	if latencyMS <= 0 {
		latencyMS = DefaultMonitorLatency
	}
	id, err := loadModule(ctx,
		"module-loopback",
		"source="+m.sink+".monitor",
		"latency_msec="+strconv.Itoa(latencyMS),
	)
	if err != nil {
		return fmt.Errorf("enable monitoring: %w", err)
	}
	m.modules = append(m.modules, id)
	return nil
}

// pulseStream is one player process, started once and held open for the life
// of the program.
//
// One long-lived player is what lets the synthesizer stay long-lived too: the
// audio carries no utterance boundaries, so there is nothing to close between
// sentences. Idle gaps are simply silence on the sink.
type pulseStream struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	name   string
	stderr *strings.Builder
}

// OpenStream starts the player and connects it to the sink.
func (m *pulseMic) OpenStream(ctx context.Context, f Format) (Stream, error) {
	argv, err := m.playerArgv(f)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	// The sink is already named on the command line; PULSE_SINK is a fallback
	// for players that route by environment instead.
	cmd.Env = append(os.Environ(), "PULSE_SINK="+m.sink)

	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.playerKind, err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s: %w", m.playerKind, err)
	}

	return &pulseStream{cmd: cmd, in: in, name: m.playerKind, stderr: &stderr}, nil
}

// playerArgv builds a raw-PCM playback command for the resolved player.
// The stream is headerless, so the format has to be stated explicitly.
func (m *pulseMic) playerArgv(f Format) ([]string, error) {
	rate := strconv.Itoa(f.SampleRate)
	channels := strconv.Itoa(f.Channels)

	switch m.playerKind {
	case "pw-cat":
		format, err := pwFormat(f.Bits)
		if err != nil {
			return nil, err
		}
		return []string{
			m.playerBin, "--playback", "--target", m.sink,
			"--raw", "--format", format, "--rate", rate, "--channels", channels,
			"-",
		}, nil
	case "paplay":
		format, err := paFormat(f.Bits)
		if err != nil {
			return nil, err
		}
		// paplay reads stdin when given no file argument.
		return []string{
			m.playerBin, "--device", m.sink,
			"--raw", "--format=" + format, "--rate=" + rate, "--channels=" + channels,
		}, nil
	}
	return nil, fmt.Errorf("unsupported player %q", m.playerKind)
}

func pwFormat(bits int) (string, error) {
	switch bits {
	case 16:
		return "s16", nil
	case 32:
		return "s32", nil
	}
	return "", fmt.Errorf("pw-cat: unsupported sample size %d", bits)
}

func paFormat(bits int) (string, error) {
	switch bits {
	case 16:
		return "s16le", nil
	case 32:
		return "s32le", nil
	}
	return "", fmt.Errorf("paplay: unsupported sample size %d", bits)
}

// Write sends PCM to the microphone.
func (s *pulseStream) Write(p []byte) (int, error) {
	n, err := s.in.Write(p)
	if err != nil {
		return n, fmt.Errorf("%s: %w: %s", s.name, err, strings.TrimSpace(s.stderr.String()))
	}
	return n, nil
}

// Close stops the player by closing its input, dropping whatever it still had
// buffered. It does not wait for the process: cancelling the context the
// stream was opened with kills it if it is somehow still there.
func (s *pulseStream) Close() error {
	if s.cmd == nil {
		return nil
	}
	s.in.Close()
	s.cmd = nil
	return nil
}

// Close unloads the modules Open created, in reverse order so the source goes
// before the sink it depends on. It builds its own context because the
// caller's is typically already cancelled by the interrupt that triggered the
// shutdown, and teardown still has to run.
func (m *pulseMic) Close() error {
	if m == nil || len(m.modules) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var firstErr error
	for i := len(m.modules) - 1; i >= 0; i-- {
		if _, err := run(ctx, "pactl", "unload-module", m.modules[i]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	m.modules = nil
	return firstErr
}

// resolvePlayer picks a playback command able to target a sink by name.
func resolvePlayer() (bin, kind string, err error) {
	for _, name := range []string{"pw-cat", "paplay"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, filepath.Base(name), nil
		}
	}
	return "", "", fmt.Errorf("no playback command found; install pipewire (pw-cat) or pulseaudio-utils (paplay)")
}

// deviceExists reports whether a sink or source is already present. kind is
// "sinks" or "sources". Device names are matched rather than IDs, which are
// reassigned every time a module is loaded.
func deviceExists(ctx context.Context, kind, name string) (bool, error) {
	out, err := run(ctx, "pactl", "list", "short", kind)
	if err != nil {
		return false, err
	}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		// id \t name \t driver \t format \t state
		if fields := strings.Split(sc.Text(), "\t"); len(fields) > 1 && fields[1] == name {
			return true, nil
		}
	}
	return false, sc.Err()
}

func loadModule(ctx context.Context, args ...string) (string, error) {
	out, err := run(ctx, "pactl", append([]string{"load-module"}, args...)...)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	if id == "" {
		return "", fmt.Errorf("pactl load-module returned no module id")
	}
	return id, nil
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return stdout.String(), nil
}
