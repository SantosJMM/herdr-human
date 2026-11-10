package tui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/tui"
)

// Un selector alojado dentro de otro modelo (la pantalla de inicio) no cierra el
// programa: avisa con LeaveMsg y es quien lo aloja quien decide qué hacer.

func startHosted(t *testing.T) tui.Model {
	t.Helper()
	m := tui.New(tui.Options{Inventory: inv(), Style: plain(), Hosted: true})
	return send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

// leaves ejecuta el comando que devuelve la tecla y dice si fue un LeaveMsg.
func leaves(t *testing.T, m tui.Model, msg tea.Msg) (tui.LeaveMsg, bool) {
	t.Helper()
	_, cmd := m.Update(msg)
	if cmd == nil {
		return tui.LeaveMsg{}, false
	}
	out := cmd()
	if _, quit := out.(tea.QuitMsg); quit {
		t.Fatalf("a hosted selector must not quit the program")
	}
	l, ok := out.(tui.LeaveMsg)
	return l, ok
}

func TestHostedQuitKeysLeaveWithoutChoosing(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		l, ok := leaves(t, startHosted(t), key(k))
		if !ok || l.Chosen {
			t.Errorf("%s: want a LeaveMsg without a choice, got %+v (ok=%v)", k, l, ok)
		}
	}
}

func TestHostedGoToItLeavesWithTheSelectedAgent(t *testing.T) {
	t.Parallel()
	m := send(t, startHosted(t), key("G"))
	m = send(t, m, key("enter"))
	l, ok := leaves(t, m, key("g"))
	if !ok || !l.Chosen || l.Target.Ref() != "odoous/w1:p3" {
		t.Errorf("got %+v (ok=%v), want the agent odoous/w1:p3", l, ok)
	}
}

func TestHostedCtrlCInTheMenuLeaves(t *testing.T) {
	t.Parallel()
	m := send(t, startHosted(t), key("enter"))
	if l, ok := leaves(t, m, key("ctrl+c")); !ok || l.Chosen {
		t.Errorf("got %+v (ok=%v)", l, ok)
	}
}

func TestNotHostedStillQuitsTheProgram(t *testing.T) {
	t.Parallel()
	// Lo de siempre: sin Hosted, las mismas teclas terminan el programa.
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		if !quits(start(t, plain(), 100, 30), key(k)) {
			t.Errorf("%s must still quit when not hosted", k)
		}
	}
}
