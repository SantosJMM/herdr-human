package tui_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/tui"
)

// refresher registra qué máquinas se volvieron a consultar y qué devolvieron.
type refresher struct {
	mu     sync.Mutex
	called []string
	answer map[string][]inventory.Target
	err    error
}

// refresh lo llaman varias goroutines a la vez, como ocurre con herdr real.
func (r *refresher) refresh(_ context.Context, machine string) ([]inventory.Target, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.called = append(r.called, machine)
	return slices.Clone(r.answer[machine]), r.err
}

// loaded es un selector con carga progresiva ya completa y acciones conectadas.
func loaded(t *testing.T, f *fakeActions, r *refresher) tui.Model {
	t.Helper()
	opts := tui.Options{
		Machines: []string{"", "odoous", "remoto2"}, Updates: make(chan inventory.Update),
		Style: plain(), Actions: f,
	}
	if r != nil {
		opts.Refresh = r.refresh
	}
	m := send(t, tui.New(opts), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = send(t, m, localUpdate())
	m = send(t, m, odoousUpdate())
	return send(t, m, tui.UpdateMsg{Machine: "remoto2", Elapsed: 50 * time.Millisecond})
}

// settle ejecuta el comando y entrega sus mensajes (también los que provoquen
// después), salvo los de reloj, que no terminan nunca.
func settle(t *testing.T, m tui.Model, cmd tea.Cmd) tui.Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	for _, msg := range runCmd(t, cmd) {
		switch msg.(type) {
		case tui.TickMsg, tui.BusyTickMsg, tea.QuitMsg:
			continue
		}
		next, more := m.Update(msg)
		m = settle(t, next.(tui.Model), more)
	}
	return m
}

func importerWorking() []inventory.Target {
	ts := machineTargets("")
	ts[0].Status = "working"
	return ts
}

func TestAnActionRefreshesTheMachineOfTheAgent(t *testing.T) {
	t.Parallel()

	f := &fakeActions{}
	r := &refresher{answer: map[string][]inventory.Target{"": importerWorking()}}
	m := loaded(t, f, r)

	m = typed(t, send(t, m, ctrl("s")), "go on")
	next, cmd := m.Update(key("enter"))
	m = settle(t, next.(tui.Model), cmd)

	if len(r.called) != 1 || r.called[0] != "" {
		t.Fatalf("refreshed machines = %q, want only the local server", r.called)
	}
	view := m.View()
	if !strings.Contains(view, "importer") || strings.Contains(view, "idle") {
		t.Errorf("the status should now be working, not idle:\n%s", view)
	}
	if got := breadcrumb(t, m); got != "local / raw / wS:p1" {
		t.Errorf("the selection moved to %q", got)
	}
	if strings.Contains(view, "refreshing") {
		t.Errorf("the machine finished refreshing:\n%s", view)
	}
}

func TestRKeyRefreshesEveryMachineAndShowsItWhileWaiting(t *testing.T) {
	t.Parallel()

	r := &refresher{answer: map[string][]inventory.Target{"": importerWorking()}}
	m := loaded(t, &fakeActions{}, r)

	next, cmd := m.Update(key("r"))
	m = next.(tui.Model)
	if cmd == nil || strings.Count(m.View(), "refreshing") != 3 {
		t.Fatalf("every machine should show it is refreshing (cmd=%v):\n%s", cmd != nil, m.View())
	}
	if !strings.Contains(m.View(), "importer") {
		t.Errorf("the rows stay visible while refreshing:\n%s", m.View())
	}
	m = settle(t, m, cmd)

	if len(r.called) != 3 {
		t.Errorf("refreshed machines = %q, want local, odoous and remoto2", r.called)
	}
	if strings.Contains(m.View(), "refreshing") || !strings.Contains(m.View(), "working") {
		t.Errorf("after the answers the new statuses show and nothing is pending:\n%s", m.View())
	}
}

func TestAFailedRefreshKeepsTheRowsAndSaysSo(t *testing.T) {
	t.Parallel()

	r := &refresher{err: errors.New("connection reset")}
	m := loaded(t, &fakeActions{}, r)

	next, cmd := m.Update(key("r"))
	m = settle(t, next.(tui.Model), cmd)
	view := m.View()
	if !strings.Contains(view, "importer") || !strings.Contains(view, "Analiza login") {
		t.Errorf("a failed refresh must not erase the rows:\n%s", view)
	}
	if !strings.Contains(view, "refresh failed") || !strings.Contains(view, "connection reset") {
		t.Errorf("the failure should be reported:\n%s", view)
	}
	if strings.Contains(view, "refreshing") {
		t.Errorf("nothing is refreshing any more:\n%s", view)
	}
}

func TestRefreshKeepsTheSelectedAgent(t *testing.T) {
	t.Parallel()

	r := &refresher{answer: map[string][]inventory.Target{"": importerWorking()}}
	m := send(t, loaded(t, &fakeActions{}, r), key("j"))
	next, cmd := m.Update(key("r"))
	m = settle(t, next.(tui.Model), cmd)
	if got := breadcrumb(t, m); got != "local / raw / wS:p4" {
		t.Errorf("selection = %q, want it unchanged", got)
	}
}

func TestWithoutARefreshFunctionNothingIsRefreshed(t *testing.T) {
	t.Parallel()

	m := loaded(t, &fakeActions{}, nil)
	next, cmd := m.Update(key("r"))
	if cmd != nil || !strings.Contains(next.(tui.Model).View(), "refresh: not available") {
		t.Errorf("r without a refresh function should say so (cmd=%v):\n%s", cmd != nil, next.(tui.Model).View())
	}

	m = typed(t, send(t, m, ctrl("s")), "hi")
	sent, sendCmd := m.Update(key("enter"))
	m = settle(t, sent.(tui.Model), sendCmd)
	if !strings.Contains(m.View(), "sent to local/wS:p1") {
		t.Errorf("the action itself must still work:\n%s", m.View())
	}
}

func TestLookDoesNotTriggerARefresh(t *testing.T) {
	t.Parallel()

	r := &refresher{}
	f := &fakeActions{lookText: "text"}
	m := loaded(t, f, r)
	next, cmd := m.Update(ctrl("l"))
	settle(t, next.(tui.Model), cmd)
	if len(r.called) != 0 {
		t.Errorf("Look only reads: refreshed %q", r.called)
	}
}
