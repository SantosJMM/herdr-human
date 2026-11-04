package alias_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/alias"
)

func store(t *testing.T) alias.Store {
	t.Helper()
	return alias.Store{Path: filepath.Join(t.TempDir(), "conf", "aliases.json")}
}

func TestSetListAndRemove(t *testing.T) {
	t.Parallel()

	s := store(t)
	if got, err := s.List(); err != nil || len(got) != 0 {
		t.Fatalf("a missing file is an empty list: %v %v", got, err)
	}
	for name, target := range map[string]string{"docs": "odoous/odoo-dev", "api": "importer"} {
		if err := s.Set(name, target); err != nil {
			t.Fatalf("Set(%s): %v", name, err)
		}
	}
	got, err := s.List()
	if err != nil || got["docs"] != "odoous/odoo-dev" || got["api"] != "importer" || len(got) != 2 {
		t.Fatalf("List = %v, %v", got, err)
	}
	if err := s.Set("docs", "local/other"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.List(); got["docs"] != "local/other" {
		t.Errorf("Set must replace an existing alias: %v", got)
	}
	if err := s.Remove("docs"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.List(); len(got) != 1 || got["api"] == "" {
		t.Errorf("after Remove: %v", got)
	}
	if err := s.Remove("docs"); !errors.Is(err, alias.ErrNotFound) {
		t.Errorf("removing a missing alias = %v, want ErrNotFound", err)
	}
}

func TestSetRejectsInvalidNamesAndTargets(t *testing.T) {
	t.Parallel()

	s := store(t)
	for _, name := range []string{"", "Docs", "a b", "a/b", "-x", strings.Repeat("a", 33), "bad\x1b"} {
		if err := s.Set(name, "importer"); !errors.Is(err, alias.ErrInvalidName) {
			t.Errorf("name %q: err = %v, want ErrInvalidName", name, err)
		}
	}
	for _, target := range []string{"", "   ", "-x", "a\x1b[0m", "a\nb", strings.Repeat("a", 201)} {
		if err := s.Set("docs", target); !errors.Is(err, alias.ErrInvalidTarget) {
			t.Errorf("target %q: err = %v, want ErrInvalidTarget", target, err)
		}
	}
	if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
		t.Errorf("nothing valid was set, yet the file exists (%v)", err)
	}
}

func TestACorruptFileIsReportedAndNeverOverwritten(t *testing.T) {
	t.Parallel()

	s := store(t)
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	const junk = "{not json"
	if err := os.WriteFile(s.Path, []byte(junk), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err == nil {
		t.Error("List accepted a corrupt file")
	}
	if err := s.Set("docs", "importer"); err == nil {
		t.Error("Set overwrote a corrupt file")
	}
	if err := s.Remove("docs"); err == nil {
		t.Error("Remove accepted a corrupt file")
	}
	if data, _ := os.ReadFile(s.Path); string(data) != junk {
		t.Errorf("the corrupt file was modified: %q", data)
	}
}

func TestAHugeFileIsRejectedWithoutReadingItAll(t *testing.T) {
	t.Parallel()

	s := store(t)
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Path, []byte(`{"aliases":{"a":"`+strings.Repeat("x", 200<<10)+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err == nil {
		t.Error("an oversized file was accepted")
	}
}

func TestLookupIgnoresErrorsAndMissingNames(t *testing.T) {
	t.Parallel()

	s := store(t)
	if _, ok := s.Lookup("docs"); ok {
		t.Error("found an alias in an empty store")
	}
	if err := s.Set("docs", "odoous/odoo-dev"); err != nil {
		t.Fatal(err)
	}
	if target, ok := s.Lookup("docs"); !ok || target != "odoous/odoo-dev" {
		t.Errorf("Lookup = %q, %v", target, ok)
	}
	if _, ok := (alias.Store{}).Lookup("docs"); ok {
		t.Error("a store without path found something")
	}
}

func TestTheFileIsPrivate(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions do not apply on Windows")
	}
	s := store(t)
	if err := s.Set("docs", "importer"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.Path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, %v, want 0600", info.Mode().Perm(), err)
	}
}

func TestSetWithoutAPathFailsClearly(t *testing.T) {
	t.Parallel()

	if err := (alias.Store{}).Set("docs", "importer"); !errors.Is(err, alias.ErrNoStore) {
		t.Errorf("err = %v, want ErrNoStore", err)
	}
}
