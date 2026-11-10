package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/tui"
)

// startAgentsHome abre la pantalla de inicio con la pestaña Agents (el selector de
// pick alojado) y otra pestaña de texto, a 100x30.
func startAgentsHome(t *testing.T) tui.HomeModel {
	t.Helper()
	tabs := []tui.Tab{
		tui.NewAgentsTab(tui.Options{Inventory: inv(), Style: plain()}),
		tui.NewTextTab("Other", "other body"),
	}
	m := tui.NewHome(tui.HomeOptions{Style: plain(), Tabs: tabs, Embedded: true})
	return homeSend(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

func TestAgentsTabShowsTheAgentsOfPick(t *testing.T) {
	t.Parallel()
	v := startAgentsHome(t).View()
	for _, want := range []string{"[1 Agents]", "2 Other", "importer", "Needs approval", "/ search agents", "local / raw"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	if n := len(strings.Split(v, "\n")); n != 30 {
		t.Errorf("the view must fill the window: %d lines, want 30", n)
	}
}

func TestSearchingInTheAgentsTabKeepsTheHomeKeysAway(t *testing.T) {
	t.Parallel()
	m := startAgentsHome(t)
	m = homeSend(t, m, key("/"))
	// q, 2 y tab son texto de la búsqueda (o no hacen nada), no órdenes de la pantalla.
	for _, k := range []string{"q", "2"} {
		if homeQuits(m, key(k)) {
			t.Fatalf("%s while searching must not close the home", k)
		}
		m = homeSend(t, m, key(k))
	}
	m = homeSend(t, m, tea.KeyMsg{Type: tea.KeyTab})
	v := m.View()
	if !strings.Contains(v, "/ q2") || !strings.Contains(v, "[1 Agents]") {
		t.Errorf("the query is q2 and the tab did not change:\n%s", v)
	}
}

func TestEscWhileSearchingClearsTheSearchAndOnlyTheNextEscClosesTheHome(t *testing.T) {
	t.Parallel()
	m := startAgentsHome(t)
	m = homeSend(t, m, key("/"))
	m = homeSend(t, m, key("x"))
	if homeQuits(m, key("esc")) {
		t.Fatal("the first esc leaves the search, it does not close the home")
	}
	m = homeSend(t, m, key("esc"))
	if !strings.Contains(m.View(), "/ search agents") {
		t.Errorf("the search should be cleared:\n%s", m.View())
	}
	if !homeQuits(m, key("esc")) {
		t.Error("with nothing left to clear, esc closes the home")
	}
}

func TestTabAndNumbersWorkOnTheAgentListAndEscInTheMenuGoesBackToTheList(t *testing.T) {
	t.Parallel()
	m := startAgentsHome(t)
	m = homeSend(t, m, key("enter")) // abre el menú de acciones del agente
	if homeQuits(m, key("esc")) {
		t.Fatal("esc in the action menu closes the menu, not the home")
	}
	m = homeSend(t, m, key("esc"))
	if !strings.Contains(m.View(), "/ search agents") {
		t.Errorf("back on the list:\n%s", m.View())
	}
	m = homeSend(t, m, key("2"))
	if !strings.Contains(m.View(), "other body") {
		t.Errorf("2 switches to the second tab from the list:\n%s", m.View())
	}
}

func TestChoosingAnAgentFromTheMenuClosesTheHomeWithThatAgent(t *testing.T) {
	t.Parallel()
	m := startAgentsHome(t)
	m = homeSend(t, m, key("G")) // el último agente de la lista
	m = homeSend(t, m, key("enter"))
	next, cmd := m.Update(key("g")) // «Go to it»
	if cmd == nil {
		t.Fatal("go to it must produce a command")
	}
	// El selector avisa con LeaveMsg; la pantalla lo recibe, guarda el agente y cierra.
	leave, ok := cmd().(tui.LeaveMsg)
	if !ok || !leave.Chosen {
		t.Fatalf("expected a LeaveMsg with a choice, got %T %+v", cmd(), leave)
	}
	next, cmd = next.(tui.HomeModel).Update(leave)
	if cmd == nil {
		t.Fatal("the home must close when the agent is chosen")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Fatal("the home must quit")
	}
	res := next.(tui.HomeModel).Result()
	if !res.Chosen || res.Target.PaneID != leave.Target.PaneID || res.Target.PaneID == "" {
		t.Errorf("result = %+v, leave = %+v", res, leave)
	}
}

func TestClickingARowInTheAgentsTabSelectsItDespiteTheHomeBarAbove(t *testing.T) {
	t.Parallel()
	m := startAgentsHome(t)
	// En la pantalla incrustada, el cuerpo del selector empieza dos líneas por debajo
	// de la barra de pestañas: Local, raw, p1, p4 → la fila de p4.
	var y int
	for i, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "Pane ID work") {
			y = i
		}
	}
	if y == 0 {
		t.Fatalf("the row is not on the screen:\n%s", m.View())
	}
	m = homeSend(t, m, tea.MouseMsg{X: 12, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if !strings.Contains(m.View(), "local / raw / wS:p4") {
		t.Errorf("the click on the p4 row should select it:\n%s", m.View())
	}
}

func TestClickingTheTabBarSwitchesTabs(t *testing.T) {
	t.Parallel()
	m := startAgentsHome(t)
	m = homeSend(t, m, tea.MouseMsg{X: 16, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if !strings.Contains(m.View(), "other body") {
		t.Errorf("a click on «2 Other» in the tab bar selects it:\n%s", m.View())
	}
}
