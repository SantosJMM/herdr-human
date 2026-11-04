package tui_test

import (
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/tui"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

type branches struct {
	mu    sync.Mutex
	dirs  []string
	names map[string]string
}

func (b *branches) lookup(dir string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dirs = append(b.dirs, dir)
	return b.names[dir]
}

func withBranches(t *testing.T, b *branches) tui.Model {
	t.Helper()
	m := tui.New(tui.Options{Inventory: inv(), Style: plain(), Branch: b.lookup})
	return send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

func lineWith(t *testing.T, view, text string) string {
	t.Helper()
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, text) {
			return l
		}
	}
	t.Fatalf("no line with %q in:\n%s", text, view)
	return ""
}

func TestLocalWorkspaceShowsItsBranchOnTheRight(t *testing.T) {
	t.Parallel()

	b := &branches{names: map[string]string{`C:\src\raw`: "feat/ux"}}
	m := withBranches(t, b)
	l := lineWith(t, m.View(), " raw")
	if !strings.HasSuffix(strings.TrimRight(l, "│ "), "feat/ux") {
		t.Errorf("the branch should end the workspace row: %q", l)
	}
	if ui.VisibleLen(l) != 100 {
		t.Errorf("the row is %d cells wide", ui.VisibleLen(l))
	}
}

func TestBranchIsOnlyLookedUpForLocalWorkspacesWithAFolder(t *testing.T) {
	t.Parallel()

	b := &branches{names: map[string]string{`C:\src\raw`: "main", "/home/odoo": "should-never-show"}}
	view := withBranches(t, b).View()
	if strings.Contains(view, "should-never-show") {
		t.Errorf("remote checkouts must not get a branch:\n%s", view)
	}
	if len(b.dirs) != 1 || b.dirs[0] != `C:\src\raw` {
		t.Errorf("looked up %q, want only the local folder of the workspace", b.dirs)
	}
}

func TestBranchLookupsAreCachedAcrossRedraws(t *testing.T) {
	t.Parallel()

	b := &branches{names: map[string]string{`C:\src\raw`: "main"}}
	m := withBranches(t, b)
	m = typeText(t, send(t, m, key("/")), "login")
	m = send(t, m, key("esc"))
	m = send(t, m, key("b"))
	_ = send(t, m, key("a"))
	if len(b.dirs) != 1 {
		t.Errorf("the folder was looked up %d times, want once", len(b.dirs))
	}
}

func TestNoBranchWithoutALookupFunctionOrWithoutAnAnswer(t *testing.T) {
	t.Parallel()

	if strings.Contains(start(t, plain(), 100, 30).View(), "main") {
		t.Errorf("no branch expected when the option is not set")
	}
	b := &branches{names: map[string]string{}}
	if view := withBranches(t, b).View(); strings.Contains(view, "feat") {
		t.Errorf("a folder that is not a repository shows nothing:\n%s", view)
	}
}

func TestBranchAppearsWhenTheLocalAnswerArrives(t *testing.T) {
	t.Parallel()

	b := &branches{names: map[string]string{`C:\src\raw`: "main"}}
	m := tui.New(tui.Options{
		Machines: []string{"", "odoous"}, Updates: make(chan inventory.Update), Style: plain(), Branch: b.lookup,
	})
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if len(b.dirs) != 0 {
		t.Fatalf("nothing to look up before any answer: %q", b.dirs)
	}
	m = send(t, m, localUpdate())
	if !strings.Contains(lineWith(t, m.View(), " raw"), "main") {
		t.Errorf("the branch should show once the local agents arrive:\n%s", m.View())
	}
}

func TestRefreshReadsTheBranchAgain(t *testing.T) {
	t.Parallel()

	b := &branches{names: map[string]string{`C:\src\raw`: "main"}}
	r := &refresher{answer: map[string][]inventory.Target{"": machineTargets("")}}
	m := tui.New(tui.Options{
		Machines: []string{""}, Updates: make(chan inventory.Update), Style: plain(), Branch: b.lookup,
		Refresh: r.refresh, Actions: &fakeActions{},
	})
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = send(t, m, localUpdate())

	b.mu.Lock()
	b.names[`C:\src\raw`] = "other-branch"
	b.mu.Unlock()
	next, cmd := m.Update(key("r"))
	m = settle(t, next.(tui.Model), cmd)

	if !strings.Contains(lineWith(t, m.View(), " raw"), "other-branch") {
		t.Errorf("a refresh should show the branch the folder is on now:\n%s", m.View())
	}
}
