package tui_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/tui"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// fakeActions registra lo que el selector pide hacer.
type fakeActions struct {
	said, renamed, asked, copied []string
	looked, waited               int
	sayErr                       error
	lookText                     string
	reply                        tui.Reply
	waitStatus                   string
	copyErr                      error
	lastTarget                   inventory.Target
}

func (f *fakeActions) Say(_ context.Context, t inventory.Target, text string) error {
	f.said, f.lastTarget = append(f.said, text), t
	return f.sayErr
}

func (f *fakeActions) Look(_ context.Context, t inventory.Target) (string, error) {
	f.looked++
	f.lastTarget = t
	return f.lookText, nil
}

func (f *fakeActions) Rename(_ context.Context, t inventory.Target, name string) error {
	f.renamed, f.lastTarget = append(f.renamed, name), t
	return nil
}

func (f *fakeActions) Ask(_ context.Context, t inventory.Target, text string) (tui.Reply, error) {
	f.asked, f.lastTarget = append(f.asked, text), t
	return f.reply, nil
}

func (f *fakeActions) WaitNotify(_ context.Context, t inventory.Target) (string, error) {
	f.waited++
	f.lastTarget = t
	return f.waitStatus, nil
}

func (f *fakeActions) Copy(text string) error {
	f.copied = append(f.copied, text)
	return f.copyErr
}

