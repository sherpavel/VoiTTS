// The Linux half of the preflight report: the PipeWire/PulseAudio pieces the
// virtual microphone is assembled from, and the install lines for the rest.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"voitts/internal/tts"
)

// checkTimeout bounds the sound-server probe, so a server that is wedged
// rather than absent delays startup by a moment instead of forever.
const checkTimeout = 5 * time.Second

// audioChecks covers what audio.Open needs on Linux: pactl to load the
// modules, and a raw-PCM player to feed the sink they make.
func audioChecks(ctx context.Context) []result {
	return []result{checkAudioServer(ctx), checkPlayer()}
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

// piperHint is the fix line for a missing synthesizer.
func piperHint() string {
	return "uv tool install piper-tts   (or: pipx install piper-tts)\n" +
		"the distro package named `piper` on Arch is a mouse configurator, not this"
}

// voiceHint is the fix line for a missing voice. It names the directory
// tts.DefaultVoiceDir actually searches first, so the command it prints puts
// the voice somewhere the server will find it.
func voiceHint() string {
	dir := shortenHome(tts.DefaultVoiceDir())
	return "mkdir -p " + dir + "\n" +
		"uvx --from piper-tts python -m piper.download_voices " + tts.DefaultModel +
		" --data-dir " + dir
}

// shortenHome writes a path back with ~ for the home directory, which is both
// how the README spells the voice directories and short enough not to wrap.
//
// It is safe in hints as well as details here: a leading ~ in a word is
// expanded by every shell the fix lines are meant to be pasted into.
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
