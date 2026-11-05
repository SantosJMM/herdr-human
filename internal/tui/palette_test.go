package tui_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/palette"
	"github.com/SantosJMM/herdr-human/internal/tui"
)

func pluginEntry(id, title, source string) palette.Entry {
	return palette.Entry{Kind: palette.KindPlugin, ID: id, Title: title, Source: source, PluginID: "p", ActionID: id}
}

func nativeEntry(id, title string) palette.Entry {
	return palette.Entry{Kind: palette.KindNative, ID: id, Title: title, Source: "Herdr", Command: []string{"x", id}}
}

func startPalette(t *testing.T, o tui.PaletteOptions) tui.PaletteModel {
	t.Helper()
	o.Style = plain()
	m := tui.NewPalette(o)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	return next.(tui.PaletteModel)
}

func palSend(t *testing.T, m tui.PaletteModel, msgs ...tea.Msg) (tui.PaletteModel, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, msg := range msgs {
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(tui.PaletteModel)
	}
	return m, cmd
}

func typeRunes(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestPaletteFirstFrameNeedsNoWaiting(t *testing.T) {
	updates := make(chan tui.PluginUpdate) // nadie contesta: la paleta ya debe dibujar
	m := startPalette(t, tui.PaletteOptions{
		Entries: []palette.Entry{nativeEntry("a", "Workspace: Next"), pluginEntry("p.b", "Open thing", "Thing")},
		Updates: updates,
	})
	view := m.View()
	for _, want := range []string{"Command palette", "Workspace: Next", "Open thing", "2/2", "loading plugins"} {
		if !strings.Contains(view, want) {
			t.Errorf("first frame lacks %q:\n%s", want, view)
		}
	}
}

func TestPaletteTypingFiltersAndSelectionMoves(t *testing.T) {
	m := startPalette(t, tui.PaletteOptions{Entries: []palette.Entry{
		nativeEntry("a", "Tab: Close"), nativeEntry("b", "Tab: New"), nativeEntry("c", "Pane: Close"),
	}})
	m, _ = palSend(t, m, typeRunes("clo"))
	view := m.View()
	if !strings.Contains(view, "2/3") || strings.Contains(view, "Tab: New") {
		t.Errorf("typing filters the list:\n%s", view)
	}
	m, _ = palSend(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, cmd := palSend(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("choosing an entry must leave the palette")
	}
	got := m.Result()
	if !got.Chosen || got.Entry.ID != "a" && got.Entry.ID != "c" {
		t.Errorf("unexpected result %+v", got)
	}
}

func TestPaletteNoMatchAndCtrlWAndCtrlU(t *testing.T) {
	m := startPalette(t, tui.PaletteOptions{Entries: []palette.Entry{nativeEntry("a", "Tab: Close")}})
	m, _ = palSend(t, m, typeRunes("zzz"))
	if !strings.Contains(m.View(), "No entry matches") {
		t.Errorf("empty result is explained:\n%s", m.View())
	}
	m, _ = palSend(t, m, tea.KeyMsg{Type: tea.KeyCtrlU}, typeRunes("tab clo"), tea.KeyMsg{Type: tea.KeyCtrlW})
	if !strings.Contains(m.View(), "❯ tab ") || strings.Contains(m.View(), "❯ tab clo") {
		t.Errorf("ctrl+w deletes the last word:\n%s", m.View())
	}
	m, _ = palSend(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if !strings.Contains(m.View(), "1/1") {
		t.Errorf("ctrl+u clears the query:\n%s", m.View())
	}
}

func TestPaletteEscQuitsWithoutChoosing(t *testing.T) {
	m := startPalette(t, tui.PaletteOptions{Entries: []palette.Entry{nativeEntry("a", "A")}})
	m, cmd := palSend(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil || m.Result().Chosen {
		t.Errorf("esc closes and chooses nothing: %+v", m.Result())
	}
}

func TestPalettePluginsArriveLaterAndKeepTheSelection(t *testing.T) {
	merge := func(plugins []palette.Entry) []palette.Entry {
		return append([]palette.Entry{nativeEntry("a", "Alpha"), nativeEntry("c", "Gamma")}, plugins...)
	}
	m := startPalette(t, tui.PaletteOptions{
		Entries: []palette.Entry{nativeEntry("a", "Alpha"), nativeEntry("c", "Gamma")},
		Updates: make(chan tui.PluginUpdate),
		Merge:   merge,
	})
	m, _ = palSend(t, m, tea.KeyMsg{Type: tea.KeyDown}) // sobre Gamma
	m, _ = palSend(t, m, tui.PluginUpdate{Entries: []palette.Entry{pluginEntry("p.z", "Zulu", "Z")}})
	view := m.View()
	if strings.Contains(view, "loading") || !strings.Contains(view, "Zulu") || !strings.Contains(view, "3/3") {
		t.Errorf("late plugin entries appear and loading ends:\n%s", view)
	}
	m, _ = palSend(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.Result().Entry.ID != "c" {
		t.Errorf("the selection stays on the same entry when the list grows, got %q", m.Result().Entry.ID)
	}
}

func TestPaletteFailedPluginRefreshKeepsWhatWasShown(t *testing.T) {
	m := startPalette(t, tui.PaletteOptions{
		Entries: []palette.Entry{nativeEntry("a", "Alpha")},
		Updates: make(chan tui.PluginUpdate),
		Merge:   func(p []palette.Entry) []palette.Entry { return p },
	})
	m, _ = palSend(t, m, tui.PluginUpdate{Err: errors.New("herdr down")})
	view := m.View()
	if !strings.Contains(view, "Alpha") || !strings.Contains(view, "plugins unavailable") {
		t.Errorf("a failed refresh keeps the list and says so:\n%s", view)
	}
}

func TestPalettePluginActionIsReturnedWithoutResolving(t *testing.T) {
	resolved := false
	m := startPalette(t, tui.PaletteOptions{
		Entries: []palette.Entry{pluginEntry("p.a", "Do it", "P")},
		Resolve: func(context.Context, palette.Entry, palette.Prompter) ([]string, bool, error) {
			resolved = true
			return nil, true, nil
		},
	})
	m, _ = palSend(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if resolved || !m.Result().Chosen || m.Result().Entry.ActionID != "p.a" || m.Result().Argv != nil {
		t.Errorf("plugin actions need no argument resolution: %+v resolved=%v", m.Result(), resolved)
	}
}

// --- flujo interactivo con un programa real ---

// screen acumula lo que el programa dibuja y deja esperar a que aparezca un texto.
type screen struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *screen) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		ok := strings.Contains(s.buf.String(), want)
		s.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q on screen", want)
}

type runResult struct {
	res tui.PaletteResult
	err error
}

func runPalette(t *testing.T, o tui.PaletteOptions) (in *io.PipeWriter, scr *screen, done <-chan runResult) {
	t.Helper()
	pr, pw := io.Pipe()
	scr = &screen{}
	o.Style = plain()
	ch := make(chan runResult, 1)
	go func() {
		res, err := tui.RunPalette(context.Background(), pr, scr, o)
		ch <- runResult{res, err}
	}()
	t.Cleanup(func() { _ = pw.Close() })
	return pw, scr, ch
}

func finish(t *testing.T, done <-chan runResult) runResult {
	t.Helper()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatal(r.err)
		}
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("the palette did not exit")
		return runResult{}
	}
}

func TestRunPaletteAsksForInputAndReturnsTheCommand(t *testing.T) {
	entry := nativeEntry("ws.new", "Workspace: New")
	in, scr, done := runPalette(t, tui.PaletteOptions{
		Entries: []palette.Entry{entry},
		Resolve: func(ctx context.Context, e palette.Entry, p palette.Prompter) ([]string, bool, error) {
			text, ok, err := p.Input(ctx, palette.InputSpec{Prompt: "workspace directory", Initial: "C:/a", Required: true})
			if err != nil || !ok {
				return nil, false, err
			}
			return append(slices.Clone(e.Command), "--cwd", text), true, nil
		},
	})
	scr.waitFor(t, "Workspace: New")
	_, _ = in.Write([]byte("\r"))
	scr.waitFor(t, "workspace directory")
	_, _ = in.Write([]byte("b\r"))
	r := finish(t, done)
	if !r.res.Chosen || !slices.Equal(r.res.Argv, []string{"x", "ws.new", "--cwd", "C:/ab"}) {
		t.Errorf("got %+v", r.res)
	}
}

func TestRunPaletteRequiredInputRefusesEmpty(t *testing.T) {
	in, scr, done := runPalette(t, tui.PaletteOptions{
		Entries: []palette.Entry{nativeEntry("n", "Name it")},
		Resolve: func(ctx context.Context, e palette.Entry, p palette.Prompter) ([]string, bool, error) {
			text, ok, err := p.Input(ctx, palette.InputSpec{Prompt: "new name", Required: true})
			return []string{text}, ok, err
		},
	})
	scr.waitFor(t, "Name it")
	_, _ = in.Write([]byte("\r"))
	scr.waitFor(t, "new name")
	_, _ = in.Write([]byte("\r"))
	scr.waitFor(t, "This value is required.")
	_, _ = in.Write([]byte("z\r"))
	if r := finish(t, done); !r.res.Chosen || r.res.Argv[0] != "z" {
		t.Errorf("got %+v", r.res)
	}
}

func TestRunPaletteChooseFromList(t *testing.T) {
	in, scr, done := runPalette(t, tui.PaletteOptions{
		Entries: []palette.Entry{nativeEntry("sw", "Workspace: Switch")},
		Resolve: func(ctx context.Context, e palette.Entry, p palette.Prompter) ([]string, bool, error) {
			v, ok, err := p.Choose(ctx, "workspace", []palette.Choice{{Value: "w1", Label: "alpha"}, {Value: "w2", Label: "beta"}})
			return []string{"focus", v}, ok, err
		},
	})
	scr.waitFor(t, "Workspace: Switch")
	_, _ = in.Write([]byte("\r"))
	scr.waitFor(t, "beta")
	_, _ = in.Write([]byte("be"))
	scr.waitFor(t, "1/2")
	_, _ = in.Write([]byte("\r"))
	if r := finish(t, done); !slices.Equal(r.res.Argv, []string{"focus", "w2"}) {
		t.Errorf("the list filters and returns the value, got %+v", r.res)
	}
}

func TestRunPaletteEscInAQuestionAbortsEverything(t *testing.T) {
	in, scr, done := runPalette(t, tui.PaletteOptions{
		Entries: []palette.Entry{nativeEntry("n", "Ask me")},
		Resolve: func(ctx context.Context, e palette.Entry, p palette.Prompter) ([]string, bool, error) {
			_, ok, err := p.Input(ctx, palette.InputSpec{Prompt: "say something"})
			return []string{"never"}, ok, err
		},
	})
	scr.waitFor(t, "Ask me")
	_, _ = in.Write([]byte("\r"))
	scr.waitFor(t, "say something")
	_, _ = in.Write([]byte("\x1b"))
	if r := finish(t, done); r.res.Chosen {
		t.Errorf("Esc in a question must run nothing, got %+v", r.res)
	}
}

func TestRunPaletteShowsResolveErrorsInsteadOfClosingSilently(t *testing.T) {
	boom := errors.New("nothing to choose for \"tab\"")
	in, scr, done := runPalette(t, tui.PaletteOptions{
		Entries: []palette.Entry{nativeEntry("n", "Fail me")},
		Resolve: func(context.Context, palette.Entry, palette.Prompter) ([]string, bool, error) {
			return nil, false, boom
		},
	})
	scr.waitFor(t, "Fail me")
	_, _ = in.Write([]byte("\r"))
	scr.waitFor(t, "nothing to choose")
	_, _ = in.Write([]byte("q"))
	r := finish(t, done)
	if r.res.Chosen || !errors.Is(r.res.Err, boom) {
		t.Errorf("the error is shown, then returned: %+v", r.res)
	}
}

func TestPaletteTopResultFollowsTheBestMatchWhenPluginsArrive(t *testing.T) {
	merge := func(plugins []palette.Entry) []palette.Entry {
		return append([]palette.Entry{nativeEntry("n", "Pane: Split ping")}, plugins...)
	}
	m := startPalette(t, tui.PaletteOptions{
		Entries: []palette.Entry{nativeEntry("n", "Pane: Split ping")},
		Updates: make(chan tui.PluginUpdate),
		Merge:   merge,
	})
	m, _ = palSend(t, m, typeRunes("ping"))
	m, _ = palSend(t, m, tui.PluginUpdate{Entries: []palette.Entry{pluginEntry("p.ping", "Ping", "Plug")}})
	m, _ = palSend(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.Result().Entry.ID; got != "p.ping" {
		t.Errorf("an untouched selection follows the best match, got %q", got)
	}
}
