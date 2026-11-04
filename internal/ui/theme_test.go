package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/ui"
)

func TestLoadThemeDefaultsToCatppuccin(t *testing.T) {
	t.Parallel()

	for name, cfg := range map[string]string{
		"empty config":        "",
		"no theme section":    "[terminal]\ndefault_shell = \"nu\"\n",
		"theme without name":  "[theme]\nauto_switch = false\n",
		"commented-out name":  "[theme]\n# name = \"nord\"\n",
		"only unrelated keys": "onboarding = false\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			th := ui.LoadTheme(strings.NewReader(cfg))
			if th.Name != "catppuccin" || th.Palette == nil {
				t.Fatalf("LoadTheme() = %+v, want the embedded catppuccin palette", th)
			}
			if th.Palette.Text != (ui.Color{R: 0xcd, G: 0xd6, B: 0xf4}) {
				t.Errorf("text = %+v, want catppuccin mocha text #cdd6f4", th.Palette.Text)
			}
		})
	}
}

func TestLoadThemeLatte(t *testing.T) {
	t.Parallel()

	th := ui.LoadTheme(strings.NewReader("[theme]\nname = \"catppuccin-latte\"\n"))
	if th.Name != "catppuccin-latte" || th.Palette == nil {
		t.Fatalf("LoadTheme() = %+v, want latte", th)
	}
	if th.Palette.Text != (ui.Color{R: 0x4c, G: 0x4f, B: 0x69}) {
		t.Errorf("text = %+v, want latte text #4c4f69", th.Palette.Text)
	}
}

func TestLoadThemeKeepsNameWhenAutoSwitchIsOn(t *testing.T) {
	t.Parallel()

	// El tema claro/oscuro depende del terminal anfitrión, que hum no puede
	// consultar: se usa name.
	cfg := "[theme]\nname = \"catppuccin\"\nauto_switch = true\nlight_name = \"catppuccin-latte\"\ndark_name = \"catppuccin\"\n"
	if th := ui.LoadTheme(strings.NewReader(cfg)); th.Name != "catppuccin" {
		t.Errorf("Name = %q, want catppuccin", th.Name)
	}
}

func TestLoadThemeFallsBackToTheTerminalPalette(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"terminal", "tokyo-night", "does-not-exist"} {
		th := ui.LoadTheme(strings.NewReader("[theme]\nname = \"" + name + "\"\n"))
		if th.Name != name || th.Palette != nil {
			t.Errorf("LoadTheme(%q) = %+v, want no embedded palette", name, th)
		}
	}
}

func TestLoadThemeAppliesCustomTokens(t *testing.T) {
	t.Parallel()

	cfg := `
[theme]
name = "catppuccin"   # inline comment

[theme.custom]
accent = "#112233"
selection_bg = "rgb(1,2,3)"
panel_bg = "reset"        # ignorado: hum no usa este token
red = "#ff6188"
green = "not a color"     # inválido: se conserva el valor del tema
text = "#010203"
overlay1 = "#0a0b0c"

[theme.custom.light]
accent = "#ffffff"

[other]
accent = "#000000"
`
	p := ui.LoadTheme(strings.NewReader(cfg)).Palette
	if p == nil {
		t.Fatal("Palette = nil")
	}
	want := map[string][2]ui.Color{
		"accent":    {p.Accent, {R: 0x11, G: 0x22, B: 0x33}},
		"selection": {p.Selection, {R: 1, G: 2, B: 3}},
		"red":       {p.Red, {R: 0xff, G: 0x61, B: 0x88}},
		"green":     {p.Green, {R: 0xa6, G: 0xe3, B: 0xa1}},
		"text":      {p.Text, {R: 1, G: 2, B: 3}},
		"dim":       {p.Dim, {R: 0x0a, G: 0x0b, B: 0x0c}},
	}
	for name, pair := range want {
		if pair[0] != pair[1] {
			t.Errorf("%s = %+v, want %+v", name, pair[0], pair[1])
		}
	}
}

func TestConfigPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{"windows", "windows", map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`}, `C:\Users\u\AppData\Roaming\herdr\config.toml`},
		{"linux xdg", "linux", map[string]string{"XDG_CONFIG_HOME": "/x", "HOME": "/h"}, "/x/herdr/config.toml"},
		{"linux home", "linux", map[string]string{"HOME": "/h"}, "/h/.config/herdr/config.toml"},
		{"nothing known", "linux", nil, ""},
		{"windows without appdata", "windows", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ui.ConfigPath(tt.goos, envOf(tt.env)); got != tt.want {
				t.Errorf("ConfigPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadThemeFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "herdr", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[theme]\nname = \"catppuccin-latte\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	env := envOf(map[string]string{"XDG_CONFIG_HOME": dir})
	if th := ui.LoadThemeFile("linux", env); th.Name != "catppuccin-latte" {
		t.Errorf("LoadThemeFile() = %+v, want the file's theme", th)
	}
	// Sin archivo o sin ruta se usa el tema por defecto, sin error.
	for name, getenv := range map[string]func(string) string{
		"missing file": envOf(map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "nope")}),
		"unknown path": envOf(nil),
	} {
		if th := ui.LoadThemeFile("linux", getenv); th.Name != "catppuccin" || th.Palette == nil {
			t.Errorf("%s: LoadThemeFile() = %+v, want the default theme", name, th)
		}
	}
}
