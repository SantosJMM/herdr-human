package keybind

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Tipos de comando de un atajo que este paquete sabe crear.
const (
	TypePluginAction = "plugin_action"
	TypeShell        = "shell"
)

// Binding es lo que un atajo ejecuta: el tipo y el comando de un
// [[keys.command]]. Dos atajos son «el mismo» si tienen el mismo Binding.
type Binding struct {
	Type    string
	Command string
}

// Block es un [[keys.command]] del archivo con el lugar que ocupa.
type Block struct {
	// Start y End son las líneas del bloque: de la cabecera hasta la siguiente
	// tabla (End no incluido).
	Start, End int
	// KeyLine es la línea de `key = ...`, o -1 si el bloque no la tiene.
	KeyLine int
	// LastLine es la última línea con una asignación del bloque: lo que viene
	// después (comentarios, líneas en blanco) ya no es del bloque.
	LastLine                        int
	Key, Type, Command, Description string
}

// Config es un config.toml leído para consultar y editar sus atajos. Guarda el
// texto línea a línea tal cual, con sus comentarios y sus finales de línea, para
// que una edición no cambie nada fuera de lo que toca.
type Config struct {
	lines   []string
	eol     string
	blocks  []Block
	named   []namedKeys
	indexed []indexedKey
}

// namedKeys son los valores de una acción interna de la tabla [keys].
type namedKeys struct {
	name string
	keys []string
}

// indexedKey es una entrada de [keys.indexed]: un modificador que, con 1..9,
// elige un workspace, una pestaña o un agente.
type indexedKey struct{ name, modifier string }

// ErrNotBound se devuelve al quitar un atajo que no existe.
var ErrNotBound = errors.New("that action has no shortcut in config.toml")

var (
	quoted     = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
	keyLineRe  = regexp.MustCompile(`^(\s*key\s*=\s*)"(?:[^"\\]|\\.)*"`)
	assignRe   = regexp.MustCompile(`^\s*([A-Za-z0-9_.-]+)\s*=\s*(.*)$`)
	tableHead  = regexp.MustCompile(`^\s*\[\[?\s*([^\]]+?)\s*\]\]?\s*(#.*)?$`)
	commandTbl = "keys.command"
)

// Parse lee el texto de un config.toml. Es un lector deliberadamente estrecho:
// entiende las tablas [keys], [keys.indexed] y [[keys.command]] y los valores de
// una sola línea; el resto lo conserva sin interpretar.
func Parse(text string) *Config {
	c := &Config{eol: "\n", lines: strings.SplitAfter(text, "\n")}
	if strings.Contains(text, "\r\n") {
		c.eol = "\r\n"
	}
	if n := len(c.lines); n > 0 && c.lines[n-1] == "" {
		c.lines = c.lines[:n-1]
	}

	var cur *Block
	closeBlock := func(end int) {
		if cur != nil {
			cur.End = end
			c.blocks = append(c.blocks, *cur)
			cur = nil
		}
	}
	table := ""
	for i, raw := range c.lines {
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			closeBlock(i)
			m := tableHead.FindStringSubmatch(line)
			table = ""
			if m != nil {
				table = m[1]
			}
			if table == commandTbl && strings.HasPrefix(line, "[[") {
				cur = &Block{Start: i, KeyLine: -1, LastLine: i}
			}
			continue
		}
		m := assignRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name, value := m[1], strings.TrimSpace(m[2])
		switch {
		case cur != nil && table == commandTbl:
			cur.LastLine = i
			c.setField(cur, i, name, value)
		case table == "keys":
			c.named = append(c.named, namedKeys{name: name, keys: quotedStrings(value)})
		case table == "keys.indexed":
			if s := quotedStrings(value); len(s) == 1 {
				c.indexed = append(c.indexed, indexedKey{name: name, modifier: strings.ToLower(s[0])})
			}
		}
	}
	closeBlock(len(c.lines))
	return c
}

func (c *Config) setField(b *Block, line int, name, value string) {
	s := quotedStrings(value)
	if len(s) != 1 {
		return
	}
	switch name {
	case "key":
		b.Key, b.KeyLine = s[0], line
	case "type":
		b.Type = s[0]
	case "command":
		b.Command = s[0]
	case "description":
		b.Description = s[0]
	}
}

