package ui_test

import (
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/ui"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseMode(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]ui.Mode{"auto": ui.ColorAuto, "always": ui.ColorAlways, "never": ui.ColorNever} {
		got, err := ui.ParseMode(in)
		if err != nil || got != want {
			t.Errorf("ParseMode(%q) = (%v, %v), want %v", in, got, err, want)
		}
	}
	if _, err := ui.ParseMode("sometimes"); err == nil {
		t.Error("ParseMode(sometimes) error = nil, want an error")
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     ui.Mode
		terminal bool
		goos     string
		env      map[string]string
		want     ui.Capability
	}{
		{"terminal on windows", ui.ColorAuto, true, "windows", nil, ui.Capability{Color: true, TrueColor: true}},
		{"terminal on linux without colorterm", ui.ColorAuto, true, "linux", map[string]string{"TERM": "xterm"}, ui.Capability{Color: true}},
		{"colorterm truecolor", ui.ColorAuto, true, "linux", map[string]string{"COLORTERM": "truecolor"}, ui.Capability{Color: true, TrueColor: true}},
		{"colorterm 24bit", ui.ColorAuto, true, "linux", map[string]string{"COLORTERM": "24bit"}, ui.Capability{Color: true, TrueColor: true}},
		{"inside herdr", ui.ColorAuto, true, "linux", map[string]string{"HERDR_ENV": "1"}, ui.Capability{Color: true, TrueColor: true}},
		{"not a terminal", ui.ColorAuto, false, "windows", nil, ui.Capability{}},
		{"NO_COLOR", ui.ColorAuto, true, "windows", map[string]string{"NO_COLOR": "1"}, ui.Capability{}},
		{"empty NO_COLOR is ignored", ui.ColorAuto, true, "windows", map[string]string{"NO_COLOR": ""}, ui.Capability{Color: true, TrueColor: true}},
		{"dumb terminal", ui.ColorAuto, true, "linux", map[string]string{"TERM": "dumb"}, ui.Capability{}},
		{"always beats pipe and NO_COLOR", ui.ColorAlways, false, "windows", map[string]string{"NO_COLOR": "1"}, ui.Capability{Color: true, TrueColor: true}},
		{"never beats terminal", ui.ColorNever, true, "windows", nil, ui.Capability{}},
		{"ascii", ui.ColorAuto, false, "linux", map[string]string{"HUM_ASCII": "1"}, ui.Capability{ASCII: true}},
		{"ascii zero is off", ui.ColorAuto, false, "linux", map[string]string{"HUM_ASCII": "0"}, ui.Capability{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ui.Detect(tt.mode, tt.terminal, tt.goos, envOf(tt.env)); got != tt.want {
				t.Errorf("Detect() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func mocha() ui.Theme { return ui.LoadTheme(strings.NewReader("")) }

func TestPaintTrueColor(t *testing.T) {
	t.Parallel()

	s := ui.NewStyle(ui.Capability{Color: true, TrueColor: true}, mocha())
	// idle es verde (#a6e3a1) en catppuccin.
	if got, want := s.Paint(ui.RoleIdle, "idle"), "\x1b[38;2;166;227;161midle\x1b[0m"; got != want {
		t.Errorf("Paint(idle) = %q, want %q", got, want)
	}
	if got := s.Paint(ui.RoleBlocked, "x"); !strings.HasPrefix(got, "\x1b[38;2;243;139;168m") {
		t.Errorf("Paint(blocked) = %q, want catppuccin red", got)
	}
}

func TestPaintANSIFallback(t *testing.T) {
	t.Parallel()

	// Sin truecolor, o con un tema sin paleta embebida, se usa la paleta ANSI.
	noPalette := ui.LoadTheme(strings.NewReader("[theme]\nname = \"terminal\"\n"))
	for name, s := range map[string]*ui.Style{
		"no truecolor":   ui.NewStyle(ui.Capability{Color: true}, mocha()),
		"terminal theme": ui.NewStyle(ui.Capability{Color: true, TrueColor: true}, noPalette),
		"terminal no tc": ui.NewStyle(ui.Capability{Color: true}, noPalette),
	} {
		if got, want := s.Paint(ui.RoleBlocked, "b"), "\x1b[31mb\x1b[0m"; got != want {
			t.Errorf("%s: Paint(blocked) = %q, want %q", name, got, want)
		}
		if got, want := s.Paint(ui.RoleDim, "d"), "\x1b[2md\x1b[0m"; got != want {
			t.Errorf("%s: Paint(dim) = %q, want %q", name, got, want)
		}
		if got := s.Paint(ui.RoleText, "t"); got != "t" {
			t.Errorf("%s: Paint(text) = %q, want plain text", name, got)
		}
	}
}

func TestStyleWithoutColorIsPlain(t *testing.T) {
	t.Parallel()

	s := ui.NewStyle(ui.Capability{}, mocha())
	for name, got := range map[string]string{
		"paint":    s.Paint(ui.RoleBlocked, "x"),
		"bold":     s.Bold("x"),
		"selected": s.Selected("x"),
	} {
		if got != "x" {
			t.Errorf("%s = %q, want plain text", name, got)
		}
	}
}

func TestBold(t *testing.T) {
	t.Parallel()

	s := ui.NewStyle(ui.Capability{Color: true}, mocha())
	if got, want := s.Bold("ws"), "\x1b[1mws\x1b[0m"; got != want {
		t.Errorf("Bold() = %q, want %q", got, want)
	}
}

func TestSelectedKeepsTheBackgroundAfterInnerResets(t *testing.T) {
	t.Parallel()

	s := ui.NewStyle(ui.Capability{Color: true, TrueColor: true}, mocha())
	inner := s.Paint(ui.RoleIdle, "idle") + " tail"
	got := s.Selected(inner)

	bg := "\x1b[48;2;49;50;68m" // selection_bg de catppuccin: #313244
	if !strings.HasPrefix(got, bg) || !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("Selected() = %q, want it wrapped in the selection background", got)
	}
	// El reinicio del texto pintado no debe apagar la barra de selección.
	if !strings.Contains(got, "idle\x1b[0m"+bg+" tail") {
		t.Errorf("Selected() = %q, want the background restored after the inner reset", got)
	}

	ansi := ui.NewStyle(ui.Capability{Color: true}, mocha()).Selected("row")
	if ansi != "\x1b[7mrow\x1b[0m" {
		t.Errorf("ANSI Selected() = %q, want reverse video", ansi)
	}
}

func TestGlyphs(t *testing.T) {
	t.Parallel()

	uni := ui.NewStyle(ui.Capability{}, mocha()).Glyphs()
	if uni.Agent != "○" || uni.Plain != "·" || uni.Current != "◆" ||
		uni.Branch != "├─ " || uni.LastBranch != "└─ " || uni.Ellipsis != "…" || uni.Fail != "✖" {
		t.Errorf("unicode glyphs = %+v", uni)
	}
	ascii := ui.NewStyle(ui.Capability{ASCII: true}, mocha()).Glyphs()
	for name, g := range map[string]string{
		"agent": ascii.Agent, "plain": ascii.Plain, "current": ascii.Current,
		"branch": ascii.Branch, "last": ascii.LastBranch, "ellipsis": ascii.Ellipsis, "fail": ascii.Fail,
	} {
		for _, r := range g {
			if r > 127 {
				t.Errorf("ascii glyph %s = %q has a non-ASCII rune", name, g)
			}
		}
	}
	if ascii.Branch == ascii.LastBranch {
		t.Error("ascii branch glyphs must differ")
	}
}
