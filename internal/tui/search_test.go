package tui_test

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/tui"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func strip(s string) string { return ansi.ReplaceAllString(s, "") }

func special(name string) tea.KeyMsg {
	return map[string]tea.KeyMsg{
		"left":   {Type: tea.KeyLeft},
		"right":  {Type: tea.KeyRight},
		"home":   {Type: tea.KeyHome},
		"end":    {Type: tea.KeyEnd},
		"delete": {Type: tea.KeyDelete},
	}[name]
}

func TestSearchCursorMovesAndEditsInTheMiddle(t *testing.T) {
	t.Parallel()

	m := typeText(t, send(t, start(t, plain(), 100, 30), key("/")), "logn")
	m = send(t, m, special("left"))
	if !strings.Contains(m.View(), "log▌n") {
		t.Fatalf("the cursor should sit before the last letter:\n%s", m.View())
	}
	m = typeText(t, m, "i")
	if !strings.Contains(m.View(), "logi▌n") || !strings.Contains(m.View(), "Analiza login") {
		t.Errorf("inserting at the cursor should give login and keep searching:\n%s", m.View())
	}
	if strings.Contains(m.View(), "importer") {
		t.Errorf("only the login agent matches:\n%s", m.View())
	}
}

func TestSearchHomeEndDeleteAndBackspaceAtTheCursor(t *testing.T) {
	t.Parallel()

	m := typeText(t, send(t, start(t, plain(), 100, 30), key("/")), "xlogin")
	m = send(t, m, special("home"))
	if !strings.Contains(m.View(), "▌xlogin") {
		t.Fatalf("home should put the cursor first:\n%s", m.View())
	}
	m = send(t, m, special("delete"))
	if !strings.Contains(m.View(), "▌login") || !strings.Contains(m.View(), "Analiza login") {
		t.Errorf("delete should remove the letter under the cursor:\n%s", m.View())
	}
	m = send(t, m, key("backspace"))
	if !strings.Contains(m.View(), "▌login") {
		t.Errorf("backspace at the start does nothing:\n%s", m.View())
	}
	m = send(t, m, special("end"))
	m = send(t, m, key("backspace"))
	if !strings.Contains(m.View(), "logi▌") {
		t.Errorf("end then backspace should remove the last letter:\n%s", m.View())
	}
	m = send(t, m, special("right"))
	m = send(t, m, special("delete"))
	if !strings.Contains(m.View(), "logi▌") {
		t.Errorf("right and delete at the end do nothing:\n%s", m.View())
	}
}

func TestSearchEditingIsPerLetterNotPerByte(t *testing.T) {
	t.Parallel()

	m := typeText(t, send(t, start(t, plain(), 100, 30), key("/")), "añu")
	m = send(t, m, special("left"))
	m = send(t, m, key("backspace"))
	if !strings.Contains(m.View(), "a▌u") {
		t.Errorf("backspace should remove the whole ñ:\n%s", m.View())
	}
}

func TestSayLineHasTheSameCursorEditing(t *testing.T) {
	t.Parallel()

	f := &fakeActions{}
	m := typed(t, send(t, withActions(t, f), ctrl("s")), "helo")
	m = send(t, m, special("left"))
	m = typed(t, m, "l")
	if !strings.Contains(m.View(), "hell▌o") {
		t.Fatalf("the cursor should follow the edit:\n%s", m.View())
	}
	m = send(t, m, special("home"))
	m = typed(t, m, ">")
	m = send(t, m, special("delete"))
	next, cmd := m.Update(key("enter"))
	_ = settle(t, next.(tui.Model), cmd)
	if len(f.said) != 1 || f.said[0] != ">ello" {
		t.Errorf("sent %q, want %q", f.said, ">ello")
	}
}

func coloured() *ui.Style { return ui.NewStyle(ui.Capability{Color: true}, ui.Theme{}) }

func TestSearchHighlightsWhatMatches(t *testing.T) {
	t.Parallel()

	m := send(t, start(t, coloured(), 100, 30), key("/"))
	m = typeText(t, m, "LOGIN")
	view := m.View()
	if !strings.Contains(view, "\x1b[36mlogin\x1b[0m") {
		t.Errorf("the matching letters should be highlighted, ignoring case:\n%q", view)
	}
	if plainText := strip(view); !strings.Contains(plainText, "Analiza login") {
		t.Errorf("highlighting must not change the text:\n%s", plainText)
	}
}

func TestSearchHighlightsTheAgentTypeAndFuzzyLetters(t *testing.T) {
	t.Parallel()

	m := typeText(t, send(t, start(t, coloured(), 100, 30), key("/")), "codex")
	if !strings.Contains(m.View(), "\x1b[36mcodex\x1b[0m") {
		t.Errorf("the agent type column should be highlighted:\n%q", m.View())
	}

	m = typeText(t, send(t, start(t, coloured(), 100, 30), key("/")), "apprvl")
	view := m.View()
	if !strings.Contains(strip(view), "Needs approval") {
		t.Fatalf("the fuzzy match should list the agent:\n%s", strip(view))
	}
	// "approval" con a-p-p-r seguidas, o y a sin marcar, v y l sueltas.
	if !strings.Contains(view, "\x1b[36mv\x1b[0m") || !strings.Contains(view, "\x1b[36mappr\x1b[0m") {
		t.Errorf("fuzzy matches highlight letter by letter:\n%q", view)
	}
}

func TestHighlightKeepsEveryLineTheExactWidth(t *testing.T) {
	t.Parallel()

	m := typeText(t, send(t, start(t, coloured(), 60, 20), key("/")), "o")
	for i, l := range strings.Split(m.View(), "\n") {
		if w := ui.VisibleLen(l); w != 60 {
			t.Errorf("line %d is %d cells wide: %q", i, w, l)
		}
	}
}

func TestPlainOutputHasNoHighlightMarks(t *testing.T) {
	t.Parallel()

	m := typeText(t, send(t, start(t, plain(), 100, 30), key("/")), "login")
	if strings.Contains(m.View(), "\x1b") {
		t.Errorf("without colour there must be no escape sequences:\n%q", m.View())
	}
}
