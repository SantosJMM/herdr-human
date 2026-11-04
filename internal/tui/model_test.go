package tui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/tui"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

func inv() inventory.Inventory {
	return inventory.Inventory{
		Targets: []inventory.Target{
			{Workspace: "raw", PaneID: "wS:p1", Agent: "codex", Name: "importer", Status: "idle", Cwd: `C:\src\raw`},
			{Workspace: "raw", PaneID: "wS:p4", Agent: "claude", Name: "Pane ID work", Status: "working", Focused: true},
			{Workspace: "demo", PaneID: "wS:p7", Agent: "claude", Name: "Needs approval", Status: "blocked"},
			{Machine: "odoous", Workspace: "odoo-inc", PaneID: "w1:p3", Agent: "codex", Name: "Finished job", Status: "done"},
			{Machine: "odoous", Workspace: "odoo-inc", PaneID: "w1:p2", Agent: "codex", Name: "Analiza login | odoo-inc | 5h 8% left", Status: "blocked", Cwd: "/home/odoo"},
		},
		Timings: []inventory.Timing{
			{Machine: ""},
			{Machine: "odoous", Elapsed: 1300 * time.Millisecond},
			{Machine: "remoto2", Elapsed: 8 * time.Second, Err: context.DeadlineExceeded},
		},
	}
}

func plain() *ui.Style { return ui.NewStyle(ui.Capability{}, ui.Theme{}) }

func start(t *testing.T, style *ui.Style, w, h int) tui.Model {
	t.Helper()
	m := tui.New(tui.Options{Inventory: inv(), Style: style})
	return send(t, m, tea.WindowSizeMsg{Width: w, Height: h})
}

