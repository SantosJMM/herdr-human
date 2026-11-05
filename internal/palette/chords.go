package palette

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	commandTable     = "[[keys.command]]"
	pluginActionType = "plugin_action"
)

// ConfigPath es el config.toml de herdr. HERDR_CONFIG_PATH manda; si no, la
// carpeta de configuración del sistema (en Windows, %APPDATA%). Si ahí no está,
// se prueba la ruta estilo XDG (~/.config), que es la de Linux y macOS.
func ConfigPath(getenv func(string) string) string {
	if p := getenv("HERDR_CONFIG_PATH"); p != "" {
		return p
	}
	var candidates []string
	if dir, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(dir, "herdr", "config.toml"))
	}
	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "herdr", "config.toml"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "herdr", "config.toml"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// LoadChords lee los atajos de config.toml; un archivo ausente o ilegible solo
// significa que no hay atajos que mostrar.
func LoadChords(getenv func(string) string) map[string]string {
	path := ConfigPath(getenv)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return ParseChords(string(data))
}

// ParseChords extrae, de cada [[keys.command]] de tipo plugin_action, la tecla
// asociada a la acción. Es un lector TOML deliberadamente estrecho: solo
// entiende esa tabla y las cadenas entre comillas de una línea; lo demás se
// ignora sin adivinar. Si una acción tiene varias teclas, gana la primera.
func ParseChords(toml string) map[string]string {
	chords := map[string]string{}
	var cur map[string]string
	var tables []map[string]string

	for _, raw := range strings.Split(toml, "\n") {
		line := strings.TrimSpace(stripComment(raw))
		switch {
		case line == "":
		case line == commandTable:
			cur = map[string]string{}
			tables = append(tables, cur)
		case strings.HasPrefix(line, "["):
			cur = nil
		case cur != nil:
			if name, value, ok := parseAssignment(line); ok {
				cur[name] = value
			}
		}
	}

	for _, t := range tables {
		if t["type"] != pluginActionType || t["command"] == "" || t["key"] == "" {
			continue
		}
		if _, dup := chords[t["command"]]; !dup {
			chords[t["command"]] = t["key"]
		}
	}
	return chords
}

// stripComment quita el comentario final, respetando los # dentro de comillas.
func stripComment(line string) string {
	inString := false
	for i, r := range line {
		switch {
		case r == '"':
			inString = !inString
		case r == '#' && !inString:
			return line[:i]
		}
	}
	return line
}

func parseAssignment(line string) (name, value string, ok bool) {
	name, raw, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	name, raw = strings.TrimSpace(name), strings.TrimSpace(raw)
	if name == "" || len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", "", false
	}
	return name, raw[1 : len(raw)-1], true
}
