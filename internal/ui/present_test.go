package ui_test

import (
	"os"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/ui"
)

func TestShortNameKeepsTheFirstSegmentUnlessFull(t *testing.T) {
	t.Parallel()

	const codex = "Analiza login | odoo-inc | 5h 8% left"
	if got := ui.ShortName(codex, false); got != "Analiza login" {
		t.Errorf("ShortName = %q", got)
	}
	if got := ui.ShortName(codex, true); got != codex {
		t.Errorf("ShortName full = %q", got)
	}
	if got := ui.ShortName("importer", false); got != "importer" {
		t.Errorf("ShortName plain = %q", got)
	}
	if got := ui.ShortName(" | only tail", false); got != " | only tail" {
		t.Errorf("ShortName with an empty first segment = %q, want the original", got)
	}
}

func TestStatusRoleMapsEveryKnownStatus(t *testing.T) {
	t.Parallel()

	for status, want := range map[string]ui.Role{
		"blocked": ui.RoleBlocked, "working": ui.RoleWorking, "done": ui.RoleDone,
		"idle": ui.RoleIdle, "unknown": ui.RoleDim, "": ui.RoleDim,
	} {
		if got := ui.StatusRole(status); got != want {
			t.Errorf("StatusRole(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestFormatElapsed(t *testing.T) {
	t.Parallel()

	if got := ui.FormatElapsed(180 * time.Millisecond); got != "180ms" {
		t.Errorf("got %q", got)
	}
	if got := ui.FormatElapsed(1300 * time.Millisecond); got != "1.3s" {
		t.Errorf("got %q", got)
	}
}

func TestStyleReportsItsCapability(t *testing.T) {
	t.Parallel()

	plain := ui.NewStyle(ui.Capability{}, ui.Theme{})
	if plain.Color() || plain.ASCII() {
		t.Errorf("plain style: Color=%v ASCII=%v, want both false", plain.Color(), plain.ASCII())
	}
	rich := ui.NewStyle(ui.Capability{Color: true, ASCII: true}, ui.Theme{})
	if !rich.Color() || !rich.ASCII() {
		t.Errorf("rich style: Color=%v ASCII=%v, want both true", rich.Color(), rich.ASCII())
	}
}

func TestSpinnerCyclesWithTheElapsedTime(t *testing.T) {
	t.Parallel()

	step := 100 * time.Millisecond
	if ui.Spinner(false, 0) == ui.Spinner(false, step) {
		t.Errorf("consecutive frames must differ")
	}
	if ui.Spinner(false, 0) != ui.Spinner(false, 10*step) {
		t.Errorf("the spinner should loop after ten frames")
	}
	if got := ui.Spinner(true, 0); got != "|" || ui.Spinner(true, step) != "/" {
		t.Errorf("ASCII spinner = %q, %q", got, ui.Spinner(true, step))
	}
	if ui.Spinner(false, -time.Second) == "" {
		t.Errorf("a negative duration must still give a frame")
	}
}

func TestHeightIsZeroWhenTheFileIsNotATerminal(t *testing.T) {
	t.Parallel()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if h := ui.Height(w); h != 0 {
		t.Errorf("Height(pipe) = %d, want 0 (unknown)", h)
	}
}
