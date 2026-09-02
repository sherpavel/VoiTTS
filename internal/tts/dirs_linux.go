package tts

import (
	"os"
	"path/filepath"
)

// pythonCandidates are the interpreters tried, in order, when looking for an
// importable piper module.
var pythonCandidates = []string{"python3", "python"}

// piperExeName is the console script uv and pipx install.
const piperExeName = "piper"

// piperDataDirs lists where voice models are kept, most specific first.
func piperDataDirs() []string {
	var dirs []string
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		dirs = append(dirs, filepath.Join(xdg, "piper", "voices"), filepath.Join(xdg, "piper"))
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
	return append(dirs, "/usr/share/piper/voices", "/usr/share/piper")
}
