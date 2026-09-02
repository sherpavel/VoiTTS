package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestHintsArePasteable guards a fix line that could not be pasted. The
// Windows voice hint used to print ~\AppData\Local\piper\voices, and while
// PowerShell expands ~ for its own cmdlets it hands the tilde to a native
// program verbatim: the download landed in a directory literally named "~"
// under the working directory, where nothing would ever look for it. cmd.exe
// does not expand it anywhere at all.
func TestHintsArePasteable(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("~ is shell syntax on unix, and expands in every shell these are pasted into")
	}
	for name, hint := range map[string]string{"piper": piperHint(), "voice": voiceHint()} {
		if strings.Contains(hint, "~") {
			t.Errorf("%s hint contains ~, which Windows does not expand for native commands:\n%s",
				name, hint)
		}
	}
}

// TestShortenHome pins the platform difference the hints depend on: details
// are shortened where the shorthand is real, and left alone where it is not.
func TestShortenHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory:", err)
	}
	path := filepath.Join(home, "somewhere", "piper")

	got := shortenHome(path)
	if runtime.GOOS == "windows" {
		if got != path {
			t.Errorf("shortenHome(%q) = %q, want it unchanged on Windows", path, got)
		}
		return
	}
	if want := filepath.Join("~", "somewhere", "piper"); got != want {
		t.Errorf("shortenHome(%q) = %q, want %q", path, got, want)
	}
}

// TestShortenHomeLeavesOtherPathsAlone checks the case that would mangle a
// system path: nothing outside the home directory may be rewritten.
func TestShortenHomeLeavesOtherPathsAlone(t *testing.T) {
	for _, path := range []string{
		filepath.Join(string(filepath.Separator), "usr", "bin", "pactl"),
		"relative/piper",
	} {
		if got := shortenHome(path); got != path {
			t.Errorf("shortenHome(%q) = %q, want it unchanged", path, got)
		}
	}
}
