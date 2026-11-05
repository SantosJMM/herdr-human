package keybind

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeHerdr simula `herdr config check` y `server reload-config`.
type fakeHerdr struct {
	checked  []string // el texto de cada archivo que se pidió validar
	reloads  int
	reloadOK bool
	// issuesFor devuelve la salida de `config check` para un texto.
	issuesFor func(text string) string
	checkErr  error
}

func (f *fakeHerdr) check(_ context.Context, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	f.checked = append(f.checked, string(data))
	if f.checkErr != nil {
		return "", f.checkErr
	}
	if f.issuesFor != nil {
		return f.issuesFor(string(data)), nil
	}
	return "config: ok\n", nil
}

func (f *fakeHerdr) reload(context.Context) error {
	f.reloads++
	if !f.reloadOK {
		return errors.New("server not running")
	}
	return nil
}

func newStore(t *testing.T, initial string) (Store, *fakeHerdr, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if initial != "" {
		if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := &fakeHerdr{reloadOK: true}
	day := time.Date(2026, 11, 5, 14, 30, 0, 0, time.UTC)
	return Store{Path: path, Check: h.check, Reload: h.reload, Now: func() time.Time { return day }}, h, dir
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func listing(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestSaveWritesBacksUpAndReloads(t *testing.T) {
	s, h, dir := newStore(t, sample)
	res, err := s.Save(context.Background(), fresh, " Ctrl+Alt+K ", "Ping the thing")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Parse(sample).Set(fresh, "ctrl+alt+k", "Ping the thing")
	if got := read(t, s.Path); got != want {
		t.Errorf("the file holds the new block, normalized:\n%s", got)
	}
	if !res.Changed || !res.Reloaded || h.reloads != 1 {
		t.Errorf("result = %+v, reloads = %d", res, h.reloads)
	}
	if res.Backup != s.Path+".bak-hum-20261105" || read(t, res.Backup) != sample {
		t.Errorf("the backup is the file as it was, named by the day: %q", res.Backup)
	}
	if names := listing(t, dir); len(names) != 2 {
		t.Errorf("only the config and the backup remain (no temp files): %v", names)
	}
}

func TestTheChangeIsValidatedOnACopyAndTheOriginalIsCheckedAsABaseline(t *testing.T) {
	s, h, _ := newStore(t, sample)
	if _, err := s.Save(context.Background(), fresh, "f4", ""); err != nil {
		t.Fatal(err)
	}
	if len(h.checked) != 2 || h.checked[0] != sample || !strings.Contains(h.checked[1], `key = "f4"`) {
		t.Errorf("herdr checks the file as it was and as it would be, before anything is written; checked %d texts", len(h.checked))
	}
}

func TestOnlyTheFirstChangeOfTheDayMakesABackup(t *testing.T) {
	s, _, _ := newStore(t, sample)
	first, _ := s.Save(context.Background(), fresh, "f4", "")
	second, err := s.Save(context.Background(), pick, "f5", "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Backup == "" || second.Backup != "" {
		t.Errorf("backups: first %q second %q", first.Backup, second.Backup)
	}
	if read(t, first.Backup) != sample {
		t.Error("the day's backup keeps the file as it was before the day's first change")
	}
	s.Now = func() time.Time { return time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC) }
	third, _ := s.Save(context.Background(), palette1, "f6", "")
	if !strings.HasSuffix(third.Backup, ".bak-hum-20261106") {
		t.Errorf("a new day, a new backup: %q", third.Backup)
	}
}

func TestAChangeHerdrRejectsIsNeverWritten(t *testing.T) {
	s, h, dir := newStore(t, sample)
	h.issuesFor = func(text string) string {
		if strings.Contains(text, `"zz"`) {
			return "config: issues found\ninvalid keybinding: keys.command[4].key = \"zz\"; disabling binding\n"
		}
		return "config: ok\n"
	}
	res, err := s.Save(context.Background(), fresh, "zz", "")
	var rejected *RejectedError
	if !errors.As(err, &rejected) || len(rejected.Issues) != 1 || !strings.Contains(rejected.Error(), "invalid keybinding") {
		t.Fatalf("want a RejectedError naming the problem, got %v", err)
	}
	if read(t, s.Path) != sample || h.reloads != 0 || res.Changed {
		t.Error("a rejected change leaves the file alone and does not reload")
	}
	if names := listing(t, dir); len(names) != 1 {
		t.Errorf("no backup and no temp files for a change that was not applied: %v", names)
	}
}

func TestADuplicateKeyHerdrWouldDisableIsRejected(t *testing.T) {
	s, h, _ := newStore(t, sample)
	h.issuesFor = func(text string) string {
		if strings.Count(text, `key = "f1"`) > 1 {
			return "config: issues found\nf1: kept keys.command[1].key, disabled keys.command[4].key\n"
		}
		return "config: ok\n"
	}
	_, err := s.Save(context.Background(), fresh, "f1", "")
	var rejected *RejectedError
	if !errors.As(err, &rejected) || !strings.Contains(rejected.Error(), "disabled") {
		t.Errorf("got %v", err)
	}
}

func TestProblemsTheFileAlreadyHadDoNotBlockAnUnrelatedChange(t *testing.T) {
	s, h, _ := newStore(t, sample)
	// El mismo aviso antes y después, con otro índice porque el bloque nuevo desplaza números.
	h.issuesFor = func(text string) string {
		if strings.Contains(text, `key = "f4"`) {
			return "config: issues found\nold problem: keys.command[7].key is odd\n"
		}
		return "config: issues found\nold problem: keys.command[3].key is odd\n"
	}
	if _, err := s.Save(context.Background(), fresh, "f4", ""); err != nil {
		t.Errorf("a pre-existing problem is not this change's fault: %v", err)
	}
}

func TestAReloadFailureDoesNotUndoTheSavedFile(t *testing.T) {
	s, h, _ := newStore(t, sample)
	h.reloadOK = false
	res, err := s.Save(context.Background(), fresh, "f4", "")
	if err != nil {
		t.Fatalf("the file is saved, so this is not an error: %v", err)
	}
	if res.Reloaded || res.ReloadErr == nil || !strings.Contains(read(t, s.Path), `key = "f4"`) {
		t.Errorf("saved but not reloaded, and it says why: %+v", res)
	}
}

func TestAskingForTheKeyThatIsAlreadyThereChangesNothing(t *testing.T) {
	s, h, dir := newStore(t, sample)
	res, err := s.Save(context.Background(), palette1, "f1", "")
	if err != nil || res.Changed || h.reloads != 0 || len(h.checked) != 0 || len(listing(t, dir)) != 1 {
		t.Errorf("no change, no check, no backup, no reload: %+v %v", res, err)
	}
}

func TestBadKeysNeverReachHerdr(t *testing.T) {
	s, h, _ := newStore(t, sample)
	for _, key := range []string{"", "wat+q", "ctrl+"} {
		if _, err := s.Save(context.Background(), fresh, key, ""); err == nil {
			t.Errorf("%q must be refused", key)
		}
	}
	if len(h.checked) != 0 {
		t.Error("a key that is invalid on its face is refused without asking herdr")
	}
}

func TestSaveCreatesTheConfigWhenThereIsNone(t *testing.T) {
	s, _, dir := newStore(t, "")
	res, err := s.Save(context.Background(), fresh, "f4", "")
	if err != nil || !res.Changed || res.Backup != "" {
		t.Fatalf("%+v %v", res, err)
	}
	if got := read(t, s.Path); !strings.HasPrefix(got, "[[keys.command]]\nkey = \"f4\"") {
		t.Errorf("got %q", got)
	}
	if len(listing(t, dir)) != 1 {
		t.Error("there was nothing to back up")
	}
}

func TestRemoveAppliesTheSameWay(t *testing.T) {
	s, h, _ := newStore(t, sample)
	res, err := s.Remove(context.Background(), palette1)
	if err != nil || !res.Changed || h.reloads != 1 || res.Backup == "" {
		t.Fatalf("%+v %v", res, err)
	}
	if Parse(read(t, s.Path)).KeyOf(palette1) != "" {
		t.Error("the shortcut is gone from the file")
	}
	if _, err := s.Remove(context.Background(), fresh); err != ErrNotBound {
		t.Errorf("removing what is not bound: %v", err)
	}
}

func TestWithoutAConfigPathOrACheckNothingIsWritten(t *testing.T) {
	if _, err := (Store{}).Save(context.Background(), fresh, "f4", ""); !errors.Is(err, ErrNoConfig) {
		t.Errorf("got %v", err)
	}
	s, _, _ := newStore(t, sample)
	s.Check = nil
	if _, err := s.Save(context.Background(), fresh, "f4", ""); err == nil || read(t, s.Path) != sample {
		t.Errorf("an unvalidated change is never applied: %v", err)
	}
}

func TestIfHerdrCannotBeRunTheFileIsLeftAlone(t *testing.T) {
	s, h, _ := newStore(t, sample)
	h.checkErr = errors.New("herdr not found")
	if _, err := s.Save(context.Background(), fresh, "f4", ""); err == nil || read(t, s.Path) != sample {
		t.Errorf("got %v", err)
	}
}

func TestIssuesAreComparedWithoutTheirIndexes(t *testing.T) {
	got := issues("config: issues found\nf1: kept keys.command[2].key, disabled keys.command[10].key\n\n")
	if len(got) != 1 || got[0] != "f1: kept keys.command[].key, disabled keys.command[].key" {
		t.Errorf("got %q", got)
	}
	if len(issues("config: ok\n")) != 0 {
		t.Error("a clean check has no issues")
	}
	added := newIssues([]string{"a", "b"}, []string{"b", "a", "a", "c"})
	if strings.Join(added, ",") != "a,c" {
		t.Errorf("issues are counted, not just listed: %v", added)
	}
}
