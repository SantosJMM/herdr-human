package ui

import (
	"fmt"
	"strings"
)

// Mode es la opción --color.
type Mode int

// Valores de --color.
const (
	ColorAuto Mode = iota
	ColorAlways
	ColorNever
)

// ParseMode interpreta el valor de --color.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "auto":
		return ColorAuto, nil
	case "always":
		return ColorAlways, nil
	case "never":
		return ColorNever, nil
	}
	return ColorAuto, fmt.Errorf("invalid --color value %q (want auto, always or never)", s)
}

// Capability resume lo que puede mostrar la salida.
type Capability struct {
	// Color indica si se pueden emitir secuencias ANSI de color.
	Color bool
	// TrueColor indica si se pueden usar colores de 24 bits; si no, se usa la
	// paleta ANSI de la terminal.
	TrueColor bool
	// ASCII fuerza los glifos de respaldo (HUM_ASCII=1).
	ASCII bool
}

// Detect decide qué se puede mostrar. terminal indica si la salida es una
// terminal; goos y getenv se inyectan para poder probarlo.
//
// Color: --color never lo apaga y always lo fuerza (incluso con NO_COLOR o una
// tubería); en auto exige una terminal, que NO_COLOR no esté definido y que
// TERM no sea "dumb". Los 24 bits se asumen en Windows, dentro de herdr
// (HERDR_ENV=1) o si COLORTERM lo declara.
func Detect(mode Mode, terminal bool, goos string, getenv func(string) string) Capability {
	c := Capability{ASCII: truthy(getenv("HUM_ASCII"))}
	switch mode {
	case ColorNever:
		return c
	case ColorAlways:
		c.Color = true
	default:
		c.Color = terminal && getenv("NO_COLOR") == "" && getenv("TERM") != "dumb"
	}
	if c.Color {
		ct := getenv("COLORTERM")
		c.TrueColor = ct == "truecolor" || ct == "24bit" || goos == "windows" || getenv("HERDR_ENV") == "1"
	}
	return c
}

func truthy(s string) bool { return s != "" && s != "0" }

// Role es el papel semántico de un texto; el estilo decide su color.
type Role int

// Roles. Los colores de estado los decidió el autor en B0 (sin capturas de la
// barra de herdr): blocked rojo, working amarillo, done azul, idle verde. Se
// cambian en un solo sitio: Palette.color y ansiCodes.
const (
	RoleText Role = iota
	RoleDim
	RoleAccent
	RoleBlocked
	RoleWorking
	RoleDone
	RoleIdle
)

const reset = "\x1b[0m"

// ansiCodes es la paleta de respaldo: usa los colores de la terminal anfitriona.
var ansiCodes = map[Role]string{
	RoleDim:     "2",
	RoleAccent:  "36",
	RoleBlocked: "31",
	RoleWorking: "33",
	RoleDone:    "34",
	RoleIdle:    "32",
}

// Style pinta texto según la capacidad de la salida y el tema.
type Style struct {
	cap   Capability
	theme Theme
}

// NewStyle crea un estilo.
func NewStyle(c Capability, t Theme) *Style { return &Style{cap: c, theme: t} }

// truePalette devuelve la paleta de 24 bits si se puede usar.
func (s *Style) truePalette() *Palette {
	if s.cap.Color && s.cap.TrueColor {
		return s.theme.Palette
	}
	return nil
}

func (p *Palette) color(r Role) (Color, bool) {
	switch r {
	case RoleText:
		return p.Text, true
	case RoleDim:
		return p.Dim, true
	case RoleAccent:
		return p.Accent, true
	case RoleBlocked:
		return p.Red, true
	case RoleWorking:
		return p.Yellow, true
	case RoleDone:
		return p.Blue, true
	case RoleIdle:
		return p.Green, true
	}
	return Color{}, false
}

// Paint pinta text con el color del rol; sin color devuelve text tal cual.
// RoleText sin paleta embebida no se pinta: usa el color por defecto de la terminal.
func (s *Style) Paint(r Role, text string) string {
	if !s.cap.Color {
		return text
	}
	if p := s.truePalette(); p != nil {
		if c, ok := p.color(r); ok {
			return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s%s", c.R, c.G, c.B, text, reset)
		}
	}
	if code, ok := ansiCodes[r]; ok {
		return "\x1b[" + code + "m" + text + reset
	}
	return text
}

// Bold pone text en negrita; sin color devuelve text tal cual.
func (s *Style) Bold(text string) string {
	if !s.cap.Color {
		return text
	}
	return "\x1b[1m" + text + reset
}

// Selected dibuja line como la fila seleccionada. El fondo se restablece tras
// cada reinicio interno para que el texto ya pintado no corte la barra.
// Sin color devuelve line tal cual (quien llama debe marcar la fila de otro modo).
func (s *Style) Selected(line string) string {
	if !s.cap.Color {
		return line
	}
	start := "\x1b[7m"
	if p := s.truePalette(); p != nil {
		start = fmt.Sprintf("\x1b[48;2;%d;%d;%dm", p.Selection.R, p.Selection.G, p.Selection.B)
	}
	return start + strings.ReplaceAll(line, reset, reset+start) + reset
}

// Glyphs son los símbolos del árbol y de las filas.
type Glyphs struct {
	Agent      string // ○ agente
	Plain      string // · terminal plana
	Current    string // ◆ pane actual
	Branch     string // ├─
	LastBranch string // └─
	Ellipsis   string // …
	Fail       string // ✖ máquina inalcanzable
	Check      string // ✔ paso completado
}

// Glyphs devuelve los símbolos, con respaldo ASCII si se pidió HUM_ASCII=1.
func (s *Style) Glyphs() Glyphs {
	if s.cap.ASCII {
		return Glyphs{Agent: "o", Plain: ".", Current: "*", Branch: "|- ", LastBranch: "`- ", Ellipsis: "...", Fail: "x", Check: "ok"}
	}
	return Glyphs{Agent: "○", Plain: "·", Current: "◆", Branch: "├─ ", LastBranch: "└─ ", Ellipsis: "…", Fail: "✖", Check: "✔"}
}