func quotedStrings(value string) []string {
	var out []string
	for _, m := range quoted.FindAllStringSubmatch(value, -1) {
		out = append(out, unescape(m[1]))
	}
	return out
}

// unescape deshace los escapes de una cadena TOML entre comillas que este
// paquete mismo escribe (\\ y \") y los más comunes; otros se dejan como están.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	return strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\n`, "\n", `\t`, "\t", `\r`, "\r").Replace(s)
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

// Text devuelve el archivo tal como está.
func (c *Config) Text() string { return strings.Join(c.lines, "") }

// Blocks devuelve los [[keys.command]] del archivo.
func (c *Config) Blocks() []Block { return append([]Block(nil), c.blocks...) }

// Find devuelve los bloques que enlazan b.
func (c *Config) Find(b Binding) []Block {
	var out []Block
	for _, blk := range c.blocks {
		if blk.Type == b.Type && blk.Command == b.Command {
			out = append(out, blk)
		}
	}
	return out
}

// KeyOf devuelve la tecla de b, o "" si no tiene ninguna.
func (c *Config) KeyOf(b Binding) string {
	if found := c.Find(b); len(found) > 0 {
		return found[0].Key
	}
	return ""
}

// Set pone key a b: cambia la tecla del bloque que ya existe o añade un bloque
// al final. key debe estar ya validada con Normalize. Devuelve el texto nuevo; el
// resto del archivo queda igual, byte a byte. Si b aparece en más de un bloque,
// solo se cambia el primero.
func (c *Config) Set(b Binding, key, description string) (string, error) {
	if key == "" || strings.ContainsAny(key, "\"\r\n\\") {
		return "", fmt.Errorf("%q is not a shortcut that can be written", key)
	}
	lines := append([]string(nil), c.lines...)
	if found := c.Find(b); len(found) > 0 {
		blk := found[0]
		if blk.KeyLine >= 0 {
			lines[blk.KeyLine] = keyLineRe.ReplaceAllString(lines[blk.KeyLine], `${1}"`+key+`"`)
			return strings.Join(lines, ""), nil
		}
		line := "key = \"" + key + "\"" + c.eol
		lines = append(lines[:blk.Start+1], append([]string{line}, lines[blk.Start+1:]...)...)
		return strings.Join(lines, ""), nil
	}
	return strings.Join(c.appendBlock(lines, b, key, description), ""), nil
}

// appendBlock añade un bloque nuevo al final, separado por una línea en blanco.
func (c *Config) appendBlock(lines []string, b Binding, key, description string) []string {
	if n := len(lines); n > 0 && !strings.HasSuffix(lines[n-1], "\n") {
		lines[n-1] += c.eol
	}
	if n := len(lines); n > 0 && strings.TrimSpace(lines[n-1]) != "" {
		lines = append(lines, c.eol)
	}
	block := []string{
		"[[keys.command]]" + c.eol,
		`key = "` + key + `"` + c.eol,
		`type = "` + b.Type + `"` + c.eol,
		`command = "` + escape(b.Command) + `"` + c.eol,
	}
	if description != "" {
		block = append(block, `description = "`+escape(description)+`"`+c.eol)
	}
	return append(lines, block...)
}

// escape escapa lo que no puede ir tal cual dentro de una cadena TOML entre comillas.
func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// Remove quita el atajo de b: borra su bloque (o todos, si está repetido) con una
// línea en blanco, para no dejar huecos dobles. Devuelve ErrNotBound si no hay.
func (c *Config) Remove(b Binding) (string, error) {
	found := c.Find(b)
	if len(found) == 0 {
		return "", ErrNotBound
	}
	lines := append([]string(nil), c.lines...)
	for i := len(found) - 1; i >= 0; i-- {
		// Solo se borran las líneas del bloque hasta su última asignación; los
		// comentarios o las líneas en blanco que le siguen no son suyos.
		start, end := found[i].Start, found[i].LastLine+1
		cut := end
		if cut < len(lines) && strings.TrimSpace(lines[cut]) == "" {
			cut++ // la línea en blanco que lo separaba del siguiente
		} else if cut >= len(lines) && start > 0 && strings.TrimSpace(lines[start-1]) == "" {
			start-- // era el último: que no quede una línea en blanco colgando
		}
		lines = append(lines[:start], lines[cut:]...)
	}
	return strings.Join(lines, ""), nil
}
