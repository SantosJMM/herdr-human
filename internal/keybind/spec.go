package keybind

import (
	"regexp"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/palette"
)

// safeToken es lo único que se escribe tal cual en un comando de shell: sin
// espacios, comillas ni caracteres que una shell interprete.
var safeToken = regexp.MustCompile(`^[A-Za-z0-9_./:=@-]+$`)

// shellEnv es la variable de entorno donde herdr deja, a los comandos de tipo
// shell, el id del lugar desde donde se pulsó el atajo.
var shellEnv = map[string]string{
	"pane_id":      "HERDR_ACTIVE_PANE_ID",
	"tab_id":       "HERDR_ACTIVE_TAB_ID",
	"workspace_id": "HERDR_ACTIVE_WORKSPACE_ID",
}

// Datos del plugin que abre la paleta, para el atajo «entrar directo en una
// entrada». Deben coincidir con el manifest (id del plugin y del pane).
const (
	// PluginID es el id del plugin hum en herdr.
	PluginID = "herdr-human"
	// PaletteEntrypoint es el id del pane de la paleta en el manifest.
	PaletteEntrypoint = "palette"
	// PaletteEntryEnv es la variable con la que el atajo le dice al popup en qué
	// entrada entrar directamente.
	PaletteEntryEnv = "HUM_PALETTE_ENTRY"
)

// How cuenta cómo se ejecuta un atajo de una entrada.
type How int

const (
	// HowPluginAction: herdr invoca la acción del plugin.
	HowPluginAction How = iota
	// HowDirect: herdr ejecuta el comando de shell tal cual, sin interfaz.
	HowDirect
	// HowOpenPalette: el atajo abre la paleta ya dentro de la entrada, que pregunta
	// lo que necesita y luego se ejecuta.
	HowOpenPalette
)

// SpecFor dice qué [[keys.command]] corresponde a una entrada de la paleta, o
// false si no se puede enlazar a una tecla.
//
//   - Una acción de plugin es type = "plugin_action" con su id calificado.
//   - Un comando de herdr sin preguntas (argumentos fijos o ids que herdr ya da al
//     comando, HERDR_ACTIVE_*) se enlaza como type = "shell" que lo ejecuta tal
//     cual, con la sintaxis de la shell del sistema: cmd.exe en Windows y sh en el
//     resto.
//   - Todo lo demás (comandos que eligen o piden algo, valores calculados y los
//     comandos de hum) también es type = "shell", pero el comando abre el popup de
//     la paleta ya dentro de esa entrada (PaletteEntryEnv), que hace las preguntas.
func SpecFor(e palette.Entry, goos string) (Binding, bool) {
	b, _, ok := SpecHow(e, goos)
	return b, ok
}

// SpecHow es SpecFor y además dice cómo se ejecuta el atajo.
func SpecHow(e palette.Entry, goos string) (Binding, How, bool) {
	switch e.Kind {
	case palette.KindPlugin:
		if e.ID == "" {
			return Binding{}, 0, false
		}
		return Binding{Type: TypePluginAction, Command: e.ID}, HowPluginAction, true
	case palette.KindNative, palette.KindHum:
		if e.Kind == palette.KindNative {
			if cmd, ok := shellCommand(e, goos); ok {
				return Binding{Type: TypeShell, Command: cmd}, HowDirect, true
			}
		}
		if cmd, ok := openPaletteCommand(e.ID, goos); ok {
			return Binding{Type: TypeShell, Command: cmd}, HowOpenPalette, true
		}
	}
	return Binding{}, 0, false
}

// openPaletteCommand es el comando de shell que abre el popup de la paleta con la
// entrada indicada. Usa la misma ruta de herdr (HERDR_BIN_PATH) que los demás.
func openPaletteCommand(id string, goos string) (string, bool) {
	if !safeToken.MatchString(id) {
		return "", false
	}
	bin := `"$HERDR_BIN_PATH"`
	if goos == "windows" {
		bin = `"%HERDR_BIN_PATH%"`
	}
	return bin + " plugin pane open --plugin " + PluginID + " --entrypoint " + PaletteEntrypoint +
		" --focus --env " + PaletteEntryEnv + "=" + id, true
}

func shellCommand(e palette.Entry, goos string) (string, bool) {
	windows := goos == "windows"
	bin := `"$HERDR_BIN_PATH"`
	variable := func(name string) string { return `"$` + name + `"` }
	if windows {
		bin = `"%HERDR_BIN_PATH%"`
		variable = func(name string) string { return "%" + name + "%" }
	}

	parts := []string{bin}
	for _, p := range e.Command {
		if !safeToken.MatchString(p) {
			return "", false
		}
		parts = append(parts, p)
	}
	for _, a := range e.Arguments {
		switch a.Source {
		case palette.SourceLiteral:
			if !safeToken.MatchString(a.Value) {
				return "", false
			}
			parts = append(parts, a.Value)
		case palette.SourceContext:
			name, ok := shellEnv[a.Key]
			if !ok {
				return "", false
			}
			parts = append(parts, variable(name))
		default:
			return "", false // pregunta algo: no se puede lanzar con una tecla
		}
	}
	return strings.Join(parts, " "), true
}

// Chords devuelve, por id de entrada, la tecla que el archivo le da, para las
// entradas que se pueden enlazar y la tienen.
func Chords(entries []palette.Entry, c *Config, goos string) map[string]string {
	out := map[string]string{}
	for _, e := range entries {
		if b, ok := SpecFor(e, goos); ok {
			if key := c.KeyOf(b); key != "" {
				out[e.ID] = key
			}
		}
	}
	return out
}
