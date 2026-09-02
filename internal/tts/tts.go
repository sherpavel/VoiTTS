package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// PCM format produced by Piper
type Format struct {
	SampleRate int
	Channels   int
	Bits       int
}

const DefaultModel = "en_US-hfc_male-medium"

type Piper struct {
	argv        []string // command prefix, e.g. ["python3", "-m", "piper"]
	model       string   // absolute path to the .onnx model
	voice       string   // display name
	format      Format
	speaker     int
	lengthScale float64

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr *strings.Builder
}

// New locates the Piper CLI and the requested voice model, and reads the
// voice's sample rate. No process is started until Start.
//
// model is either a path to a .onnx file or a voice name such as
// "en_US-hfc_male-medium", which is looked up in the standard Piper data
// directories. cmdline overrides CLI discovery when non-empty.
func New(cmdline, model string, rate, speaker int) (*Piper, error) {
	argv, err := resolvePiperCmd(cmdline)
	if err != nil {
		return nil, err
	}
	modelPath, sampleRate, err := LookupVoice(model)
	if err != nil {
		return nil, err
	}

	return &Piper{
		argv:  argv,
		model: modelPath,
		voice: strings.TrimSuffix(filepath.Base(modelPath), ".onnx"),
		// Piper's raw output is always mono 16-bit; only the rate varies by
		// voice, and the low tier differs from medium and high.
		format:  Format{SampleRate: sampleRate, Channels: 1, Bits: 16},
		speaker: speaker,
		// length-scale is phoneme duration, so it runs opposite to rate:
		// larger values speak slower. The exponential keeps the -10..10 range
		// symmetric around 1.0.
		lengthScale: math.Pow(1.07, float64(-clamp(rate, -10, 10))),
	}, nil
}

// Format reports the PCM layout the voice will produce.
func (p *Piper) Format() Format { return p.format }

func (p *Piper) Name() string {
	return fmt.Sprintf("piper %s @ %d Hz, length-scale %.2f",
		p.voice, p.format.SampleRate, p.lengthScale)
}

// Start launches the persistent Piper process writing PCM into out.
func (p *Piper) Start(ctx context.Context, out io.Writer) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil {
		return fmt.Errorf("piper: already started")
	}

	args := append([]string{}, p.argv[1:]...)
	args = append(args,
		"--model", p.model,
		"--output-raw",
		"--length-scale", strconv.FormatFloat(p.lengthScale, 'f', 3, 64),
	)
	if p.speaker > 0 {
		args = append(args, "--speaker", strconv.Itoa(p.speaker))
	}

	cmd := exec.CommandContext(ctx, p.argv[0], args...)
	cmd.Stdout = out

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("piper: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("piper: %w", err)
	}

	p.cmd, p.stdin, p.stderr = cmd, stdin, &stderr
	return nil
}

// Say queues one utterance. It returns once the text has been handed to the
// running Piper process, which is not when playback finishes.
func (p *Piper) Say(text string) error {
	text = flatten(text)
	if text == "" {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stdin == nil {
		return fmt.Errorf("piper: Start not called")
	}

	if _, err := io.WriteString(p.stdin, text+"\n"); err != nil {
		// A closed pipe means the process is gone; its stderr says why.
		return cmdError("piper", err, p.stderr.String())
	}
	return nil
}

// Close stops Piper by closing its input, and does not wait around for it:
// whatever is still queued is lost. Cancelling the context the process was
// started with kills off anything that has not exited by then.
func (p *Piper) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil {
		return nil
	}

	if p.stdin != nil {
		p.stdin.Close()
	}
	p.cmd, p.stdin = nil, nil
	return nil
}

// LookupCmd reports how Piper would be launched, without launching it. The
// discovery is the one New performs, so a nil error here means New will not
// fail on the CLI. An empty override selects automatic discovery.
func LookupCmd(override string) ([]string, error) {
	return resolvePiperCmd(override)
}

// LookupVoice resolves a voice name or path to its model file and sample rate
// the way New does, so a nil error here means New will not fail on the voice.
// Both the .onnx and its companion .onnx.json have to be readable. An empty
// model selects DefaultModel.
func LookupVoice(model string) (path string, sampleRate int, err error) {
	if model == "" {
		model = DefaultModel
	}
	path, err = resolveModel(model)
	if err != nil {
		return "", 0, err
	}
	sampleRate, err = readSampleRate(path)
	if err != nil {
		return "", 0, err
	}
	return path, sampleRate, nil
}

