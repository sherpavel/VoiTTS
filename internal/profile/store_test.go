package profile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func tempStorePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "sub", "profiles.json")
}

func writeStoreFile(t *testing.T, contents string) string {
	t.Helper()
	path := tempStorePath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// loaded returns a store holding the named profiles, in the order given.
func loaded(t *testing.T, names ...string) (*ProfileStore, string) {
	t.Helper()
	path := tempStorePath(t)
	ps := NewStore()
	if err := ps.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, name := range names {
		if err := ps.Upsert(Profile{Name: name, DisplayName: name, Texts: []string{name + " text"}}); err != nil {
			t.Fatalf("Upsert %s: %v", name, err)
		}
	}
	return ps, path
}

func order(ps *ProfileStore) []string {
	var names []string
	for _, p := range ps.Get() {
		names = append(names, p.Name)
	}
	return names
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestUpsertAndDeletePersist(t *testing.T) {
	ps, path := loaded(t, "standup", "meeting")

	if err := ps.Delete("meeting"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	reloaded := NewStore()
	if err := reloaded.Load(path); err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := reloaded.Get()
	if len(got) != 1 || got[0].Name != "standup" {
		t.Fatalf("after reload the store holds %v, want just standup", order(reloaded))
	}
	if got[0].Texts[0] != "standup text" {
		t.Errorf("text round-tripped as %q", got[0].Texts[0])
	}
}

// Order is the whole point of the slice: it must survive a save and reload
// unsorted, which is what the map could not do.
func TestOrderSurvivesReload(t *testing.T) {
	ps, path := loaded(t, "zulu", "alpha", "mike", "bravo")
	want := []string{"zulu", "alpha", "mike", "bravo"}

	if got := order(ps); !equal(got, want) {
		t.Fatalf("in memory the order is %v, want %v", got, want)
	}

	reloaded := NewStore()
	if err := reloaded.Load(path); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := order(reloaded); !equal(got, want) {
		t.Errorf("after reload the order is %v, want %v", got, want)
	}
}

func TestSaveIsStable(t *testing.T) {
	ps, path := loaded(t, "zulu", "alpha", "mike")

	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if err := ps.Save(); err != nil {
			t.Fatalf("Save: %v", err)
		}
		again, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(first) {
			t.Fatalf("re-saving an unchanged store rewrote the file:\n%s\nwant:\n%s", again, first)
		}
	}
}

func TestUpsertKeepsPosition(t *testing.T) {
	ps, _ := loaded(t, "one", "two", "three")

	if err := ps.Upsert(Profile{Name: "two", DisplayName: "Edited", Texts: []string{"new"}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	want := []string{"one", "two", "three"}
	if got := order(ps); !equal(got, want) {
		t.Errorf("editing a profile moved it: order is %v, want %v", got, want)
	}
	if got := ps.Get()[1].DisplayName; got != "Edited" {
		t.Errorf("edit did not apply, display name is %q", got)
	}
}

func TestUpsertAppendsNewToEnd(t *testing.T) {
	ps, _ := loaded(t, "one", "two")

	if err := ps.Upsert(Profile{Name: "three", DisplayName: "Three"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	want := []string{"one", "two", "three"}
	if got := order(ps); !equal(got, want) {
		t.Errorf("order is %v, want %v", got, want)
	}
}

func TestDeleteKeepsOrder(t *testing.T) {
	ps, _ := loaded(t, "one", "two", "three", "four")

	if err := ps.Delete("two"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	want := []string{"one", "three", "four"}
	if got := order(ps); !equal(got, want) {
		t.Errorf("order is %v, want %v", got, want)
	}
}

func TestDeleteMissingIsNotAnError(t *testing.T) {
	ps, _ := loaded(t)
	if err := ps.Delete("nope"); err != nil {
		t.Errorf("Delete of a missing profile: %v", err)
	}
}

func TestUpsertRejectsEmptyName(t *testing.T) {
	ps, _ := loaded(t)
	if err := ps.Upsert(Profile{DisplayName: "Nameless"}); err == nil {
		t.Error("Upsert accepted a profile without a name")
	}
}

func TestReorder(t *testing.T) {
	ps, path := loaded(t, "one", "two", "three")

	if err := ps.Reorder([]string{"three", "one", "two"}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	want := []string{"three", "one", "two"}
	if got := order(ps); !equal(got, want) {
		t.Fatalf("order is %v, want %v", got, want)
	}

	reloaded := NewStore()
	if err := reloaded.Load(path); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := order(reloaded); !equal(got, want) {
		t.Errorf("reorder did not persist: order is %v, want %v", got, want)
	}
}

func TestReorderRejectsNonPermutations(t *testing.T) {
	cases := map[string][]string{
		"too few":   {"one", "two"},
		"too many":  {"one", "two", "three", "four"},
		"duplicate": {"one", "one", "two"},
		"unknown":   {"one", "two", "nope"},
	}
	for label, names := range cases {
		t.Run(label, func(t *testing.T) {
			ps, _ := loaded(t, "one", "two", "three")
			before := order(ps)

			err := ps.Reorder(names)
			if !errors.Is(err, ErrOrderMismatch) {
				t.Fatalf("Reorder(%v) returned %v, want ErrOrderMismatch", names, err)
			}
			if got := order(ps); !equal(got, before) {
				t.Errorf("a rejected reorder still changed the order to %v", got)
			}
		})
	}
}

func TestLoadNormalizesAbsentTexts(t *testing.T) {
	path := writeStoreFile(t, `[
  {"name": "work", "displayName": "Work"}
]`)

	ps := NewStore()
	if err := ps.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ps.Get()[0].Texts == nil {
		t.Error("absent texts stayed nil, want an empty slice so it encodes as []")
	}
}

func TestLoadReportsWhereParsingFailed(t *testing.T) {
	path := writeStoreFile(t, "[\n  {\n    \"name\": \"work\",\n  }\n]\n")

	err := NewStore().Load(path)
	if err == nil {
		t.Fatal("Load accepted a file with a trailing comma")
	}
	// v2 points at the comma itself, not the brace that trips over it below.
	if !strings.Contains(err.Error(), "offset 24") {
		t.Errorf("error does not locate the stray comma: %v", err)
	}
	if !strings.Contains(err.Error(), `"/0"`) {
		t.Errorf("error does not say which profile: %v", err)
	}
}

func TestLoadRejectsDuplicateNames(t *testing.T) {
	path := writeStoreFile(t, `[
  {"name": "work", "displayName": "First"},
  {"name": "work", "displayName": "Second"}
]`)
	err := NewStore().Load(path)
	if err == nil {
		t.Fatal("Load accepted the same profile name twice")
	}
	if !strings.Contains(err.Error(), `named "work"`) {
		t.Errorf("error does not name the duplicate: %v", err)
	}
}

func TestLoadRejectsEmptyName(t *testing.T) {
	path := writeStoreFile(t, `[
  {"name": "", "displayName": "Nameless"}
]`)
	err := NewStore().Load(path)
	if err == nil {
		t.Fatal("Load accepted a profile with an empty name")
	}
	if !strings.Contains(err.Error(), "index 0") {
		t.Errorf("error does not say which profile: %v", err)
	}
}

func TestLoadRejectsUnknownMembers(t *testing.T) {
	path := writeStoreFile(t, `[
  {"name": "work", "dispalyName": "typo"}
]`)
	err := NewStore().Load(path)
	if err == nil {
		t.Fatal("Load accepted a misspelled member name")
	}
	if !strings.Contains(err.Error(), "dispalyName") {
		t.Errorf("error does not name the offending member: %v", err)
	}
}

func TestLoadErrorNamesTheValue(t *testing.T) {
	path := writeStoreFile(t, `[
  {
    "name": "work",
    "texts": ["fine", 42]
  }
]`)
	err := NewStore().Load(path)
	if err == nil {
		t.Fatal("Load accepted a number in texts")
	}
	if !strings.Contains(err.Error(), "/0/texts/1") {
		t.Errorf("error does not point at the bad element: %v", err)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	path := writeStoreFile(t, "\n  \n")

	ps := NewStore()
	if err := ps.Load(path); err != nil {
		t.Fatalf("Load of an emptied file: %v", err)
	}
	if got := len(ps.Get()); got != 0 {
		t.Errorf("loaded %d profiles from an empty file, want 0", got)
	}
}

func TestGetIsACopy(t *testing.T) {
	ps, _ := loaded(t, "keep", "drop")

	snapshot := ps.Get()
	snapshot = snapshot[:1]
	snapshot[0].Texts[0] = "mutated"

	current := ps.Get()
	if len(current) != 2 {
		t.Errorf("reslicing a Get result changed the store to %v", order(ps))
	}
	if current[0].Texts[0] == "mutated" {
		t.Error("writing to a Get result's texts reached the store")
	}
}

// The texts handed to Upsert stay the caller's; the store must not alias them.
func TestUpsertDoesNotAliasCallerTexts(t *testing.T) {
	ps, _ := loaded(t)

	texts := []string{"original"}
	if err := ps.Upsert(Profile{Name: "work", DisplayName: "Work", Texts: texts}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	texts[0] = "mutated"

	if got := ps.Get()[0].Texts[0]; got != "original" {
		t.Errorf("store reads %q after the caller mutated its own slice, want %q", got, "original")
	}
}

func TestConcurrentEditsStayConsistent(t *testing.T) {
	ps, path := loaded(t)

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			p := Profile{Name: "p" + string(rune('a'+i)), DisplayName: "P", Texts: []string{"x"}}
			if err := ps.Upsert(p); err != nil {
				t.Errorf("Upsert: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			ps.Get()
		}()
	}
	wg.Wait()

	// Whatever the interleaving, the file parses and matches memory.
	reloaded := NewStore()
	if err := reloaded.Load(path); err != nil {
		t.Fatalf("reload after concurrent edits: %v", err)
	}
	if !equal(order(reloaded), order(ps)) {
		t.Errorf("file holds %v, memory holds %v", order(reloaded), order(ps))
	}
}

func TestSaveWithoutLoad(t *testing.T) {
	if err := NewStore().Upsert(Profile{Name: "x"}); err == nil {
		t.Error("Upsert succeeded on a store that was never loaded")
	}
}
