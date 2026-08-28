package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"voitts/internal/tts"
)

// checkTimeout bounds the sound-server probe, so a server that is wedged
// rather than absent delays startup by a moment instead of forever.
const checkTimeout = 5 * time.Second

// reportWidth is the column the report wraps details at. It is a backstop for
// the pathological line — a list of search paths — not a target: a resolved
// path and its sample rate fit on one line.
const reportWidth = 90

// A result is one line of the preflight report.
type result struct {
	name   string // the piece being checked
	ok     bool
	detail string // what was found, or what went wrong
	hint   string // how to fix it; printed only on failure, may span lines
}

// preflight verifies everything the server shells out to at runtime — pactl
// and a raw-PCM player for the virtual microphone, Piper for synthesis, and
// the voice it speaks with — and writes a report to w. The web UI is not
// among them: it is compiled into this binary by internal/web.
//
// It returns an error naming the unmet dependencies if any check failed. The
// checks are read-only: nothing is loaded, started or downloaded here.
func preflight(ctx context.Context, w io.Writer) error {
	results := []result{
		checkAudioServer(ctx),
		checkPlayer(),
		checkPiper(),
		checkVoice(),
	}

	report(w, results)

	var missing []string
	for _, r := range results {
		if !r.ok {
			missing = append(missing, r.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("unmet dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

// checkAudioServer looks for pactl and asks it whether a sound server is
// actually reachable. Both matter: audio.Open shells out to pactl for every
// module it loads, and pactl without a server behind it fails at the first
// load-module rather than at startup.
func checkAudioServer(ctx context.Context) result {
	r := result{name: "audio server", hint: pkgHint(
		"libpulse pipewire-pulse",
		"pulseaudio-utils pipewire-pulse",
		"pulseaudio-utils pipewire-pulseaudio",
	)}

	bin, err := exec.LookPath("pactl")
	if err != nil {
		r.detail = "pactl not found in PATH"
		return r
	}

	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "info")
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		r.detail = fmt.Sprintf("%s: no sound server reachable: %s", bin, oneLine(stderr.String(), err.Error()))
		r.hint = "start the sound server: systemctl --user start pipewire-pulse"
		return r
	}

	r.ok = true
	r.detail = shortenHome(bin)
	if name := serverName(stdout.String()); name != "" {
		r.detail = fmt.Sprintf("%s (%s)", name, shortenHome(bin))
	}
	return r
}

// checkPlayer looks for the PCM player that feeds the sink, in the order
// audio.resolvePlayer picks one: pw-cat first, paplay as the fallback.
func checkPlayer() result {
	r := result{name: "pcm player", hint: pkgHint(
		"pipewire-audio (pw-cat) or libpulse (paplay)",
		"pipewire-bin (pw-cat) or pulseaudio-utils (paplay)",
		"pipewire-utils (pw-cat) or pulseaudio-utils (paplay)",
	)}

	for _, name := range []string{"pw-cat", "paplay"} {
		bin, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		r.ok, r.detail = true, shortenHome(bin)
		if name == "paplay" {
			r.detail += " (fallback; pw-cat is preferred but absent)"
		}
		return r
	}

	r.detail = "neither pw-cat nor paplay found in PATH"
	return r
}

// checkPiper resolves the synthesizer the same way tts.New does, which for the
// Python module means importing it rather than trusting the file to be there.
func checkPiper() result {
	r := result{name: "piper", hint: "uv tool install piper-tts   (or: pipx install piper-tts)\n" +
		"the distro package named `piper` on Arch is a mouse configurator, not this"}

	argv, err := tts.LookupCmd("")
	if err != nil {
		r.detail = "no importable `python -m piper` and no piper in PATH"
		return r
	}

	// The report is about legibility, not reproduction: a resolved binary under
	// the home directory reads better as ~ than in full.
	argv[0] = shortenHome(argv[0])
	r.ok, r.detail = true, strings.Join(argv, " ")
	return r
}

// checkVoice resolves the voice the server runs with. Both the model and its
// companion .onnx.json are required — the sample rate lives in the config, and
// the whole pipeline is configured from it.
func checkVoice() result {
	r := result{name: "voice", hint: "mkdir -p ~/.local/share/piper/voices\n" +
		"uvx --from piper-tts python -m piper.download_voices " + tts.DefaultModel +
		" --data-dir ~/.local/share/piper/voices"}

	path, rate, err := tts.LookupVoice(tts.DefaultModel)
	if err != nil {
		// The lookup error carries its own download line; the headline is the
		// part that says what is missing and where it was looked for.
		r.detail = oneLine(err.Error())
		return r
	}

	r.ok = true
	r.detail = fmt.Sprintf("%s (%d Hz)", shortenHome(path), rate)
	return r
}

// report renders the checks as an aligned block, each failure followed by its
// hint indented under the detail column.
func report(w io.Writer, results []result) {
	width := 0
	for _, r := range results {
		width = max(width, len(r.name))
	}
	indent := strings.Repeat(" ", width+6)

	fmt.Fprintln(w)
	for _, r := range results {
		mark := "x"
		if r.ok {
			mark = "+"
		}

		detail := wrap(r.detail, reportWidth-len(indent))
		fmt.Fprintf(w, "  %s %-*s  %s\n", mark, width, r.name, detail[0])
		for _, line := range detail[1:] {
			fmt.Fprintln(w, indent+line)
		}

		if r.ok || r.hint == "" {
			continue
		}
		// Hints are commands to copy, so they are printed as written. Only the
		// gutter marks them, and only on the first line of the block.
		for i, line := range strings.Split(r.hint, "\n") {
			gutter := indent
			if i == 0 {
				gutter = fmt.Sprintf("%*s", len(indent), "fix: ")
			}
			fmt.Fprintln(w, gutter+line)
		}
	}
	fmt.Fprintln(w)
}

// wrap breaks text into lines of at most width columns, at spaces. A word too
// long to fit — a search path, usually — takes a line of its own rather than
// being cut. The result always has at least one line.
func wrap(text string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	return append(lines, line)
}

// shortenHome writes a path back with ~ for the home directory, which is both
// how the README spells the voice directories and short enough not to wrap.
func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rest, found := strings.CutPrefix(path, home+string(os.PathSeparator)); found {
		return "~" + string(os.PathSeparator) + rest
	}
	return path
}

// pkgHint formats the per-distro install lines the README lists.
func pkgHint(arch, debian, fedora string) string {
	return "Arch:          pacman -S " + arch + "\n" +
		"Debian/Ubuntu: apt install " + debian + "\n" +
		"Fedora:        dnf install " + fedora
}

// serverName pulls the "Server Name:" field out of `pactl info`, which is what
// tells PipeWire's PulseAudio shim apart from real PulseAudio. The field is
// locale-dependent, so a miss is not an error.
func serverName(info string) string {
	for _, line := range strings.Split(info, "\n") {
		if name, found := strings.CutPrefix(line, "Server Name:"); found {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

// oneLine reduces messages to their headline, keeping the report one line per
// check. Later arguments are fallbacks for when the earlier ones are blank.
func oneLine(messages ...string) string {
	for _, msg := range messages {
		if head, _, _ := strings.Cut(strings.TrimSpace(msg), "\n"); head != "" {
			return head
		}
	}
	return ""
}
