package tui_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/palette"
	"github.com/SantosJMM/herdr-human/internal/tui"
)

// fakeKeys simula el servicio de atajos y anota lo que se le pide.
type fakeKeys struct {
	bound    map[string]string // id de la entrada -> atajo que ya tiene
	cannot   map[string]bool   // entradas que no se pueden enlazar
	conflict map[string]string // tecla -> motivo que impide usarla
	warn     map[string]string // tecla -> aviso
	saveErr  error
	saved    []string // "id=tecla"
	removed  []string
}

func newFakeKeys() *fakeKeys {
	return &fakeKeys{bound: map[string]string{}, cannot: map[string]bool{}, conflict: map[string]string{}, warn: map[string]string{}}
}

func (f *fakeKeys) Bindable(e palette.Entry) (string, bool) { return f.bound[e.ID], !f.cannot[e.ID] }

func (f *fakeKeys) Check(_ palette.Entry, key string) tui.KeyCheck {
	k := strings.ToLower(strings.TrimSpace(key))
	switch {
	case !strings.Contains(k, "+") && len(k) == 1:
		return tui.KeyCheck{Key: k, Problem: k + " is a plain key"}
	case f.conflict[k] != "":
		return tui.KeyCheck{Key: k, Problem: f.conflict[k]}
	}
	return tui.KeyCheck{Key: k, Warning: f.warn[k]}
}

func (f *fakeKeys) Save(_ context.Context, e palette.Entry, key string) (tui.KeySaved, error) {
	if f.saveErr != nil {
		return tui.KeySaved{}, f.saveErr
	}
	f.saved = append(f.saved, e.ID+"="+key)
	f.bound[e.ID] = key
	return tui.KeySaved{Key: key}, nil
}

func (f *fakeKeys) Remove(_ context.Context, e palette.Entry) (tui.KeySaved, error) {
	f.removed = append(f.removed, e.ID)
	delete(f.bound, e.ID)
	return tui.KeySaved{}, nil
}

func keyPalette(t *testing.T, k *fakeKeys, entries ...palette.Entry) tui.PaletteModel {
	t.Helper()
	if len(entries) == 0 {
		entries = []palette.Entry{described("pane.close", "Pane: Close current", "Close it."), described("tab.new", "Tab: New", "New tab.")}
	}
	return startPalette(t, tui.PaletteOptions{Entries: entries, Keys: k})
}

// press envía una tecla y, si pide trabajo en segundo plano (guardar), lo ejecuta
// y entrega su resultado, como haría el programa.
func press(t *testing.T, m tui.PaletteModel, msgs ...tea.Msg) tui.PaletteModel {
	t.Helper()
	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		m = next.(tui.PaletteModel)
		for cmd != nil {
			result := cmd()
			if result == nil {
				break
			}
			if _, quit := result.(tea.QuitMsg); quit {
				break
			}
			next, cmd = m.Update(result)
			m = next.(tui.PaletteModel)
		}
	}
	return m
}

var (
	ctrlK    = tea.KeyMsg{Type: tea.KeyCtrlK}
	enter    = tea.KeyMsg{Type: tea.KeyEnter}
	esc      = tea.KeyMsg{Type: tea.KeyEsc}
	tab      = tea.KeyMsg{Type: tea.KeyTab}
	deleteK  = tea.KeyMsg{Type: tea.KeyDelete}
	ctrlT    = tea.KeyMsg{Type: tea.KeyCtrlT}
	ctrlAltK = tea.KeyMsg{Type: tea.KeyCtrlK, Alt: true}
)

