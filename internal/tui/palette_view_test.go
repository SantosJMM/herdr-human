package tui_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/palette"
	"github.com/SantosJMM/herdr-human/internal/tui"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// accent es lo que se escribe antes de una letra resaltada: negrita y color de acento.
const accent = "\x1b[1m\x1b[36m"

func colorPalette(t *testing.T, o tui.PaletteOptions, width int) tui.PaletteModel {
	t.Helper()
	o.Style = ui.NewStyle(ui.Capability{Color: true}, ui.Theme{})
	next, _ := tui.NewPalette(o).Update(tea.WindowSizeMsg{Width: width, Height: 20})
	return next.(tui.PaletteModel)
}

func described(id, title, desc string) palette.Entry {
	e := nativeEntry(id, title)
	e.Description = desc
	return e
}

func TestPaletteHighlightsTheLettersTheFilterMatched(t *testing.T) {
	m := colorPalette(t, tui.PaletteOptions{Entries: []palette.Entry{described("tab.close", "Tab: Close current", "Close it.")}}, 80)
	if strings.Contains(m.View(), accent) {
		t.Error("with no query there is nothing to highlight")
	}
	m, _ = palSend(t, m, typeRunes("clo"))
	view := m.View()
	if !strings.Contains(view, accent+"Clo") {
		t.Errorf("the matched letters are in bold accent:\n%q", view)
	}
	if strings.Contains(view, accent+"se") || strings.Contains(view, accent+"Tab") {
		t.Errorf("only the matched letters are highlighted:\n%q", view)
	}
}

func TestPaletteHighlightsScatteredLettersAndTheSourceColumn(t *testing.T) {
	m := colorPalette(t, tui.PaletteOptions{Entries: []palette.Entry{pluginEntry("p.ping", "Ping thing", "Plugin")}}, 80)
	m, _ = palSend(t, m, typeRunes("pg"))
	view := m.View()
	// "pg" cae en P…g: dos trozos, no uno.
	if !strings.Contains(view, accent+"P") || !strings.Contains(view, accent+"g") {
		t.Errorf("each matched letter is highlighted where it falls:\n%q", view)
	}
	m, _ = palSend(t, m, tea.KeyMsg{Type: tea.KeyCtrlU}, typeRunes("plu"))
	if !strings.Contains(m.View(), accent+"Plu") {
		t.Errorf("a match in the source column is highlighted there:\n%q", m.View())
	}
}

func TestPaletteHighlightIsConsistentWithWhatIsListed(t *testing.T) {
	// El texto buscado incluye la descripción y el id: una entrada puede aparecer
	// sin que su título tenga letras coincidentes. Aparece, y no se resalta de más.
	m := colorPalette(t, tui.PaletteOptions{Entries: []palette.Entry{described("a", "Alpha", "zzqq secret")}}, 80)
	m, _ = palSend(t, m, typeRunes("zzqq"))
	view := m.View()
	if !strings.Contains(view, "Alpha") || !strings.Contains(view, "1/1") {
		t.Errorf("a match in the description still lists the entry:\n%q", view)
	}
	if strings.Contains(view, accent+"A") {
		t.Errorf("the title has none of the matched letters:\n%q", view)
	}
}

func TestPaletteHighlightsInTheChoiceList(t *testing.T) {
	in, scr, done := runPalette(t, tui.PaletteOptions{
		Entries: []palette.Entry{nativeEntry("sw", "Workspace: Switch")},
		Resolve: chooseTwo,
	})
	scr.waitFor(t, "Workspace: Switch")
	_, _ = in.Write([]byte("\r"))
	scr.waitFor(t, "beta")
	_, _ = in.Write([]byte("be\r"))
	if r := finish(t, done); r.res.Argv[1] != "w2" {
		t.Errorf("got %+v", r.res)
	}
}

func TestPaletteRowsKeepTheirWidthWithColorsAndTruncation(t *testing.T) {
	long := described("x", "A very long command title that cannot possibly fit in a narrow popup", "Description that is also long enough to need cutting off at the edge")
	m := colorPalette(t, tui.PaletteOptions{Entries: []palette.Entry{long, pluginEntry("p.y", "Another quite long plugin action title here", "Some Plugin Name")}}, 40)
	for _, query := range []string{"", "very", "xz", "pl"} {
		m, _ = palSend(t, m, tea.KeyMsg{Type: tea.KeyCtrlU}, typeRunes(query))
		for i, line := range strings.Split(m.View(), "\n") {
			if got := ui.VisibleLen(line); got != 40 {
				t.Errorf("query %q line %d is %d cells wide, want 40: %q", query, i, got, line)
			}
		}
	}
}

func TestPaletteShowsTheDescriptionOfTheSelectedEntry(t *testing.T) {
	m := startPalette(t, tui.PaletteOptions{Entries: []palette.Entry{
		described("a", "Alpha", "Does the alpha thing."),
		described("b", "Beta", "Does the beta thing."),
		pluginEntry("p.c", "Gamma", "Plug"),
	}})
	if v := m.View(); !strings.Contains(v, "Does the alpha thing.") || strings.Contains(v, "beta thing") {
		t.Errorf("the first entry is selected and described:\n%s", v)
	}
	m, _ = palSend(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if v := m.View(); !strings.Contains(v, "Does the beta thing.") || strings.Contains(v, "alpha thing") {
		t.Errorf("the description follows the selection:\n%s", v)
	}
	m, _ = palSend(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if v := m.View(); !strings.Contains(v, "Plugin action p.c") {
		t.Errorf("a plugin action without a description shows what it is bound by:\n%s", v)
	}
}

func TestPaletteDescriptionClearsWhenNothingIsSelected(t *testing.T) {
	m := startPalette(t, tui.PaletteOptions{Entries: []palette.Entry{described("a", "Alpha", "Does the alpha thing.")}})
	m, _ = palSend(t, m, typeRunes("zzz"))
	if v := m.View(); strings.Contains(v, "alpha thing") {
		t.Errorf("no match, no stale description:\n%s", v)
	}
}

func chooseTwo(ctx context.Context, e palette.Entry, p palette.Prompter) ([]string, bool, error) {
	v, ok, err := p.Choose(ctx, "workspace", []palette.Choice{{Value: "w1", Label: "alpha"}, {Value: "w2", Label: "beta"}})
	return []string{"focus", v}, ok, err
}