// resolvePiperCmd finds a way to invoke Piper.
//
// The Python module is preferred over a bare "piper" on PATH because Arch's
// extra/piper package is an unrelated gaming-mouse configurator, and launching
// that instead of a synthesizer is a confusing way to fail.
//
// Which interpreters are worth trying differs by platform; the list lives in
// dirs_linux.go and dirs_windows.go.
func resolvePiperCmd(override string) ([]string, error) {
	if override != "" {
		// An override that names a file is taken whole. Splitting it would
		// let an override be a command line rather than a path -- "python -m
		// piper" -- but on Windows a path with a space in it is far commoner
		// than that, and silently cutting one in half is a baffling failure.
		if info, err := os.Stat(override); err == nil && !info.IsDir() {
			return []string{override}, nil
		}

		argv := strings.Fields(override)
		if len(argv) == 0 {
			return nil, fmt.Errorf("piper command override is blank")
		}
		// An override naming nothing runnable is a typo, and saying so here is
		// the difference between the preflight report catching it and the
		// server dying on the first utterance.
		// Not quoted here: exec.Error already prints the name quoted, and on a
		// Windows path strconv.Quote doubles every separator, so saying it
		// twice reads as two different broken paths.
		if _, err := exec.LookPath(argv[0]); err != nil {
			return nil, fmt.Errorf("piper command override: %w", err)
		}
		return argv, nil
	}

	for _, py := range pythonCandidates {
		bin, err := exec.LookPath(py)
		if err != nil {
			continue
		}
		if exec.Command(bin, "-c", "import piper.__main__").Run() == nil {
			return []string{bin, "-m", "piper"}, nil
		}
	}

	if bin, err := exec.LookPath("piper"); err == nil {
		return []string{bin}, nil
	}

	// PATH is not the only place a correctly installed Piper can be. Both uv
	// and pipx write their console scripts to ~/.local/bin, which the shell
	// picks up by default on Linux and on no Windows machine at all: there,
	// `uv tool install piper-tts` succeeds and leaves nothing on PATH.
	binDirs := piperBinDirs()
	for _, dir := range binDirs {
		candidate := filepath.Join(dir, piperExeName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return []string{candidate}, nil
		}
	}

	return nil, fmt.Errorf("no importable piper module (tried %s), and no %s in PATH or %s",
		strings.Join(pythonCandidates, ", "), piperExeName, strings.Join(binDirs, ", "))
}

// piperBinDirs lists the directories Python tool installers put console
// scripts in, most specific first. It is searched after PATH: PATH is what the
// user configured, and this is only the fallback for a tool that installed
// itself correctly somewhere the shell was never told about.
func piperBinDirs() []string {
	var dirs []string
	// Both installers let the location be moved, and someone who has moved it
	// is exactly the person whose piper is not where this would otherwise look.
	for _, key := range []string{"UV_TOOL_BIN_DIR", "PIPX_BIN_DIR"} {
		if dir := os.Getenv(key); dir != "" {
			dirs = append(dirs, dir)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	return dirs
}

// DefaultVoiceDir is the directory voices are searched for first, and so the
// one the install instructions point at. The search list itself is built per
// platform, in dirs_linux.go and dirs_windows.go.
func DefaultVoiceDir() string {
	if dirs := piperDataDirs(); len(dirs) > 0 {
		return dirs[0]
	}
	return "."
}

// resolveModel turns a voice name or path into an absolute .onnx path.
func resolveModel(model string) (string, error) {
	if strings.ContainsRune(model, os.PathSeparator) || strings.HasSuffix(model, ".onnx") {
		abs, err := filepath.Abs(model)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("piper model %s: %w", abs, err)
		}
		return abs, nil
	}

	dirs := piperDataDirs()
	for _, dir := range dirs {
		candidate := filepath.Join(dir, model+".onnx")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("piper voice %q not found in %s\ndownload it with: python -m piper.download_voices %s --data-dir %s",
		model, strings.Join(dirs, ", "), model, DefaultVoiceDir())
}

// readSampleRate pulls audio.sample_rate from the voice's companion config,
// which Piper expects to sit alongside the model as <model>.onnx.json.
func readSampleRate(modelPath string) (int, error) {
	configPath := modelPath + ".json"
	data, err := os.ReadFile(configPath)
	if err != nil {
		return 0, fmt.Errorf("piper voice config: %w", err)
	}

	var cfg struct {
		Audio struct {
			SampleRate int `json:"sample_rate"`
		} `json:"audio"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return 0, fmt.Errorf("parse %s: %w", configPath, err)
	}
	if cfg.Audio.SampleRate <= 0 {
		return 0, fmt.Errorf("%s: missing audio.sample_rate", configPath)
	}
	return cfg.Audio.SampleRate, nil
}

// flatten collapses an utterance onto one line. Piper reads one utterance per
// line, so an embedded newline would split the text into several.
func flatten(text string) string {
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	return strings.TrimSpace(text)
}

// isShutdownErr reports whether a process exit is the orderly kind produced by
// closing its input or cancelling its context.
func cmdError(name string, err error, stderr string) error {
	if msg := strings.TrimSpace(stderr); msg != "" {
		return fmt.Errorf("%s: %w: %s", name, err, msg)
	}
	return fmt.Errorf("%s: %w", name, err)
}

func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}