func withActions(t *testing.T, f *fakeActions) tui.Model {
	t.Helper()
	m := tui.New(tui.Options{Inventory: inv(), Style: plain(), Actions: f})
	return send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

// run ejecuta el comando que devolvió la última tecla y entrega sus mensajes.
func deliver(t *testing.T, m tui.Model, msg tea.Msg) tui.Model {
	t.Helper()
	m2, cmd := m.Update(msg)
	m = m2.(tui.Model)
	if cmd == nil {
		return m
	}
	for _, out := range runCmd(t, cmd) {
		if _, tick := out.(tui.BusyTickMsg); tick {
			continue
		}
		m = send(t, m, out)
	}
	return m
}

func typed(t *testing.T, m tui.Model, text string) tui.Model {
	t.Helper()
	for _, r := range text {
		if r == ' ' {
			m = send(t, m, key("space"))
			continue
		}
		m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func ctrl(name string) tea.KeyMsg {
	return map[string]tea.KeyMsg{
		"a": {Type: tea.KeyCtrlA}, "s": {Type: tea.KeyCtrlS}, "l": {Type: tea.KeyCtrlL}, "n": {Type: tea.KeyCtrlN},
	}[name]
}

func TestEnterOpensTheActionMenuForTheSelectedAgent(t *testing.T) {
	t.Parallel()

	m := withActions(t, &fakeActions{})
	if quits(m, key("enter")) {
		t.Fatalf("enter must open the menu, not quit")
	}
	view := send(t, m, key("enter")).View()
	for _, want := range []string{
		"local / importer", "Ask…", "Say…", "Look", "Rename…", "Go to it", "Wait and notify", "Copy pane id",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("menu lacks %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "▸ Ask…") {
		t.Errorf("the first item should be selected:\n%s", view)
	}
}

func TestMenuNavigationAndEscape(t *testing.T) {
	t.Parallel()

	m := send(t, withActions(t, &fakeActions{}), key("enter"))
	m = send(t, m, key("j"))
	if !strings.Contains(m.View(), "▸ Say…") {
		t.Errorf("j should select the second item:\n%s", m.View())
	}
	m = send(t, m, key("up"))
	if !strings.Contains(m.View(), "▸ Ask…") {
		t.Errorf("up should go back:\n%s", m.View())
	}
	m = send(t, m, key("esc"))
	if strings.Contains(m.View(), "Ask…") || !strings.Contains(m.View(), "importer") {
		t.Errorf("esc should close the menu and show the list:\n%s", m.View())
	}
	if quits(send(t, m, key("enter")), key("esc")) {
		t.Errorf("esc inside the menu must not quit")
	}
}

func TestGoToItChoosesTheAgent(t *testing.T) {
	t.Parallel()

	for name, keys := range map[string][]string{"hotkey": {"enter", "g"}, "selection": {"enter", "j", "j", "j", "j", "enter"}} {
		m := send(t, withActions(t, &fakeActions{}), key("G"))
		for _, k := range keys[:len(keys)-1] {
			m = send(t, m, key(k))
		}
		last := key(keys[len(keys)-1])
		if !quits(m, last) {
			t.Errorf("%s: the last key should quit", name)
		}
		m = send(t, m, last)
		got, ok := m.Result()
		if !ok || got.Ref() != "odoous/w1:p3" {
			t.Errorf("%s: Result = %v, %v", name, got.Ref(), ok)
		}
	}
}

func TestCopyPaneIDUsesTheClipboard(t *testing.T) {
	t.Parallel()

	f := &fakeActions{}
	m := send(t, send(t, withActions(t, f), key("enter")), key("c"))
	if len(f.copied) != 1 || f.copied[0] != "wS:p1" {
		t.Fatalf("copied = %v, want the pane id", f.copied)
	}
	if !strings.Contains(m.View(), "copied wS:p1") {
		t.Errorf("the copy should be confirmed:\n%s", m.View())
	}

	f2 := &fakeActions{copyErr: errors.New("no clipboard")}
	m = send(t, send(t, withActions(t, f2), key("enter")), key("c"))
	if !strings.Contains(m.View(), "no clipboard") || !strings.Contains(m.View(), "wS:p1") {
		t.Errorf("a failed copy should say why and still show the id:\n%s", m.View())
	}
}

func TestDirectShortcutsOpenTheInputOrRunTheAction(t *testing.T) {
	t.Parallel()

	for shortcut, prompt := range map[string]string{"a": "message >", "s": "message >", "n": "name >"} {
		m := send(t, withActions(t, &fakeActions{}), ctrl(shortcut))
		if !strings.Contains(m.View(), prompt) {
			t.Errorf("ctrl+%s: view lacks %q:\n%s", shortcut, prompt, m.View())
		}
	}

	f := &fakeActions{lookText: "line one\nline two"}
	m := deliver(t, withActions(t, f), ctrl("l"))
	if f.looked != 1 || !strings.Contains(m.View(), "line two") || !strings.Contains(m.View(), "── look ──") {
		t.Errorf("ctrl+l should show what the agent printed (looked=%d):\n%s", f.looked, m.View())
	}
	m = send(t, m, key("x"))
	if strings.Contains(m.View(), "line two") || !strings.Contains(m.View(), "importer") {
		t.Errorf("any key should leave the look view:\n%s", m.View())
	}
}

func TestSayRunsTheActionAndReportsIt(t *testing.T) {
	t.Parallel()

	f := &fakeActions{}
	m := send(t, withActions(t, f), ctrl("s"))
	m = typed(t, m, "hello wrld")
	m = send(t, m, key("backspace"))
	m = send(t, m, key("backspace"))
	m = send(t, m, key("backspace"))
	m = typed(t, m, "orld")
	if !strings.Contains(m.View(), "hello world") {
		t.Fatalf("the typed text should be visible:\n%s", m.View())
	}
	m = deliver(t, m, key("enter"))
	if len(f.said) != 1 || f.said[0] != "hello world" || f.lastTarget.Ref() != "local/wS:p1" {
		t.Fatalf("Say calls = %v on %v", f.said, f.lastTarget.Ref())
	}
	if !strings.Contains(m.View(), "sent to local/wS:p1") || !strings.Contains(m.View(), "importer") {
		t.Errorf("the list should come back with a confirmation:\n%s", m.View())
	}
}

func TestInputCanBeCancelledAndEmptyIsNotSent(t *testing.T) {
	t.Parallel()

	f := &fakeActions{}
	m := send(t, withActions(t, f), ctrl("s"))
	if _, cmd := m.Update(key("enter")); cmd != nil {
		t.Errorf("an empty message must not be sent")
	}
	m = typed(t, m, "oops")
	m = send(t, m, key("ctrl+u"))
	if strings.Contains(m.View(), "oops") {
		t.Errorf("ctrl+u should clear the input:\n%s", m.View())
	}
	m = typed(t, m, "again")
	m = send(t, m, key("esc"))
	if strings.Contains(m.View(), "message >") || len(f.said) != 0 {
		t.Errorf("esc should cancel without sending (said=%v):\n%s", f.said, m.View())
	}
	if !quits(m, key("esc")) {
		t.Errorf("after cancelling, esc on the list quits")
	}
}

func TestRenameValidatesWhileTyping(t *testing.T) {
	t.Parallel()

	f := &fakeActions{}
	m := send(t, withActions(t, f), ctrl("n"))
	m = typed(t, m, "Bad Name")
	if !strings.Contains(m.View(), "lowercase") {
		t.Errorf("an invalid name should be explained as you type:\n%s", m.View())
	}
	if _, cmd := m.Update(key("enter")); cmd != nil {
		t.Errorf("an invalid name must not be sent")
	}
	m = send(t, m, key("ctrl+u"))
	m = typed(t, m, "docs-agent")
	if strings.Contains(m.View(), "lowercase") {
		t.Errorf("a valid name should show no error:\n%s", m.View())
	}
	m = deliver(t, m, key("enter"))
	if len(f.renamed) != 1 || f.renamed[0] != "docs-agent" {
		t.Fatalf("Rename calls = %v", f.renamed)
	}
	if !strings.Contains(m.View(), "docs-agent") {
		t.Errorf("the list should show the new name:\n%s", m.View())
	}
}

func TestRenameWithAnEmptyNameClearsIt(t *testing.T) {
	t.Parallel()

	f := &fakeActions{}
	m := deliver(t, send(t, withActions(t, f), ctrl("n")), key("enter"))
	if len(f.renamed) != 1 || f.renamed[0] != "" {
		t.Errorf("Rename calls = %q, want one empty name", f.renamed)
	}
	if !strings.Contains(m.View(), "name cleared") {
		t.Errorf("clearing should be reported:\n%s", m.View())
	}
}

func TestAskShowsTheReplyAndComesBack(t *testing.T) {
	t.Parallel()

	f := &fakeActions{reply: tui.Reply{Status: "idle", Text: "All 42 tests passed.", AfterMessage: true}}
	m := typed(t, send(t, withActions(t, f), ctrl("a")), "run the tests")

	m2, cmd := m.Update(key("enter"))
	busy := m2.(tui.Model).View()
	if cmd == nil || !strings.Contains(busy, "asking") || !strings.Contains(busy, "esc") {
		t.Fatalf("while waiting the view should say so (cmd=%v):\n%s", cmd != nil, busy)
	}
	m = deliver(t, m, key("enter"))
	view := m.View()
	for _, want := range []string{"── reply ──", "All 42 tests passed.", "idle"} {
		if !strings.Contains(view, want) {
			t.Errorf("reply view lacks %q:\n%s", want, view)
		}
	}
	if len(f.asked) != 1 || f.asked[0] != "run the tests" {
		t.Errorf("Ask calls = %v", f.asked)
	}
	m = send(t, m, key("x"))
	if strings.Contains(m.View(), "All 42") || !strings.Contains(m.View(), "importer") {
		t.Errorf("any key should come back to the list:\n%s", m.View())
	}
}

func TestAskSaysWhenItShowsTheLastLinesOrTheAgentIsBlocked(t *testing.T) {
	t.Parallel()

	f := &fakeActions{reply: tui.Reply{Status: "idle", Text: "some screen", AfterMessage: false, Lines: 40}}
	m := deliver(t, typed(t, send(t, withActions(t, f), ctrl("a")), "hi"), key("enter"))
	if !strings.Contains(m.View(), "(last 40 lines)") {
		t.Errorf("the fallback must be announced:\n%s", m.View())
	}

	f = &fakeActions{reply: tui.Reply{Status: "blocked", Text: "Run it?\nProceed? (y/n)"}}
	m = deliver(t, typed(t, send(t, withActions(t, f), ctrl("a")), "hi"), key("enter"))
	if !strings.Contains(m.View(), "waiting for you: Proceed? (y/n)") {
		t.Errorf("a blocked agent should be explained:\n%s", m.View())
	}
}

func TestEscapeWhileWaitingStopsWaitingAndIgnoresTheLateReply(t *testing.T) {
	t.Parallel()

	f := &fakeActions{reply: tui.Reply{Status: "idle", Text: "late answer", AfterMessage: true}}
	m := typed(t, send(t, withActions(t, f), ctrl("a")), "hi")
	m2, cmd := m.Update(key("enter"))
	m = m2.(tui.Model)
	m = send(t, m, key("esc"))
	if !strings.Contains(m.View(), "stopped waiting") || !strings.Contains(m.View(), "importer") {
		t.Errorf("esc should return to the list and say so:\n%s", m.View())
	}
	for _, out := range runCmd(t, cmd) {
		if _, tick := out.(tui.BusyTickMsg); !tick {
			m = send(t, m, out)
		}
	}
	if strings.Contains(m.View(), "late answer") {
		t.Errorf("a reply that arrives after esc must be ignored:\n%s", m.View())
	}
}

func TestActionErrorsAreShownInRed(t *testing.T) {
	t.Parallel()

	f := &fakeActions{sayErr: errors.New("agent is waiting for approval")}
	m := deliver(t, typed(t, send(t, withActions(t, f), ctrl("s")), "hi"), key("enter"))
	if !strings.Contains(m.View(), "say failed: agent is waiting for approval") {
		t.Errorf("the failure should be shown:\n%s", m.View())
	}
}

func TestWaitAndNotifyRunsInTheBackground(t *testing.T) {
	t.Parallel()

	f := &fakeActions{waitStatus: "idle"}
	m := send(t, withActions(t, f), key("enter"))
	m2, cmd := m.Update(key("w"))
	m = m2.(tui.Model)
	if cmd == nil || !strings.Contains(m.View(), "waiting for local/wS:p1") {
		t.Fatalf("the wait should start and the list stay usable (cmd=%v):\n%s", cmd != nil, m.View())
	}
	m = send(t, m, key("j"))
	if got := breadcrumb(t, m); got != "local / raw / wS:p4" {
		t.Errorf("the list must stay usable while waiting: %q", got)
	}
	for _, out := range runCmd(t, cmd) {
		m = send(t, m, out)
	}
	if f.waited != 1 || !strings.Contains(m.View(), "importer is idle") {
		t.Errorf("the end of the wait should be reported (waited=%d):\n%s", f.waited, m.View())
	}
}

func TestLookViewScrolls(t *testing.T) {
	t.Parallel()

	var lines []string
	for i := 1; i <= 100; i++ {
		lines = append(lines, fmt.Sprintf("output line %03d", i))
	}
	f := &fakeActions{lookText: strings.Join(lines, "\n")}
	m := deliver(t, withActions(t, f), ctrl("l"))
	if !strings.Contains(m.View(), "output line 001") || strings.Contains(m.View(), "output line 100") {
		t.Fatalf("the view should start at the top:\n%s", m.View())
	}
	m = send(t, m, key("pgdown"))
	m = send(t, m, key("pgdown"))
	m = send(t, m, key("pgdown"))
	m = send(t, m, key("pgdown"))
	if strings.Contains(m.View(), "output line 001") || !strings.Contains(m.View(), "output line 100") {
		t.Errorf("paging down should reach the end:\n%s", m.View())
	}
}

func TestEveryModeKeepsTheFrameTheExactSize(t *testing.T) {
	t.Parallel()

	f := &fakeActions{lookText: "a very long line " + strings.Repeat("x", 300), reply: tui.Reply{Status: "idle", Text: "ok", AfterMessage: true}}
	modes := map[string]func(*testing.T) tui.Model{
		"menu": func(t *testing.T) tui.Model { return send(t, withActions(t, f), key("enter")) },
		"say":  func(t *testing.T) tui.Model { return send(t, withActions(t, f), ctrl("s")) },
		"look": func(t *testing.T) tui.Model { return deliver(t, withActions(t, f), ctrl("l")) },
		"reply": func(t *testing.T) tui.Model {
			return deliver(t, typed(t, send(t, withActions(t, f), ctrl("a")), "q"), key("enter"))
		},
		"busy": func(t *testing.T) tui.Model {
			m2, _ := typed(t, send(t, withActions(t, f), ctrl("a")), "q").Update(key("enter"))
			return m2.(tui.Model)
		},
	}
	for name, build := range modes {
		m := build(t)
		for _, size := range [][2]int{{100, 30}, {40, 14}} {
			m = send(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			lines := strings.Split(m.View(), "\n")
			if len(lines) != size[1] {
				t.Errorf("%s %dx%d: %d lines", name, size[0], size[1], len(lines))
			}
			for i, l := range lines {
				if w := ui.VisibleLen(l); w != size[0] {
					t.Errorf("%s %dx%d: line %d is %d wide: %q", name, size[0], size[1], i, w, l)
				}
			}
		}
	}
}

func TestWithoutActionsTheMenuSaysSo(t *testing.T) {
	t.Parallel()

	m := send(t, start(t, plain(), 100, 30), key("enter"))
	m = send(t, m, key("s"))
	m = typed(t, m, "hi")
	m = send(t, m, key("enter"))
	if !strings.Contains(m.View(), "not available") {
		t.Errorf("a selector without actions should explain why nothing happened:\n%s", m.View())
	}
}

// Lo que imprime un agente no es de fiar: no puede llevar secuencias que muevan
// el cursor, borren la pantalla o cambien la terminal.
func TestAgentOutputCannotInjectTerminalControlSequences(t *testing.T) {
	t.Parallel()

	f := &fakeActions{lookText: "safe\x1b[2Jtext\x07 and\ttabs\r\n\x1b]0;owned\x07end"}
	m := deliver(t, withActions(t, f), ctrl("l"))
	view := m.View()
	for _, bad := range []string{"\x1b", "\x07", "\r"} {
		if strings.Contains(view, bad) {
			t.Errorf("view contains the control character %q:\n%q", bad, view)
		}
	}
	if !strings.Contains(view, "safe[2Jtext") || !strings.Contains(view, "and    tabs") {
		t.Errorf("the visible text should survive:\n%s", view)
	}
}
