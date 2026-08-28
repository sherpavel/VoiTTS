package profile

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

const storeFile = "profiles.json"

var ErrOrderMismatch = errors.New("list does not match the stored profiles")

// In-memory story and sync all changes to json file on change.
type ProfileStore struct {
	mu    sync.Mutex
	path  string // absolute
	store []Profile
}

func NewStore() *ProfileStore {
	profileStore := &ProfileStore{
		store: []Profile{},
	}
	return profileStore
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config directory: %w", err)
	}
	return filepath.Join(dir, "voitts", storeFile), nil
}

// Absolute path to profiles json file
func (ps *ProfileStore) Path() string {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.path
}

// Reads and parses profiles.json
func (ps *ProfileStore) Load(path string) error {
	if path == "" {
		def, err := DefaultPath()
		if err != nil {
			return err
		}
		path = def
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", path, err)
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.path = abs

	data, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		ps.store = []Profile{}
		return ps.saveLocked()
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", abs, err)
	}

	// A file emptied on purpose reads as "no profiles" rather than as damage.
	if len(bytes.TrimSpace(data)) == 0 {
		ps.store = []Profile{}
		return nil
	}

	// Strict on the way in, because this file is edited by hand and a mistake
	// that is quietly tolerated is one nobody finds: a member name that matches
	// no field is a typo, not an extension. (The API stays lenient about what it
	// accepts over the wire; this is only the file.)
	var stored []Profile
	if err := json.Unmarshal(data, &stored, json.RejectUnknownMembers(true)); err != nil {
		return fmt.Errorf("parse %s: %w", abs, err)
	}

	// A slice cannot enforce unique names the way map keys did, so the check is
	// explicit now. It is worth keeping: "name" is what the API is keyed by and
	// what /profile/<name> resolves, so a duplicate makes one of the pair
	// unreachable and deletable only by deleting the other.
	seen := make(map[string]struct{}, len(stored))
	profiles := make([]Profile, 0, len(stored))
	for i, p := range stored {
		if p.Name == "" {
			return fmt.Errorf("parse %s: profile at index %d has an empty name", abs, i)
		}
		if _, duplicate := seen[p.Name]; duplicate {
			return fmt.Errorf("parse %s: more than one profile is named %q", abs, p.Name)
		}
		seen[p.Name] = struct{}{}
		profiles = append(profiles, normalized(p))
	}

	ps.store = profiles
	return nil
}

// Save writes the current profiles to the file Load resolved.
func (ps *ProfileStore) Save() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	return ps.saveLocked()
}

// Atomic save out to a file. Called must hold a lock, this function assumes all data is immutable during the call
func (ps *ProfileStore) saveLocked() error {
	if ps.path == "" {
		return errors.New("profile store has no file; Load was never called")
	}

	data, err := json.Marshal(ps.store, jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("encode profiles: %w", err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}

	dir := filepath.Dir(ps.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	// Atomic save: temp file -> rename/overwrite
	tmp, err := os.CreateTemp(dir, ".profiles-*.json")
	if err != nil {
		return fmt.Errorf("create temporary file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, ps.path); err != nil {
		return fmt.Errorf("replace %s: %w", ps.path, err)
	}
	return nil
}

// Returns a copy of the profiles, in display order. O(n), not reference
func (ps *ProfileStore) Get() []Profile {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	copy := make([]Profile, 0, len(ps.store))
	for _, p := range ps.store {
		copy = append(copy, normalized(p))
	}

	return copy
}

// Position of the named profile, -1 if absent. Caller must hold the lock.
func (ps *ProfileStore) indexOfLocked(name string) int {
	return slices.IndexFunc(ps.store, func(p Profile) bool { return p.Name == name })
}

// Creates or replaces a profile. Rollback on failure.
// An existing profile keeps its position, new - at the end.
func (ps *ProfileStore) Upsert(profile Profile) error {
	if profile.Name == "" {
		return errors.New("profile name is empty")
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	i := ps.indexOfLocked(profile.Name)
	if i < 0 {
		ps.store = append(ps.store, normalized(profile))
		if err := ps.saveLocked(); err != nil {
			ps.store = ps.store[:len(ps.store)-1]
			return err
		}
		return nil
	}

	previous := ps.store[i]
	ps.store[i] = normalized(profile)
	if err := ps.saveLocked(); err != nil {
		ps.store[i] = previous
		return err
	}
	return nil
}

// If not found - no-op
func (ps *ProfileStore) Delete(profileName string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	i := ps.indexOfLocked(profileName)
	if i < 0 {
		return nil
	}

	previous := ps.store[i]
	ps.store = slices.Delete(ps.store, i, i+1)

	if err := ps.saveLocked(); err != nil {
		ps.store = slices.Insert(ps.store, i, previous)
		return err
	}
	return nil
}

// Names must be exactly the stored names. ErrOrderMismatch if mismatch
func (ps *ProfileStore) Reorder(names []string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if len(names) != len(ps.store) {
		return fmt.Errorf("%w: got %d names for %d profiles", ErrOrderMismatch, len(names), len(ps.store))
	}

	reordered := make([]Profile, 0, len(names))
	taken := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, duplicate := taken[name]; duplicate {
			return fmt.Errorf("%w: %q listed more than once", ErrOrderMismatch, name)
		}
		i := ps.indexOfLocked(name)
		if i < 0 {
			return fmt.Errorf("%w: no profile named %q", ErrOrderMismatch, name)
		}
		taken[name] = struct{}{}
		reordered = append(reordered, ps.store[i])
	}

	previous := ps.store
	ps.store = reordered

	if err := ps.saveLocked(); err != nil {
		ps.store = previous
		return err
	}
	return nil
}

// Checks and fixes missing or nil fields
func normalized(p Profile) Profile {
	if p.Texts == nil {
		p.Texts = []string{}
	} else {
		p.Texts = slices.Clone(p.Texts)
	}
	return p
}