func send(t *testing.T, m tui.Model, msg tea.Msg) tui.Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(tui.Model)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeText(t *testing.T, m tui.Model, text string) tui.Model {
	t.Helper()
	for _, r := range text {
		m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func quits(m tui.Model, msg tea.Msg) bool {
	_, cmd := m.Update(msg)
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func breadcrumb(t *testing.T, m tui.Model) string {
	t.Helper()
	for _, l := range strings.Split(m.View(), "\n") {
		inner := strings.Trim(l, "│ ")
		if strings.Count(inner, " / ") == 2 {
			return inner
		}
	}
	t.Fatalf("no breadcrumb in:\n%s", m.View())
	return ""
}

func TestViewIsAFrameOfTheExactSize(t *testing.T) {
	t.Parallel()

	for _, size := range [][2]int{{80, 24}, {100, 30}, {40, 14}, {20, 10}} {
		m := start(t, plain(), size[0], size[1])
		lines := strings.Split(m.View(), "\n")
		if len(lines) != size[1] {
			t.Errorf("%dx%d: %d lines, want %d", size[0], size[1], len(lines), size[1])
		}
		for i, l := range lines {
			if w := ui.VisibleLen(l); w != size[0] {
				t.Errorf("%dx%d: line %d is %d cells wide: %q", size[0], size[1], i, w, l)
			}
		}
	}
}

func TestViewShowsTheTreeTheCountsAndTheMachineStatus(t *testing.T) {
	t.Parallel()

	view := start(t, plain(), 100, 30).View()
	for _, want := range []string{
		"┌ Talk to", "5 agents · 3 need you",
		"Local", "odoous", "1.3s", "remoto2", "✖ timeout 8.0s",
		" raw", " demo", " odoo-inc",
		"├─ ○ importer", "└─ ◆ ○ Pane ID work", "Analiza login", "blocked", "working",
		"└", "┘",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "5h 8% left") {
		t.Errorf("view shows the noisy Codex title segments:\n%s", view)
	}
}

func TestNeedsAttentionComesFirstInsideAWorkspace(t *testing.T) {
	t.Parallel()

	view := start(t, plain(), 100, 30).View()
	if strings.Index(view, "Analiza login") > strings.Index(view, "Finished job") {
		t.Errorf("the blocked agent should be listed before the done one:\n%s", view)
	}
}

func TestSelectionMovesAndStaysInsideTheList(t *testing.T) {
	t.Parallel()

	m := start(t, plain(), 100, 30)
	if got := breadcrumb(t, m); got != "local / raw / wS:p1" {
		t.Fatalf("initial breadcrumb = %q", got)
	}
	m = send(t, m, key("k"))
	if got := breadcrumb(t, m); got != "local / raw / wS:p1" {
		t.Errorf("k at the top moved to %q", got)
	}
	m = send(t, m, key("j"))
	if got := breadcrumb(t, m); got != "local / raw / wS:p4" {
		t.Errorf("after j: %q", got)
	}
	m = send(t, m, key("down"))
	if got := breadcrumb(t, m); got != "local / demo / wS:p7" {
		t.Errorf("after down: %q", got)
	}
	m = send(t, m, key("up"))
	if got := breadcrumb(t, m); got != "local / raw / wS:p4" {
		t.Errorf("after up: %q", got)
	}
	m = send(t, m, key("G"))
	if got := breadcrumb(t, m); got != "odoous / odoo-inc / w1:p3" {
		t.Errorf("after G: %q", got)
	}
	m = send(t, m, key("j"))
	if got := breadcrumb(t, m); got != "odoous / odoo-inc / w1:p3" {
		t.Errorf("j at the bottom moved to %q", got)
	}
	m = send(t, m, key("g"))
	if got := breadcrumb(t, m); got != "local / raw / wS:p1" {
		t.Errorf("after g: %q", got)
	}
}

func TestLeftAndRightJumpBetweenGroups(t *testing.T) {
	t.Parallel()

	m := start(t, plain(), 100, 30)
	m = send(t, m, key("right"))
	if got := breadcrumb(t, m); got != "local / demo / wS:p7" {
		t.Errorf("right: %q", got)
	}
	m = send(t, m, key("right"))
	if got := breadcrumb(t, m); got != "odoous / odoo-inc / w1:p2" {
		t.Errorf("right again: %q", got)
	}
	m = send(t, m, key("left"))
	if got := breadcrumb(t, m); got != "local / demo / wS:p7" {
		t.Errorf("left: %q", got)
	}
}

func TestFooterShowsThePathAndTheKeys(t *testing.T) {
	t.Parallel()

	view := start(t, plain(), 100, 30).View()
	for _, want := range []string{`C:\src\raw`, "search", "enter", "quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
}

func TestStatusFiltersAndAll(t *testing.T) {
	t.Parallel()

	m := send(t, start(t, plain(), 100, 30), key("b"))
	view := m.View()
	if !strings.Contains(view, "Needs approval") || !strings.Contains(view, "Analiza login") ||
		strings.Contains(view, "importer") || strings.Contains(view, "Finished job") {
		t.Errorf("filter b should keep only blocked agents:\n%s", view)
	}
	if !strings.Contains(view, "[blocked]") || !strings.Contains(view, "2 of 5 agents") {
		t.Errorf("filter b should be announced:\n%s", view)
	}

	for filter, want := range map[string]string{"w": "Pane ID work", "i": "importer", "d": "Finished job"} {
		got := send(t, m, key(filter)).View()
		if !strings.Contains(got, want) || strings.Contains(got, "Needs approval") {
			t.Errorf("filter %s:\n%s", filter, got)
		}
	}

	m = send(t, m, key("a"))
	if !strings.Contains(m.View(), "importer") || strings.Contains(m.View(), "[blocked]") {
		t.Errorf("filter a should show everything again:\n%s", m.View())
	}
}

func TestSearchFiltersAsYouType(t *testing.T) {
	t.Parallel()

	m := send(t, start(t, plain(), 100, 30), key("/"))
	m = typeText(t, m, "login")
	view := m.View()
	if !strings.Contains(view, "Analiza login") || strings.Contains(view, "importer") {
		t.Errorf("search login:\n%s", view)
	}
	if !strings.Contains(view, "login") || !strings.Contains(view, "1 of 5 agents") {
		t.Errorf("the query and the counter should be shown:\n%s", view)
	}

	// Los filtros b/w/i/d/a son texto mientras se busca.
	m = send(t, m, key("ctrl+u"))
	m = typeText(t, m, "wor")
	if !strings.Contains(m.View(), "Pane ID work") {
		t.Errorf("typing b/w/i/d/a must search, not filter:\n%s", m.View())
	}

	m = send(t, m, key("ctrl+u"))
	if !strings.Contains(m.View(), "importer") {
		t.Errorf("ctrl+u should clear the query:\n%s", m.View())
	}

	m = typeText(t, m, "xx")
	m = send(t, m, key("backspace"))
	m = send(t, m, key("backspace"))
	if !strings.Contains(m.View(), "importer") {
		t.Errorf("backspace should edit the query:\n%s", m.View())
	}
}

func TestSearchMatchesNameAgentWorkspaceAndMachine(t *testing.T) {
	t.Parallel()

	for query, want := range map[string]string{
		"codex":   "importer",
		"demo":    "Needs approval",
		"odoous":  "Finished job",
		"apprvl":  "Needs approval", // difuso: letras en orden
		"pane id": "Pane ID work",
	} {
		m := send(t, start(t, plain(), 100, 30), key("/"))
		for _, r := range query {
			if r == ' ' {
				m = send(t, m, key("space"))
				continue
			}
			m = typeText(t, m, string(r))
		}
		if !strings.Contains(m.View(), want) {
			t.Errorf("search %q should find %q:\n%s", query, want, m.View())
		}
	}
}

func TestSearchWithoutMatchesSaysSo(t *testing.T) {
	t.Parallel()

	m := typeText(t, send(t, start(t, plain(), 100, 30), key("/")), "zzzzz")
	if !strings.Contains(m.View(), "no agents match") {
		t.Errorf("view should say nothing matches:\n%s", m.View())
	}
	if quits(m, key("enter")) {
		t.Errorf("enter without a selection must not quit")
	}
}

func TestEscClearsTheSearchBeforeQuitting(t *testing.T) {
	t.Parallel()

	m := typeText(t, send(t, start(t, plain(), 100, 30), key("/")), "login")
	if quits(m, key("esc")) {
		t.Fatalf("esc while searching must clear the search, not quit")
	}
	m = send(t, m, key("esc"))
	if !strings.Contains(m.View(), "importer") {
		t.Errorf("esc should clear the query:\n%s", m.View())
	}
	if !quits(m, key("esc")) {
		t.Errorf("esc on the list should quit")
	}
}

func TestGoToItFromTheMenuChoosesTheSelectedAgentAndQuits(t *testing.T) {
	t.Parallel()

	m := send(t, start(t, plain(), 100, 30), key("G"))
	m = send(t, m, key("enter"))
	if !quits(m, key("g")) {
		t.Fatalf("g in the menu should quit")
	}
	m = send(t, m, key("g"))
	got, ok := m.Result()
	if !ok || got.Ref() != "odoous/w1:p3" {
		t.Errorf("Result = %v, %v, want odoous/w1:p3", got.Ref(), ok)
	}
}

func TestQuitKeysChooseNothing(t *testing.T) {
	t.Parallel()

	for _, k := range []string{"q", "esc", "ctrl+c"} {
		m := start(t, plain(), 100, 30)
		if !quits(m, key(k)) {
			t.Errorf("%s should quit", k)
		}
		m = send(t, m, key(k))
		if _, ok := m.Result(); ok {
			t.Errorf("%s must not choose an agent", k)
		}
	}
}

func TestMouseClickSelectsARowAndTheWheelScrolls(t *testing.T) {
	t.Parallel()

	m := start(t, plain(), 100, 30)
	// Líneas del cuerpo desde y=3: Local, raw, p1, p4, demo, p7...
	m = send(t, m, tea.MouseMsg{X: 10, Y: 6, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if got := breadcrumb(t, m); got != "local / raw / wS:p4" {
		t.Errorf("click on p4: %q", got)
	}
	m = send(t, m, tea.MouseMsg{X: 10, Y: 3, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if got := breadcrumb(t, m); got != "local / raw / wS:p4" {
		t.Errorf("a click on a machine header must not move the selection: %q", got)
	}
	m = send(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if got := breadcrumb(t, m); got != "odoous / odoo-inc / w1:p3" {
		t.Errorf("wheel down: %q", got)
	}
	m = send(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if got := breadcrumb(t, m); got != "local / raw / wS:p4" {
		t.Errorf("wheel up: %q", got)
	}
}

func TestSelectionScrollsIntoViewInASmallWindow(t *testing.T) {
	t.Parallel()

	m := start(t, plain(), 80, 12)
	m = send(t, m, key("G"))
	view := m.View()
	if !strings.Contains(view, "Finished job") {
		t.Errorf("the selected row must be visible:\n%s", view)
	}
	if strings.Contains(view, "importer") {
		t.Errorf("rows above the window should be scrolled out:\n%s", view)
	}
	if !strings.Contains(view, "┃") {
		t.Errorf("a scrollbar thumb is expected when the list overflows:\n%s", view)
	}
	if n := len(strings.Split(view, "\n")); n != 12 {
		t.Errorf("view has %d lines, want 12", n)
	}
	m = send(t, m, key("g"))
	if !strings.Contains(m.View(), "importer") {
		t.Errorf("going back to the top should scroll back:\n%s", m.View())
	}
}

func TestSelectedRowIsDrawnWithTheSelectionStyleWhenThereIsColor(t *testing.T) {
	t.Parallel()

	st := ui.NewStyle(ui.Capability{Color: true}, ui.Theme{})
	view := start(t, st, 100, 30).View()
	var selected string
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "importer") {
			selected = l
		}
	}
	if !strings.Contains(selected, "\x1b[7m") {
		t.Errorf("the selected row should be highlighted: %q", selected)
	}
	for _, l := range strings.Split(view, "\n") {
		if w := ui.VisibleLen(l); w != 100 {
			t.Errorf("colored line is %d cells wide: %q", w, l)
		}
	}
}

func TestWithoutColorTheSelectedRowIsMarked(t *testing.T) {
	t.Parallel()

	view := start(t, plain(), 100, 30).View()
	var marked []string
	for _, l := range strings.Split(view, "\n") {
		if strings.HasPrefix(l, "│▸") {
			marked = append(marked, l)
		}
	}
	if len(marked) != 1 || !strings.Contains(marked[0], "importer") {
		t.Errorf("exactly the selected row should carry the marker: %q", marked)
	}
}

func TestASCIIFallbackDrawsTheFrameWithPlainCharacters(t *testing.T) {
	t.Parallel()

	st := ui.NewStyle(ui.Capability{ASCII: true}, ui.Theme{})
	view := start(t, st, 80, 24).View()
	if strings.ContainsAny(view, "┌┐└┘│─├┤▸○…✖") {
		t.Errorf("ASCII mode left box drawing or symbols in the view:\n%s", view)
	}
	if !strings.HasPrefix(view, "+") || !strings.Contains(view, "|") {
		t.Errorf("ASCII frame expected:\n%s", view)
	}
}

func TestEmptyInventoryIsExplained(t *testing.T) {
	t.Parallel()

	m := tui.New(tui.Options{Inventory: inventory.Inventory{}, Style: plain()})
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	if !strings.Contains(m.View(), "no agents") {
		t.Errorf("view should say there are no agents:\n%s", m.View())
	}
	if quits(m, key("enter")) {
		t.Errorf("enter without agents must not quit")
	}
	if !quits(m, key("q")) {
		t.Errorf("q should still quit")
	}
}

func TestFullNamesShowsTheWholeTitle(t *testing.T) {
	t.Parallel()

	m := tui.New(tui.Options{Inventory: inv(), Style: plain(), FullNames: true})
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	if !strings.Contains(m.View(), "5h 8% left") {
		t.Errorf("FullNames should keep the whole title:\n%s", m.View())
	}
}

func TestWindowTooSmallDoesNotPanic(t *testing.T) {
	t.Parallel()

	for _, size := range [][2]int{{1, 1}, {5, 3}, {12, 4}} {
		m := start(t, plain(), size[0], size[1])
		_ = m.View()
		m = send(t, m, key("G"))
		_ = m.View()
	}
}
