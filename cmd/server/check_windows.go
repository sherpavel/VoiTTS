// The Windows half of the preflight report.
//
// There is no sound server to probe and no player to find: the audio engine is
// part of the OS and the PCM goes into it through winmm. What can be missing
// is VB-CABLE, and it can be missing by halves — the installer wants a reboot
// before the capture side appears — so the two ends are reported separately.
package main

import (
	"context"

	"voitts/internal/audio"
	"voitts/internal/tts"
)

// audioChecks covers what audio.Open needs on Windows: an installed cable,
// with both of its ends present.
func audioChecks(ctx context.Context) []result {
	sink, source, err := audio.Lookup()
	return []result{checkCable(sink, err), checkCaptureDevice(source, err)}
}

// checkCable reports the playback device this writes PCM into.
func checkCable(sink string, err error) result {
	r := result{name: "vb-cable", hint: audio.InstallHint}
	if err != nil {
		r.detail = oneLine(err.Error())
		return r
	}
	r.ok, r.detail = true, sink
	return r
}

// checkCaptureDevice reports the recording device other applications select.
//
// It is a separate line because a cable can be installed and still be half
// there: until the machine is rebooted, the playback end can be present while
// the capture end other applications need is not, and a report that only
// looked at the playback end would call that fine.
func checkCaptureDevice(source string, err error) result {
	r := result{name: "capture device", hint: audio.InstallHint}
	if err != nil {
		// The cable line above already said what went wrong, and repeating the
		// error under a second heading reads as two faults rather than one.
		r.detail = "no cable to have one"
		r.hint = ""
		return r
	}
	if source == "" {
		r.detail = "cable found, but its capture end is not in the recording device list"
		r.hint = "reboot: the VB-CABLE installer only registers the capture end on the next boot"
		return r
	}
	r.ok, r.detail = true, source
	return r
}

// piperHint is the fix line for a missing synthesizer.
func piperHint() string {
	return "uv tool install piper-tts   (or: pipx install piper-tts)\n" +
		"winget install --id astral-sh.uv   installs uv, if you have neither"
}

// voiceHint is the fix line for a missing voice. It names the directory
// tts.DefaultVoiceDir actually searches first, so the command it prints puts
// the voice somewhere the server will find it.
func voiceHint() string {
	dir := tts.DefaultVoiceDir()
	return "mkdir " + dir + "\n" +
		"uvx --from piper-tts python -m piper.download_voices " + tts.DefaultModel +
		" --data-dir " + dir
}

// shortenHome returns the path unchanged. It exists so that check.go can
// shorten paths without knowing whether the platform has a shorthand, and
// Windows has none worth using.
//
// "~" is not one. PowerShell expands it for its own cmdlets, so `mkdir ~\x`
// happens to work, but it is not shell syntax: a native program is handed the
// tilde verbatim. `--data-dir ~\AppData\...` therefore downloads the voice
// into a directory literally named "~" under the working directory, where
// nothing will ever look for it, and cmd.exe does not expand it anywhere at
// all. A path that is merely long is the better failure.
func shortenHome(path string) string { return path }
