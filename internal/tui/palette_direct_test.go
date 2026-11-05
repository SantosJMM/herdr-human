package tui_test

import (
	"testing"

	"github.com/SantosJMM/herdr-human/internal/palette"
	"github.com/SantosJMM/herdr-human/internal/tui"
)

// Un atajo asignado a un comando que pregunta abre la paleta ya dentro de él
// (StartWith): se ve la primera pregunta sin buscar ni elegir la fila.

func TestStartWithEntersTheEntryWithoutAnyKeyPress(t *testing.T) {
	in, scr, done := runPalette(t, tui.PaletteOptions{
		Entries:   []palette.Entry{nativeEntry("other", "Other: Thing"), nativeEntry("sw", "Workspace: Switch")},
		StartWith: "sw",
		Resolve:   chooseTwo,
	})
	scr.waitFor(t, "beta") // la lista de elección del comando, sin haber tecleado nada
	_, _ = in.Write([]byte("be\r"))
	r := finish(t, done)
	if !r.res.Chosen || r.res.Entry.ID != "sw" || len(r.res.Argv) != 2 || r.res.Argv[1] != "w2" {
		t.Errorf("the entry runs as if chosen from the list: %+v", r.res)
	}
}

func TestStartWithAsksInputToo(t *testing.T) {
	in, scr, done := runPalette(t, tui.PaletteOptions{
		Entries:   []palette.Entry{nativeEntry("n", "Name it")},
		StartWith: "n",
		Resolve:   inputEntry("new name", "", true).Resolve,
	})
	scr.waitFor(t, "new name")
	_, _ = in.Write([]byte("docs\r"))
	if r := finish(t, done); !r.res.Chosen || r.res.Argv[0] != "docs" {
		t.Errorf("%+v", r.res)
	}
}

func TestEscOnTheFirstQuestionClosesEverythingWhenOpenedThisWay(t *testing.T) {
	in, scr, done := runPalette(t, tui.PaletteOptions{
		Entries:   []palette.Entry{nativeEntry("sw", "Workspace: Switch")},
		StartWith: "sw",
		Resolve:   chooseTwo,
	})
	scr.waitFor(t, "beta")
	_, _ = in.Write([]byte("\x1b"))
	if r := finish(t, done); r.res.Chosen {
		t.Errorf("Esc cancels the whole command and the palette, got %+v", r.res)
	}
}

func TestStartWithAnUnknownEntryOpensTheNormalListWithANotice(t *testing.T) {
	in, scr, done := runPalette(t, tui.PaletteOptions{
		Entries:   []palette.Entry{nativeEntry("a", "Alpha thing")},
		StartWith: "gone.entry",
		Resolve:   chooseTwo,
	})
	scr.waitFor(t, "Alpha thing")
	scr.waitFor(t, "«gone.entry» is not available from here")
	_, _ = in.Write([]byte("\x1b"))
	if r := finish(t, done); r.res.Chosen {
		t.Errorf("nothing runs: %+v", r.res)
	}
}
