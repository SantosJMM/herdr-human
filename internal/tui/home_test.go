package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/tui"
)

func homeTabs() []tui.Tab {
	return []tui.Tab{
		tui.NewTextTab("Agents", "agents body"),
		tui.NewTextTab("Attention", "attention body"),
		tui.NewTextTab("Machines", "machines body"),
	}
}

func startHome(t *testing.T, embedded bool, tabs ...tui.Tab) tui.HomeModel {
	t.Helper()
	if len(tabs) == 0 {
		tabs = homeTabs()
	}
	m := tui.NewHome(tui.HomeOptions{Style: plain(), Tabs: tabs, Embedded: embedded})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	return next.(tui.HomeModel)
}

func homeSend(t *testing.T, m tui.HomeModel, msg tea.Msg) tui.HomeModel {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(tui.HomeModel)
}

func homeQuits(m tui.HomeModel, msg tea.Msg) bool {
	_, cmd := m.Update(msg)
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestHomeStartsOnTheFirstTabAndShowsTheTabBar(t *testing.T) {
	t.Parallel()
	v := startHome(t, true).View()
	for _, want := range []string{"[1 Agents]", "2 Attention", "3 Machines", "agents body"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "attention body") {
		t.Errorf("only the active tab shows its body:\n%s", v)
	}
}

func TestHomeTabKeysMoveBetweenTabsAndWrap(t *testing.T) {
	t.Parallel()
	m := startHome(t, true)
	m = homeSend(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if !strings.Contains(m.View(), "attention body") {
		t.Errorf("tab goes to the next tab:\n%s", m.View())
	}
	m = homeSend(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = homeSend(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if !strings.Contains(m.View(), "agents body") {
		t.Errorf("tab wraps around to the first tab:\n%s", m.View())
	}
	m = homeSend(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if !strings.Contains(m.View(), "machines body") {
		t.Errorf("shift+tab wraps back to the last tab:\n%s", m.View())
	}
}

func TestHomeNumberKeysJumpToATabAndIgnoreMissingOnes(t *testing.T) {
	t.Parallel()
	m := startHome(t, true)
	m = homeSend(t, m, key("3"))
	if !strings.Contains(m.View(), "machines body") {
		t.Errorf("3 selects the third tab:\n%s", m.View())
	}
	m = homeSend(t, m, key("9"))
	if !strings.Contains(m.View(), "machines body") {
		t.Errorf("a tab that does not exist changes nothing:\n%s", m.View())
	}
}

func TestHomeCloseKeysQuit(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		if !homeQuits(startHome(t, true), key(k)) {
			t.Errorf("%s should close the home", k)
		}
	}
}

// captureTab finge una pestaña que está pidiendo texto (una búsqueda): la pantalla
// de inicio no debe robarle las teclas.
type captureTab struct{ got []string }

func (c *captureTab) Title() string          { return "Typing" }
func (c *captureTab) Init() tea.Cmd          { return nil }
func (c *captureTab) Captures() bool         { return true }
func (c *captureTab) View(w, h int) []string { return []string{"typing body"} }
func (c *captureTab) Update(msg tea.Msg) (tui.Tab, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		c.got = append(c.got, k.String())
	}
	return c, nil
}

func TestHomeLeavesTheKeysToATabThatIsTyping(t *testing.T) {
	t.Parallel()
	c := &captureTab{}
	m := startHome(t, true, c, tui.NewTextTab("Other", "other body"))
	for _, k := range []string{"q", "esc", "2", "tab"} {
		next, cmd := m.Update(key(k))
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Errorf("%s must reach the tab, not close the home", k)
			}
		}
		m = next.(tui.HomeModel)
	}
	if len(c.got) != 4 || !strings.Contains(m.View(), "typing body") {
		t.Errorf("the tab should have received 4 keys and stay active: %v\n%s", c.got, m.View())
	}
	if !homeQuits(m, key("ctrl+c")) {
		t.Error("ctrl+c always closes, even while typing")
	}
}

func TestHomeForwardsOtherMessagesToEveryTab(t *testing.T) {
	t.Parallel()
	a, b := &captureTab{}, &recordTab{}
	m := startHome(t, true, a, b)
	before := b.msgs
	homeSend(t, m, loadedMsg{})
	if b.msgs != before+1 {
		t.Error("a message that is not a key reaches every tab, even the hidden ones (loading updates)")
	}
}

// loadedMsg hace de respuesta de una carga de datos.
type loadedMsg struct{}

type recordTab struct{ msgs int }

func (r *recordTab) Title() string          { return "Rec" }
func (r *recordTab) Init() tea.Cmd          { return nil }
func (r *recordTab) Captures() bool         { return false }
func (r *recordTab) View(w, h int) []string { return nil }
func (r *recordTab) Update(msg tea.Msg) (tui.Tab, tea.Cmd) {
	r.msgs++
	return r, nil
}

func TestHomeFrameDependsOnEmbedded(t *testing.T) {
	t.Parallel()
	framed := startHome(t, false).View()
	if first := strings.SplitN(framed, "\n", 2)[0]; !strings.Contains(first, "hum home") {
		t.Errorf("on its own the home draws a frame whose top border has the title:\n%s", framed)
	}
	v := startHome(t, true).View()
	if strings.Contains(v, "hum home") {
		t.Errorf("inside a herdr popup there is no title: herdr draws it:\n%s", v)
	}
	if first := strings.SplitN(v, "\n", 2)[0]; !strings.Contains(first, "[1 Agents]") {
		t.Errorf("inside a herdr popup the first line is already the tab bar:\n%s", v)
	}
}

func TestHomeViewFillsTheWindowExactly(t *testing.T) {
	t.Parallel()
	for _, embedded := range []bool{true, false} {
		lines := strings.Split(startHome(t, embedded).View(), "\n")
		if len(lines) != 20 {
			t.Errorf("embedded=%v: %d lines, want 20 (the window height)", embedded, len(lines))
		}
	}
}
