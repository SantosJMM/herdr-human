package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/palette"
	"github.com/SantosJMM/herdr-human/internal/tui"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// Dentro de un popup, herdr ya dibuja el marco y el título del pane: una segunda
// caja encima queda como una caja dentro de otra.

const boxChars = "┌┐└┘│├┤"

// hasBox dice si la vista dibuja un borde: una línea que empieza o acaba con un
// carácter de marco. Las ramas del arbol (├─ └─) van dentro de la línea, no en el borde.
func hasBox(view string) bool {
	for _, line := range strings.Split(view, "\n") {
		r := []rune(strings.TrimRight(line, " "))
		if len(r) > 0 && (strings.ContainsRune(boxChars, r[0]) || strings.ContainsRune(boxChars, r[len(r)-1])) {
			return true
		}
	}
	return false
}

func manyEntries(n int) []palette.Entry {
	entries := make([]palette.Entry, n)
	for i := range entries {
		entries[i] = described("e"+strings.Repeat("x", i), "Entry "+strings.Repeat("x", i), "Does thing.")
	}
	return entries
}

func paletteView(t *testing.T, embedded bool, width, height int, entries []palette.Entry) string {
	t.Helper()
	m := tui.NewPalette(tui.PaletteOptions{Style: plain(), Entries: entries, Embedded: embedded})
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(tui.PaletteModel).View()
}

func TestPaletteEmbeddedDrawsNoFrameAndNoTitle(t *testing.T) {
	framed := paletteView(t, false, 80, 20, manyEntries(3))
	if !hasBox(framed) || !strings.Contains(framed, "Command palette") {
		t.Fatalf("setup: the standalone palette keeps its own frame and title:\n%s", framed)
	}
	embedded := paletteView(t, true, 80, 20, manyEntries(3))
	if hasBox(embedded) {
		t.Errorf("embedded must not draw a box (herdr draws it):\n%s", embedded)
	}
	if strings.Contains(embedded, "Command palette") {
		t.Errorf("embedded must not repeat the title herdr already shows:\n%s", embedded)
	}
	for _, want := range []string{"❯", "Entry", "Does thing.", "enter run", "3/3", "─"} {
		if !strings.Contains(embedded, want) {
			t.Errorf("embedded still has the search line, the rows, the help and one separator; lacks %q:\n%s", want, embedded)
		}
	}
	if n := strings.Count(embedded, "─"); n < 10 {
		t.Errorf("the separator under the search line is a full-width rule, got %d cells", n)
	}
}

func TestPaletteEmbeddedUsesTheRoomTheFrameWouldTake(t *testing.T) {
	rowsShown := func(view string) int { return strings.Count(view, "Entry ") }
	framed := paletteView(t, false, 80, 14, manyEntries(40))
	embedded := paletteView(t, true, 80, 14, manyEntries(40))
	if rowsShown(embedded) != rowsShown(framed)+3 {
		t.Errorf("without the two borders and the footer rule there are 3 more rows: %d framed, %d embedded",
			rowsShown(framed), rowsShown(embedded))
	}
	if got := len(strings.Split(embedded, "\n")); got != 14 {
		t.Errorf("the view fills exactly the popup height, got %d lines", got)
	}
}

func TestPaletteEmbeddedKeepsItsWidthWithColors(t *testing.T) {
	o := tui.PaletteOptions{Entries: manyEntries(5), Embedded: true}
	o.Style = ui.NewStyle(ui.Capability{Color: true}, ui.Theme{})
	next, _ := tui.NewPalette(o).Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m := next.(tui.PaletteModel)
	for _, query := range []string{"", "ent", "zzz"} {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		m = next.(tui.PaletteModel)
		next, _ = m.Update(typeRunes(query))
		m = next.(tui.PaletteModel)
		for i, line := range strings.Split(m.View(), "\n") {
			if got := ui.VisibleLen(line); got != 40 {
				t.Errorf("query %q line %d is %d cells wide, want 40: %q", query, i, got, line)
			}
		}
	}
}

func TestPaletteEmbeddedStillMarksTheSelectionWithoutColor(t *testing.T) {
	view := paletteView(t, true, 60, 12, manyEntries(2))
	if !strings.Contains(view, "▸Entry") {
		t.Errorf("without colors the selected row carries a marker, even without a frame:\n%s", view)
	}
}

func TestPaletteEmbeddedQuestionsHaveNoFrameEither(t *testing.T) {
	in, scr, done := runPalette(t, tui.PaletteOptions{
		Entries:  []palette.Entry{nativeEntry("sw", "Workspace: Switch")},
		Embedded: true,
		Resolve:  chooseTwo,
	})
	scr.waitFor(t, "Workspace: Switch")
	_, _ = in.Write([]byte("\r"))
	scr.waitFor(t, "beta")
	_, _ = in.Write([]byte("\x1b"))
	finish(t, done)

	scr.mu.Lock()
	text := scr.buf.String()
	scr.mu.Unlock()
	if hasBox(text) {
		t.Errorf("no box in any frame of an embedded palette, including the question lists")
	}
}

// --- selector (hum pick) ---

func embeddedPick(t *testing.T, width, height int) tui.Model {
	t.Helper()
	m := tui.New(tui.Options{Inventory: inv(), Style: plain(), Embedded: true})
	return send(t, m, tea.WindowSizeMsg{Width: width, Height: height})
}

func TestPickEmbeddedDrawsNoFrameAndNoTitle(t *testing.T) {
	t.Parallel()
	framed := start(t, plain(), 100, 30).View()
	if !hasBox(framed) || !strings.Contains(framed, "Talk to") {
		t.Fatalf("setup: the standalone selector keeps its own frame and title:\n%s", framed)
	}
	view := embeddedPick(t, 100, 30).View()
	if hasBox(view) || strings.Contains(view, "Talk to") {
		t.Errorf("embedded must not draw a box or repeat the title:\n%s", view)
	}
	for _, want := range []string{"search agents", "raw", "Pane ID work", "─", "enter actions"} {
		if !strings.Contains(view, want) {
			t.Errorf("embedded keeps the search, the tree and the footer; lacks %q:\n%s", want, view)
		}
	}
	if got := len(strings.Split(view, "\n")); got != 30 {
		t.Errorf("the view fills exactly the popup height, got %d lines", got)
	}
}

func TestPickEmbeddedKeepsTheScrollbarAtTheRightEdge(t *testing.T) {
	t.Parallel()
	view := embeddedPick(t, 80, 10).View()
	thumb := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.HasSuffix(line, "┃") {
			thumb++
		}
		if ui.VisibleLen(line) != 80 {
			t.Errorf("every line keeps the window width: %d %q", ui.VisibleLen(line), line)
		}
	}
	if thumb == 0 {
		t.Errorf("a list longer than the window keeps its scrollbar, as the native herdr views do:\n%s", view)
	}
}

func TestPickEmbeddedClicksLandOnTheRightRow(t *testing.T) {
	t.Parallel()
	m := embeddedPick(t, 100, 30)
	// Sin marco el cuerpo empieza en y=2: Local, raw, p1, p4, demo, p7...
	m = send(t, m, tea.MouseMsg{X: 10, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if got := breadcrumb(t, m); got != "local / raw / wS:p4" {
		t.Errorf("click on p4: %q", got)
	}
	m = send(t, m, tea.MouseMsg{X: 10, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if got := breadcrumb(t, m); got != "local / raw / wS:p4" {
		t.Errorf("a click on a machine header must not move the selection: %q", got)
	}
}
