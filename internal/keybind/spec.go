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

// SpecFor dice qué [[keys.command]] corresponde a una entrada de la paleta, o
// false si no se puede enlazar a una tecla.
//
//   - Una acción de plugin es type = "plugin_action" con su id calificado.
//   - Un comando de herdr se puede enlazar si no hay que preguntarle nada a la
//     persona: sus argumentos son fijos o ids que herdr ya da al comando
//     (HERDR_ACTIVE_*). Se enlaza como type = "shell", con la sintaxis de la shell
//     del sistema: cmd.exe en Windows y sh en el resto.
//   - Un comando de hum no: necesita preguntar.
func SpecFor(e palette.Entry, goos string) (Binding, bool) {
	switch e.Kind {
	case palette.KindPlugin:
		if e.ID == "" {
			return Binding{}, false
		}
		return Binding{Type: TypePluginAction, Command: e.ID}, true
	case palette.KindNative:
		cmd, ok := shellCommand(e, goos)
		return Binding{Type: TypeShell, Command: cmd}, ok
	}
	return Binding{}, false
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
