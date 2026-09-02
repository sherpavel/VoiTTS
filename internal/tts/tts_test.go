package tts

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// isolate points every location resolvePiperCmd searches at an empty
// directory, so that a machine which really does have Piper installed — the
// one this was written on does — still runs the not-found paths.
func isolate(t *testing.T) {
	t.Helper()
	empty := t.TempDir()
	t.Setenv("PATH", "")
	t.Setenv("UV_TOOL_BIN_DIR", "")
	t.Setenv("PIPX_BIN_DIR", "")
	// os.UserHomeDir reads the first on unix and the second on Windows.
	t.Setenv("HOME", empty)
	t.Setenv("USERPROFILE", empty)
}

// touch creates an empty file, standing in for an installed console script.
func touch(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestResolvePiperCmdOverrideIsPath covers the case the flag mostly exists
// for: a path to an executable, which must survive being pointed at a
// directory whose name contains a space. Nearly every interesting directory on
// Windows does.
func TestResolvePiperCmdOverrideIsPath(t *testing.T) {
	exe := touch(t, filepath.Join(t.TempDir(), "Program Files", piperExeName))

	argv, err := resolvePiperCmd(exe)
	if err != nil {
		t.Fatalf("resolvePiperCmd(%q): %v", exe, err)
	}
	if !slices.Equal(argv, []string{exe}) {
		t.Errorf("resolvePiperCmd(%q) = %q, want the path in one piece", exe, argv)
	}
}

// TestResolvePiperCmdOverrideIsCommand checks that an override which is not a
// file still works as a command line, which is how a Python module is named.
// The interpreter is a stand-in on a PATH of one directory, so the test does
// not depend on the machine having a real Python.
func TestResolvePiperCmdOverrideIsCommand(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	touch(t, filepath.Join(dir, piperExeName))
	t.Setenv("PATH", dir)

	argv, err := resolvePiperCmd(piperExeName + " -m piper")
	if err != nil {
		t.Fatalf("resolvePiperCmd: %v", err)
	}
	if want := []string{piperExeName, "-m", "piper"}; !slices.Equal(argv, want) {
		t.Errorf("resolvePiperCmd = %q, want %q", argv, want)
	}
}

// TestResolvePiperCmdOverrideMustExist checks that a typo in the override is
// caught. Without this the preflight report marks a path to nothing as fine,
// and the server dies later instead.
func TestResolvePiperCmdOverrideMustExist(t *testing.T) {
	isolate(t)
	missing := filepath.Join(t.TempDir(), "nope", piperExeName)

	if argv, err := resolvePiperCmd(missing); err == nil {
		t.Errorf("resolvePiperCmd(%q) = %q, want an error", missing, argv)
	}
	if _, err := resolvePiperCmd("   "); err == nil {
		t.Error("resolvePiperCmd(blank) succeeded, want an error")
	}
}

// TestResolvePiperCmdFindsInstallDir is the bug this fixes: `uv tool install
// piper-tts` on Windows leaves piper in ~/.local/bin and nothing on PATH, and
// the server has to find it there anyway.
func TestResolvePiperCmdFindsInstallDir(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	exe := touch(t, filepath.Join(dir, piperExeName))
	t.Setenv("UV_TOOL_BIN_DIR", dir)

	argv, err := resolvePiperCmd("")
	if err != nil {
		t.Fatalf("resolvePiperCmd: %v", err)
	}
	if !slices.Equal(argv, []string{exe}) {
		t.Errorf("resolvePiperCmd = %q, want %q", argv, []string{exe})
	}
}

// TestResolvePiperCmdNotFound checks the error names where it looked. That
// list is the whole diagnostic: an install in an unsearched directory is
// otherwise indistinguishable from no install at all.
func TestResolvePiperCmdNotFound(t *testing.T) {
	isolate(t)
	t.Setenv("UV_TOOL_BIN_DIR", t.TempDir()) // exists, holds no piper

	_, err := resolvePiperCmd("")
	if err == nil {
		t.Fatal("resolvePiperCmd found something in an empty environment")
	}
	for _, want := range []string{piperExeName, os.Getenv("UV_TOOL_BIN_DIR")} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestPiperBinDirs checks the search order: an installer told to put its
// scripts somewhere unusual is exactly the case the default cannot cover.
func TestPiperBinDirs(t *testing.T) {
	t.Setenv("UV_TOOL_BIN_DIR", "/uv/bin")
	t.Setenv("PIPX_BIN_DIR", "/pipx/bin")

	dirs := piperBinDirs()
	if len(dirs) < 2 || dirs[0] != "/uv/bin" || dirs[1] != "/pipx/bin" {
		t.Fatalf("piperBinDirs() = %q, want the overrides first", dirs)
	}

	// The default is still there behind them, and is the only entry once the
	// overrides are gone.
	t.Setenv("UV_TOOL_BIN_DIR", "")
	t.Setenv("PIPX_BIN_DIR", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory:", err)
	}
	want := filepath.Join(home, ".local", "bin")
	if dirs := piperBinDirs(); !slices.Contains(dirs, want) {
		t.Errorf("piperBinDirs() = %q, want it to include %q", dirs, want)
	}
}

// TestPiperExeName guards the one thing the platform split has to get right.
func TestPiperExeName(t *testing.T) {
	want := "piper"
	if runtime.GOOS == "windows" {
		want = "piper.exe"
	}
	if piperExeName != want {
		t.Errorf("piperExeName = %q, want %q", piperExeName, want)
	}
}