func TestCtrlKOpensTheShortcutScreenForTheSelectedRow(t *testing.T) {
	k := newFakeKeys()
	k.bound["pane.close"] = "f2"
	m := press(t, keyPalette(t, k), ctrlK)
	view := m.View()
	for _, want := range []string{"Pane: Close current", "now: f2", "Press the shortcut you want", "(prefix: off)", "tab prefix", "ctrl+t type it", "delete remove"} {
		if !strings.Contains(view, want) {
			t.Errorf("the screen shows the entry, its current shortcut and the keys; lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Tab: New") {
		t.Errorf("the list is replaced by the shortcut screen:\n%s", view)
	}
}

func TestPressingACombinationShowsItAndEnterSavesIt(t *testing.T) {
	k := newFakeKeys()
	m := press(t, keyPalette(t, k), ctrlK, ctrlAltK)
	if v := m.View(); !strings.Contains(v, "new: ctrl+alt+k") || !strings.Contains(v, "enter save") {
		t.Fatalf("the captured combination is shown, in herdr's spelling:\n%s", v)
	}
	m = press(t, m, enter)
	if len(k.saved) != 1 || k.saved[0] != "pane.close=ctrl+alt+k" {
		t.Fatalf("saved %v", k.saved)
	}
	view := m.View()
	if !strings.Contains(view, "Shortcut ctrl+alt+k saved for Pane: Close current.") {
		t.Errorf("back on the list with the result:\n%s", view)
	}
	if !strings.Contains(view, "Pane: Close current") || !strings.Contains(view, "Tab: New") || !strings.Contains(view, "ctrl+alt+k") {
		t.Errorf("the list is back and the row shows its new shortcut:\n%s", view)
	}
}

func TestTheResultNoticeLastsOnlyUntilTheNextKey(t *testing.T) {
	m := press(t, keyPalette(t, newFakeKeys()), ctrlK, ctrlAltK, enter)
	if !strings.Contains(m.View(), "saved for") {
		t.Fatal("setup: the notice is there")
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if strings.Contains(m.View(), "saved for") {
		t.Errorf("the next key clears the notice:\n%s", m.View())
	}
}

func TestEscClosesOnlyTheShortcutScreen(t *testing.T) {
	k := newFakeKeys()
	m := press(t, keyPalette(t, k), ctrlK, ctrlAltK, esc)
	if !strings.Contains(m.View(), "Tab: New") || len(k.saved) != 0 {
		t.Errorf("esc goes back to the list and saves nothing:\n%s", m.View())
	}
	if _, cmd := m.Update(esc); cmd == nil {
		t.Error("a second esc closes the palette as usual")
	}
}

func TestAProblemWithTheKeyBlocksSavingAndSaysWhy(t *testing.T) {
	k := newFakeKeys()
	k.conflict["f1"] = "Open palette already uses f1 in config.toml; free it first"
	m := press(t, keyPalette(t, k), ctrlK, tea.KeyMsg{Type: tea.KeyF1})
	view := m.View()
	if !strings.Contains(view, "✗ Open palette already uses f1") || strings.Contains(view, "enter save") {
		t.Fatalf("the reason is shown and saving is not offered:\n%s", view)
	}
	press(t, m, enter)
	if len(k.saved) != 0 {
		t.Errorf("Enter must not save a key that has a problem: %v", k.saved)
	}
}

func TestAWarningNeedsASecondEnter(t *testing.T) {
	k := newFakeKeys()
	k.warn["prefix+n"] = "prefix+n is herdr's default for keys.next_tab; herdr may keep either"
	m := press(t, keyPalette(t, k), ctrlK, tab, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if v := m.View(); !strings.Contains(v, "new: prefix+n") || !strings.Contains(v, "⚠ prefix+n is herdr's default") {
		t.Fatalf("the warning is shown next to the key:\n%s", v)
	}
	m = press(t, m, enter)
	if len(k.saved) != 0 || !strings.Contains(m.View(), "enter again to bind it anyway") {
		t.Fatalf("the first Enter only asks again; saved %v\n%s", k.saved, m.View())
	}
	press(t, m, enter)
	if len(k.saved) != 1 || k.saved[0] != "pane.close=prefix+n" {
		t.Errorf("the second Enter binds it: %v", k.saved)
	}
}

func TestTabTogglesThePrefixBeforeAndAfterPressingAKey(t *testing.T) {
	k := newFakeKeys()
	a := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}
	m := press(t, keyPalette(t, k), ctrlK, a)
	if v := m.View(); !strings.Contains(v, "✗ a is a plain key") {
		t.Fatalf("a plain key without the prefix is refused:\n%s", v)
	}
	m = press(t, m, tab)
	if v := m.View(); !strings.Contains(v, "new: prefix+a") || strings.Contains(v, "plain key") {
		t.Errorf("Tab adds the prefix to what was pressed and it becomes valid:\n%s", v)
	}
	m = press(t, m, tab)
	if v := m.View(); !strings.Contains(v, "new: a") || !strings.Contains(v, "plain key") {
		t.Errorf("Tab again takes it off:\n%s", v)
	}
}

func TestTypingTheCombinationByHandIsAlwaysPossible(t *testing.T) {
	k := newFakeKeys()
	m := press(t, keyPalette(t, k), ctrlK, ctrlT)
	if v := m.View(); !strings.Contains(v, "type it:") || !strings.Contains(v, "enter use it") {
		t.Fatalf("%s", v)
	}
	m = press(t, m, typeRunes("prefix+alt+g"), enter)
	if v := m.View(); !strings.Contains(v, "new: prefix+alt+g") {
		t.Fatalf("what was typed becomes the candidate:\n%s", v)
	}
	press(t, m, enter)
	if len(k.saved) != 1 || k.saved[0] != "pane.close=prefix+alt+g" {
		t.Errorf("saved %v", k.saved)
	}
}

func TestEscInsideTypingGoesBackToPressing(t *testing.T) {
	m := press(t, keyPalette(t, newFakeKeys()), ctrlK, ctrlT, esc)
	if !strings.Contains(m.View(), "Press the shortcut you want") {
		t.Errorf("esc leaves the typing field but not the screen:\n%s", m.View())
	}
}

func TestDeleteRemovesTheCurrentShortcutAfterConfirming(t *testing.T) {
	k := newFakeKeys()
	k.bound["pane.close"] = "f2"
	m := press(t, keyPalette(t, k), ctrlK, deleteK)
	if v := m.View(); !strings.Contains(v, "Remove the shortcut f2?") || !strings.Contains(v, "enter removes it") {
		t.Fatalf("it asks first:\n%s", v)
	}
	if len(k.removed) != 0 {
		t.Fatal("nothing is removed before confirming")
	}
	m = press(t, m, enter)
	if len(k.removed) != 1 || k.removed[0] != "pane.close" {
		t.Fatalf("removed %v", k.removed)
	}
	if v := m.View(); !strings.Contains(v, "Shortcut removed from Pane: Close current.") {
		t.Errorf("%s", v)
	}
}

func TestDeleteWithNothingToRemoveSaysSo(t *testing.T) {
	k := newFakeKeys()
	m := press(t, keyPalette(t, k), ctrlK, deleteK)
	if v := m.View(); !strings.Contains(v, "it has no shortcut to remove") || len(k.removed) != 0 {
		t.Errorf("%s", v)
	}
}

func TestACancelledRemovalChangesNothing(t *testing.T) {
	k := newFakeKeys()
	k.bound["pane.close"] = "f2"
	m := press(t, keyPalette(t, k), ctrlK, deleteK, esc)
	if len(k.removed) != 0 || !strings.Contains(m.View(), "Tab: New") {
		t.Errorf("esc cancels: removed %v\n%s", k.removed, m.View())
	}
}

func TestASaveFailureKeepsTheScreenSoItCanBeFixed(t *testing.T) {
	k := newFakeKeys()
	k.saveErr = errors.New("herdr would not accept it, so nothing was changed: invalid keybinding")
	m := press(t, keyPalette(t, k), ctrlK, ctrlAltK, enter)
	view := m.View()
	if !strings.Contains(view, "✗ herdr would not accept it") || !strings.Contains(view, "new: ctrl+alt+k") {
		t.Fatalf("the error is shown and the screen stays:\n%s", view)
	}
	k.saveErr = nil
	press(t, m, enter)
	if len(k.saved) != 1 {
		t.Errorf("after fixing the cause, Enter tries again: %v", k.saved)
	}
}

func TestEntriesThatCannotBeBoundSayWhyInsteadOfOpeningTheScreen(t *testing.T) {
	k := newFakeKeys()
	k.cannot["pane.close"] = true
	m := press(t, keyPalette(t, k), ctrlK)
	view := m.View()
	if !strings.Contains(view, "cannot have a shortcut") || !strings.Contains(view, "Tab: New") {
		t.Errorf("the list stays and a notice explains:\n%s", view)
	}
}

func TestWithoutAKeyServiceCtrlKDoesNothingAndIsNotAdvertised(t *testing.T) {
	m := startPalette(t, tui.PaletteOptions{Entries: []palette.Entry{described("a", "Alpha", "d")}})
	if strings.Contains(m.View(), "ctrl+k") {
		t.Errorf("not advertised without the service:\n%s", m.View())
	}
	m = press(t, m, ctrlK)
	if !strings.Contains(m.View(), "Alpha") || strings.Contains(m.View(), "shortcut") {
		t.Errorf("ctrl+k does nothing:\n%s", m.View())
	}
	with := keyPalette(t, newFakeKeys())
	if !strings.Contains(with.View(), "ctrl+k shortcut") {
		t.Errorf("advertised with the service:\n%s", with.View())
	}
}

func TestReservedKeysAreNotCapturedAsShortcuts(t *testing.T) {
	k := newFakeKeys()
	m := press(t, keyPalette(t, k), ctrlK, deleteK, ctrlT, esc)
	if strings.Contains(m.View(), "new:") {
		t.Errorf("delete, ctrl+t and esc drive the screen; they are not shortcuts:\n%s", m.View())
	}
}

func TestPressedKeysAreWrittenTheWayHerdrSpellsThem(t *testing.T) {
	for name, tt := range map[string]struct {
		msg  tea.KeyMsg
		want string
	}{
		"ctrl+letter":   {tea.KeyMsg{Type: tea.KeyCtrlG}, "new: ctrl+g"},
		"alt+arrow":     {tea.KeyMsg{Type: tea.KeyLeft, Alt: true}, "new: alt+left"},
		"shift+arrow":   {tea.KeyMsg{Type: tea.KeyShiftUp}, "new: shift+up"},
		"ctrl+arrow":    {tea.KeyMsg{Type: tea.KeyCtrlRight}, "new: ctrl+right"},
		"function key":  {tea.KeyMsg{Type: tea.KeyF5}, "new: f5"},
		"alt+f key":     {tea.KeyMsg{Type: tea.KeyF5, Alt: true}, "new: alt+f5"},
		"alt+letter":    {tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true}, "new: alt+x"},
		"alt+shift+a":   {tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A"), Alt: true}, "new: alt+shift+a"},
		"alt+minus":     {tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-"), Alt: true}, "new: alt+minus"},
		"alt+bracket":   {tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]"), Alt: true}, "new: alt+]"},
		"alt+space":     {tea.KeyMsg{Type: tea.KeySpace, Alt: true}, "new: alt+space"},
		"page up":       {tea.KeyMsg{Type: tea.KeyPgUp, Alt: true}, "new: alt+pageup"},
		"shift+tab":     {tea.KeyMsg{Type: tea.KeyShiftTab}, "new: shift+tab"},
		"alt+enter":     {tea.KeyMsg{Type: tea.KeyEnter, Alt: true}, "new: alt+enter"},
		"ctrl+shift+up": {tea.KeyMsg{Type: tea.KeyCtrlShiftUp}, "new: ctrl+shift+up"},
	} {
		view := press(t, keyPalette(t, newFakeKeys()), ctrlK, tt.msg).View()
		if !strings.Contains(view, tt.want) {
			t.Errorf("%s: want %q in:\n%s", name, tt.want, view)
		}
	}
}

func TestKeysThatCannotBeToldApartExplainAndLeaveTheScreenWaiting(t *testing.T) {
	for name, msg := range map[string]tea.KeyMsg{
		"ctrl+h is backspace": {Type: tea.KeyCtrlH},
		"a paste":             {Type: tea.KeyRunes, Runes: []rune("abc")},
	} {
		view := press(t, keyPalette(t, newFakeKeys()), ctrlK, msg).View()
		if !strings.Contains(view, "ctrl+t lets you type it") || !strings.Contains(view, "Press the shortcut you want") {
			t.Errorf("%s: it says how to type it by hand and keeps waiting:\n%s", name, view)
		}
	}
}

func TestTheShortcutScreenKeepsItsWidthWithoutAFrame(t *testing.T) {
	o := tui.PaletteOptions{Entries: manyEntries(3), Embedded: true, Keys: newFakeKeys()}
	o.Style = plain()
	next, _ := tui.NewPalette(o).Update(tea.WindowSizeMsg{Width: 50, Height: 14})
	m := press(t, next.(tui.PaletteModel), ctrlK, ctrlAltK)
	for i, line := range strings.Split(m.View(), "\n") {
		if got := len([]rune(line)); got != 50 {
			t.Errorf("line %d is %d cells wide, want 50: %q", i, got, line)
		}
	}
	if hasBox(m.View()) {
		t.Errorf("embedded: no frame on this screen either:\n%s", m.View())
	}
}

func TestTheRowKeepsShowingItsNewShortcutAfterTheNoticeGoes(t *testing.T) {
	m := press(t, keyPalette(t, newFakeKeys()), ctrlK, ctrlAltK, enter)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	view := m.View()
	if strings.Contains(view, "saved for") || !strings.Contains(view, "ctrl+alt+k") {
		t.Errorf("the notice is gone but the shortcut stays in the row's column:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Pane: Close current") && !strings.Contains(line, "ctrl+alt+k") {
			t.Errorf("the saved shortcut is on the row of its entry: %q", line)
		}
	}
}
