package tts

import (
	"os"
	"path/filepath"
)

// pythonCandidates are the interpreters tried, in order, when looking for an
// importable piper module.
//
// "python3" is deliberately absent. On Windows that name is usually the Store
// app-execution alias, a stub that answers every invocation by advertising the
// Microsoft Store instead of running anything. "py", the launcher shipped with
// python.org installs, reaches every real interpreter that stub would not.
var pythonCandidates = []string{"python", "py"}

// piperExeName is the console script uv and pipx install.
const piperExeName = "piper.exe"

// piperDataDirs lists where voice models are kept, most specific first.
//
// LocalAppData leads rather than AppData because a voice is tens of megabytes
// of model weights, and Roaming profiles copy their contents between machines
// at every logon. AppData is searched anyway, as is the XDG-style path, so a
// voice downloaded by following the Linux half of the README is still found.
func piperDataDirs() []string {
	var dirs []string
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dirs = append(dirs, filepath.Join(local, "piper", "voices"), filepath.Join(local, "piper"))
	}
	if roaming := os.Getenv("APPDATA"); roaming != "" {
		dirs = append(dirs, filepath.Join(roaming, "piper", "voices"), filepath.Join(roaming, "piper"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".local", "share", "piper", "voices"),
			filepath.Join(home, ".local", "share", "piper"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	// The executable's own directory, which is where an unzipped release sits
	// and the obvious place to drop a voice beside it.
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	return dirs
}
