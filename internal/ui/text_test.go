package ui_test

import (
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/ui"
)

func TestVisibleLen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want int
	}{
		{"ascii", "hello", 5},
		{"accents count once", "añadí", 5},
		{"ansi is invisible", "\x1b[38;2;1;2;3mhi\x1b[0m", 2},
		{"wide cjk is two cells", "日本", 4},
		{"emoji is two cells", "a🚀", 3},
		{"combining mark is zero", "é", 1},
		{"box glyphs", "├─ ○", 4},
		{"empty", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ui.VisibleLen(tt.in); got != tt.want {
				t.Errorf("VisibleLen(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		in    string
		width int
		ell   string
		want  string
	}{
		{"fits", "hello", 5, "…", "hello"},
		{"cut", "hello world", 6, "…", "hello…"},
		{"ascii ellipsis", "hello world", 8, "...", "hello..."},
		{"width one", "hello", 1, "…", "…"},
		{"zero width", "hello", 0, "…", ""},
		{"wide runes never overflow", "日本語", 4, "…", "日…"},
		{"ansi kept and closed", "\x1b[31mred text\x1b[0m", 4, "…", "\x1b[31mred\x1b[0m…"},
		{"ansi fits untouched", "\x1b[31mred\x1b[0m", 3, "…", "\x1b[31mred\x1b[0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ui.Truncate(tt.in, tt.width, tt.ell)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
			}
			if ui.VisibleLen(got) > tt.width {
				t.Errorf("Truncate(%q, %d) is %d cells wide", tt.in, tt.width, ui.VisibleLen(got))
			}
		})
	}
}

func TestFitRowAlignsTheRightColumnsFlushRight(t *testing.T) {
	t.Parallel()

	got := ui.FitRow(20, "name", "codex idle", "…")
	if want := "name      codex idle"; got != want { // 4 + 6 espacios + 10 = 20
		t.Errorf("FitRow() = %q, want %q", got, want)
	}
}

func TestFitRowTruncatesTheLeftSideNotTheRight(t *testing.T) {
	t.Parallel()

	got := ui.FitRow(16, "a very long agent name", "codex idle", "…")
	// 16 celdas = izquierda (4) + hueco (2) + derecha (10).
	if want := "a v…  codex idle"; got != want {
		t.Errorf("FitRow() = %q, want %q", got, want)
	}
}

func TestFitRowWithoutWidthJoinsWithAGap(t *testing.T) {
	t.Parallel()

	if got, want := ui.FitRow(0, "left", "right", "…"), "left  right"; got != want {
		t.Errorf("FitRow(0) = %q, want %q", got, want)
	}
}

func TestFitRowCountsAnsiAsInvisible(t *testing.T) {
	t.Parallel()

	left := "\x1b[1mws\x1b[0m"
	got := ui.FitRow(10, left, "x", "…")
	if ui.VisibleLen(got) != 10 || !strings.HasPrefix(got, left) {
		t.Errorf("FitRow() = %q (%d cells), want 10 cells starting with the styled text", got, ui.VisibleLen(got))
	}
}

func TestFitRowNeverExceedsTheWidth(t *testing.T) {
	t.Parallel()

	for width := 1; width <= 30; width++ {
		got := ui.FitRow(width, "a long left side", "right side", "…")
		if ui.VisibleLen(got) > width {
			t.Errorf("FitRow(%d) = %q is %d cells wide", width, got, ui.VisibleLen(got))
		}
	}
}

func TestPadRight(t *testing.T) {
	t.Parallel()

	if got := ui.PadRight("ab", 5); got != "ab   " {
		t.Errorf("PadRight() = %q", got)
	}
	if got := ui.PadRight("abcdef", 3); got != "abcdef" {
		t.Errorf("PadRight() must not cut, got %q", got)
	}
	if got := ui.PadRight("\x1b[1mab\x1b[0m", 4); ui.VisibleLen(got) != 4 {
		t.Errorf("PadRight() ignores ansi width, got %q", got)
	}
}

func TestWidthFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		detected int
		env      map[string]string
		want     int
	}{
		{"detected wins", 120, map[string]string{"COLUMNS": "50"}, 120},
		{"columns when not detected", 0, map[string]string{"COLUMNS": "100"}, 100},
		{"invalid columns", 0, map[string]string{"COLUMNS": "wide"}, 80},
		{"non-positive columns", 0, map[string]string{"COLUMNS": "0"}, 80},
		{"nothing known", 0, nil, 80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ui.WidthFrom(tt.detected, envOf(tt.env)); got != tt.want {
				t.Errorf("WidthFrom() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPadLeft(t *testing.T) {
	t.Parallel()

	if got := ui.PadLeft("ab", 5); got != "   ab" {
		t.Errorf("PadLeft() = %q", got)
	}
	if got := ui.PadLeft("abcdef", 3); got != "abcdef" {
		t.Errorf("PadLeft() must not cut, got %q", got)
	}
	if got := ui.PadLeft("\x1b[1mab\x1b[0m", 4); ui.VisibleLen(got) != 4 || !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("PadLeft() must pad outside the styled text, got %q", got)
	}
}
