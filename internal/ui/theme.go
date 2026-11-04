package ui

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Palette son los colores que usa hum. Los nombres siguen los tokens de
// [theme.custom] de herdr cuando existen (accent, selection_bg, text, red…).
type Palette struct {
	Text      Color
	Dim       Color // overlay1 en herdr
	Accent    Color
	Selection Color // selection_bg
	Red       Color
	Green     Color
	Yellow    Color
	Blue      Color
}

// Theme es el tema resuelto. Palette es nil cuando no hay una paleta embebida
// (tema "terminal" o desconocido): entonces se usa la paleta ANSI de la terminal.
type Theme struct {
	Name    string
	Palette *Palette
}

// builtin son las paletas embebidas. herdr no expone sus colores resueltos por
// CLI ni por API, así que esto es una aproximación mantenida a mano con las
// paletas públicas de Catppuccin; si herdr cambia las suyas hay que actualizarla.
var builtin = map[string]Palette{
	"catppuccin": { // Mocha
		Text: Color{0xcd, 0xd6, 0xf4}, Dim: Color{0x7f, 0x84, 0x9c}, Accent: Color{0xf5, 0xc2, 0xe7},
		Selection: Color{0x31, 0x32, 0x44}, Red: Color{0xf3, 0x8b, 0xa8}, Green: Color{0xa6, 0xe3, 0xa1},
		Yellow: Color{0xf9, 0xe2, 0xaf}, Blue: Color{0x89, 0xb4, 0xfa},
	},
	"catppuccin-latte": {
		Text: Color{0x4c, 0x4f, 0x69}, Dim: Color{0x8c, 0x8f, 0xa1}, Accent: Color{0xea, 0x76, 0xcb},
		Selection: Color{0xcc, 0xd0, 0xda}, Red: Color{0xd2, 0x0f, 0x39}, Green: Color{0x40, 0xa0, 0x2b},
		Yellow: Color{0xdf, 0x8e, 0x1d}, Blue: Color{0x1e, 0x66, 0xf5},
	},
}

// defaultTheme es el tema de herdr cuando la configuración no fija ninguno.
const defaultTheme = "catppuccin"

// LoadTheme lee [theme] y [theme.custom] de un config.toml de herdr. Nunca
// falla: lo ausente o inválido deja el valor por defecto, como hace herdr.
// Con auto_switch se usa name, porque hum no puede saber si la terminal
// anfitriona está en modo claro u oscuro; las subtablas light/dark se ignoran.
func LoadTheme(r io.Reader) Theme {
	name := defaultTheme
	custom := map[string]string{}

	section := ""
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(stripComment(sc.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), unquote(strings.TrimSpace(value))
		switch section {
		case "theme":
			if key == "name" && value != "" {
				name = value
			}
		case "theme.custom":
			custom[key] = value
		}
	}

	th := Theme{Name: name}
	if base, ok := builtin[name]; ok {
		p := base
		applyCustom(&p, custom)
		th.Palette = &p
	}
	return th
}

// applyCustom superpone los tokens del usuario; un color inválido o un alias
// de reset conserva el valor del tema.
func applyCustom(p *Palette, custom map[string]string) {
	targets := map[string]*Color{
		"text": &p.Text, "overlay1": &p.Dim, "accent": &p.Accent, "selection_bg": &p.Selection,
		"red": &p.Red, "green": &p.Green, "yellow": &p.Yellow, "blue": &p.Blue,
	}
	for token, dst := range targets {
		if c, ok := ParseColor(custom[token]); ok {
			*dst = c
		}
	}
}

// stripComment quita un comentario # que no esté dentro de comillas.
func stripComment(s string) string {
	inQuote := false
	for i, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == '#' && !inQuote:
			return s[:i]
		}
	}
	return s
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// ConfigPath devuelve la ruta del config.toml de herdr, o "" si no se puede
// determinar. goos y getenv se inyectan para poder probarla.
func ConfigPath(goos string, getenv func(string) string) string {
	if goos == "windows" {
		if appdata := getenv("APPDATA"); appdata != "" {
			return appdata + `\herdr\config.toml`
		}
		return ""
	}
	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.ToSlash(filepath.Join(xdg, "herdr", "config.toml"))
	}
	if home := getenv("HOME"); home != "" {
		return filepath.ToSlash(filepath.Join(home, ".config", "herdr", "config.toml"))
	}
	return ""
}

// LoadThemeFile lee el tema del config.toml de herdr. Si el archivo no existe o
// no se puede leer devuelve el tema por defecto, igual que herdr.
func LoadThemeFile(goos string, getenv func(string) string) Theme {
	if path := ConfigPath(goos, getenv); path != "" {
		if f, err := os.Open(path); err == nil {
			defer f.Close()
			return LoadTheme(f)
		}
	}
	return LoadTheme(strings.NewReader(""))
}
