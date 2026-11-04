package tui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/tui"
)

// machineTargets son los agentes del fixture que pertenecen a una máquina.
func machineTargets(machine string) []inventory.Target {
	var out []inventory.Target
	for _, t := range inv().Targets {
		if t.Machine == machine {
			out = append(out, t)
		}
	}
	return out
}

func startStream(t *testing.T, ch <-chan inventory.Update) tui.Model {
	t.Helper()
	m := tui.New(tui.Options{Machines: []string{"", "odoous", "remoto2"}, Updates: ch, Style: plain()})
	return send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

func localUpdate() tui.UpdateMsg {
	return tui.UpdateMsg{Machine: "", Targets: machineTargets(""), Elapsed: 20 * time.Millisecond}
}

func odoousUpdate() tui.UpdateMsg {
	return tui.UpdateMsg{Machine: "odoous", Targets: machineTargets("odoous"), Elapsed: 1300 * time.Millisecond}
}

func TestEveryServerShowsConnectingBeforeItAnswers(t *testing.T) {
	t.Parallel()

	view := startStream(t, make(chan inventory.Update)).View()
	for _, want := range []string{"Local", "odoous", "remoto2", "connecting", "loading", "0 agents"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "no agents") {
		t.Errorf("while loading the view must not claim there are no agents:\n%s", view)
	}
}

func TestFirstAnswerAppearsWithoutWaitingForTheRest(t *testing.T) {
	t.Parallel()

	m := send(t, startStream(t, make(chan inventory.Update)), localUpdate())
	view := m.View()
	if !strings.Contains(view, "importer") || !strings.Contains(view, "3 agents · 1 need you") {
		t.Errorf("the local agents should be listed at once:\n%s", view)
	}
	if got := breadcrumb(t, m); got != "local / raw / wS:p1" {
		t.Errorf("breadcrumb = %q", got)
	}
	if strings.Count(view, "connecting") != 2 {
		t.Errorf("odoous and remoto2 should still be connecting:\n%s", view)
	}
	if strings.Contains(view, "loading") {
		t.Errorf("the loading text only belongs to an empty list:\n%s", view)
	}
}

func TestALateAnswerKeepsTheSelection(t *testing.T) {
	t.Parallel()

	m := send(t, startStream(t, make(chan inventory.Update)), localUpdate())
	m = send(t, m, key("j"))
	m = send(t, m, odoousUpdate())
	if got := breadcrumb(t, m); got != "local / raw / wS:p4" {
		t.Errorf("a late answer moved the selection to %q", got)
	}
	view := m.View()
	for _, want := range []string{"Analiza login", "1.3s", "5 agents · 3 need you"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.Count(view, "connecting") != 1 {
		t.Errorf("only remoto2 should still be connecting:\n%s", view)
	}
}

func TestAFailedServerShowsWhyAndStopsConnecting(t *testing.T) {
	t.Parallel()

	m := send(t, startStream(t, make(chan inventory.Update)), localUpdate())
	m = send(t, m, tui.UpdateMsg{Machine: "remoto2", Err: context.DeadlineExceeded, Elapsed: 8 * time.Second})
	m = send(t, m, tui.UpdateMsg{Machine: "odoous", Err: context.Canceled, Elapsed: time.Second})
	view := m.View()
	if !strings.Contains(view, "✖ timeout 8.0s") || !strings.Contains(view, "✖ failed") {
		t.Errorf("failures should be explained:\n%s", view)
	}
	if strings.Contains(view, "connecting") {
		t.Errorf("nothing is connecting any more:\n%s", view)
	}
}

func TestAFailedServerStaysVisibleWhileFiltering(t *testing.T) {
	t.Parallel()

	m := send(t, startStream(t, make(chan inventory.Update)), localUpdate())
	m = send(t, m, tui.UpdateMsg{Machine: "remoto2", Err: context.DeadlineExceeded, Elapsed: 8 * time.Second})
	m = send(t, m, key("b"))
	if !strings.Contains(m.View(), "remoto2") || !strings.Contains(m.View(), "connecting") {
		t.Errorf("failed and pending servers must not vanish when filtering:\n%s", m.View())
	}
}

func TestTicksAdvanceTheElapsedTimeAndTheSpinner(t *testing.T) {
	t.Parallel()

	m := startStream(t, make(chan inventory.Update))
	first := send(t, m, tui.TickMsg{Elapsed: 3100 * time.Millisecond})
	if !strings.Contains(first.View(), "connecting 3.1s") {
		t.Errorf("elapsed time not shown:\n%s", first.View())
	}
	second := send(t, m, tui.TickMsg{Elapsed: 3200 * time.Millisecond})
	if first.View() == second.View() {
		t.Errorf("the spinner should change between ticks")
	}
}

func TestKeysWorkWhileLoading(t *testing.T) {
	t.Parallel()

	m := startStream(t, make(chan inventory.Update))
	m = send(t, m, key("j"))
	m = send(t, m, key("/"))
	m = send(t, m, key("esc"))
	if quits(m, key("enter")) {
		t.Errorf("enter without agents must not quit")
	}
	if !quits(m, key("q")) {
		t.Errorf("q should quit while loading")
	}
}

func TestTheModelKeepsWaitingForTheStream(t *testing.T) {
	t.Parallel()

	ch := make(chan inventory.Update, 1)
	ch <- tui.UpdateMsg{Machine: "", Targets: machineTargets("")}
	m := startStream(t, ch)

	if cmd := tui.New(tui.Options{Inventory: inv(), Style: plain()}).Init(); cmd != nil {
		t.Errorf("a static list needs no commands")
	}

	_, cmd := m.Update(localUpdate())
	if cmd == nil {
		t.Errorf("after an update the model must keep waiting for the next one")
	}

	msgs := runCmd(t, m.Init())
	var gotUpdate bool
	for _, msg := range msgs {
		if _, ok := msg.(tui.UpdateMsg); ok {
			gotUpdate = true
		}
	}
	if !gotUpdate {
		t.Errorf("Init should deliver the pending update from the channel, got %v", msgs)
	}

	close(ch)
	if msg := runCmd(t, m.Init()); len(msg) == 0 {
		t.Errorf("a closed stream should still answer")
	}
}

// runCmd ejecuta un comando (o el lote) y devuelve lo que entregan antes de 1 s.
func runCmd(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("nil command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	out := make(chan tea.Msg, len(batch))
	for _, c := range batch {
		go func() { out <- c() }()
	}
	var got []tea.Msg
	timeout := time.After(time.Second)
	for range batch {
		select {
		case m := <-out:
			got = append(got, m)
		case <-timeout:
			return got
		}
	}
	return got
}
