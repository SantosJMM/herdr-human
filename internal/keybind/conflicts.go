package keybind

import (
	"fmt"
	"slices"
	"strings"
)

// Fuente de un choque.
const (
	// SourceCustom: otro [[keys.command]] del archivo. herdr deja uno y desactiva
	// el otro (lo avisa `herdr config check`).
	SourceCustom = "custom"
	// SourceConfigured: una acción interna que el archivo ya cambia en [keys] o
	// [keys.indexed]. Mismo trato que SourceCustom.
	SourceConfigured = "configured"
	// SourceDefault: un atajo por defecto de herdr que el archivo no cambia.
	// `herdr config check` NO lo avisa, así que solo se puede avisar desde aquí.
	SourceDefault = "default"
)

// Conflict es un atajo que ya usa otra cosa.
type Conflict struct {
	Source string
	// Name es la acción que lo usa: el comando de un [[keys.command]] o el nombre de
	// una acción interna («keys.next_tab»).
	Name string
	Key  string
}

// Blocking dice si herdr desactivaría el atajo nuevo (o el otro) por este choque.
// Los choques con un atajo por defecto no se bloquean: no se sabe cuál gana.
func (c Conflict) Blocking() bool { return c.Source != SourceDefault }

// Describe lo cuenta en una frase.
func (c Conflict) Describe() string {
	switch c.Source {
	case SourceCustom:
		return fmt.Sprintf("%s already uses %s in config.toml", c.Name, c.Key)
	case SourceConfigured:
		return fmt.Sprintf("%s already uses %s in config.toml", c.Name, c.Key)
	default:
		return fmt.Sprintf("%s is herdr's default for %s", c.Key, c.Name)
	}
}

// Conflicts devuelve lo que ya usa key, contando lo que dice el archivo y los
// atajos por defecto de herdr que el archivo no cambia. except es la acción a la
// que se le quiere dar la tecla: su propio bloque no es un choque.
func (c *Config) Conflicts(key string, except Binding) []Conflict {
	norm, err := Normalize(key)
	if err != nil {
		return nil
	}
	var out []Conflict

	for _, blk := range c.blocks {
		if (blk.Type == except.Type && blk.Command == except.Command) || !SameKey(blk.Key, norm) {
			continue
		}
		out = append(out, Conflict{Source: SourceCustom, Name: blk.Command, Key: norm})
	}
	for _, n := range c.named {
		if slices.ContainsFunc(n.keys, func(k string) bool { return SameKey(k, norm) }) {
			out = append(out, Conflict{Source: SourceConfigured, Name: "keys." + n.name, Key: norm})
		}
	}
	for _, ix := range c.indexed {
		if digit, ok := strings.CutPrefix(norm, ix.modifier+"+"); ok && len(digit) == 1 && digit[0] >= '1' && digit[0] <= '9' {
			out = append(out, Conflict{Source: SourceConfigured, Name: "keys.indexed." + ix.name, Key: norm})
		}
	}

	overridden := map[string]bool{}
	for _, n := range c.named {
		overridden[n.name] = true
	}
	for _, d := range defaultKeys {
		if overridden[d.name] {
			continue
		}
		if d.name == "switch_tab" {
			if digit, ok := strings.CutPrefix(norm, "prefix+"); ok && len(digit) == 1 && digit[0] >= '1' && digit[0] <= '9' {
				out = append(out, Conflict{Source: SourceDefault, Name: "keys.switch_tab", Key: norm})
			}
			continue
		}
		if SameKey(d.key, norm) {
			out = append(out, Conflict{Source: SourceDefault, Name: "keys." + d.name, Key: norm})
		}
	}
	return out
}
