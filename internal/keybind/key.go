// Paquete keybind lee y edita los atajos de teclado del config.toml de herdr:
// qué acción tiene qué tecla, qué teclas chocan y cómo cambiar una sin tocar el
// resto del archivo. No dibuja nada ni habla con herdr.
//
// Las reglas de sintaxis de las teclas y los atajos por defecto salen de la
// documentación de herdr (configuration y config-reference, v0.9.5).
package keybind

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Prefix es el marcador de «primero el prefijo de herdr, luego la tecla».
const Prefix = "prefix"

// modifierOrder es el orden canónico de los modificadores.
var modifierOrder = []string{"ctrl", "alt", "shift", "cmd"}

// namedKey reconoce el nombre de una tecla especial o de función: enter, tab,
// left, f1, minus, space… La lista completa no está documentada, así que aquí
// solo se exige la forma; quien dice la última palabra es `herdr config check`.
var namedKey = regexp.MustCompile(`^[a-z][a-z0-9_]+$`)

// ErrEmptyKey se devuelve para una tecla vacía.
var ErrEmptyKey = errors.New("empty shortcut")

// Normalize valida la forma de una tecla y la devuelve en su forma canónica
// (minúsculas, «prefix» primero y modificadores en orden fijo), que es la que se
// escribe en el archivo y con la que se comparan los atajos.
func Normalize(key string) (string, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return "", ErrEmptyKey
	}
	parts := strings.Split(key, "+")

	var prefix bool
	var mods []string
	for i, p := range parts {
		last := i == len(parts)-1
		switch {
		case p == "":
			return "", fmt.Errorf("%q has an empty part (a literal + is written \"plus\")", key)
		case p == Prefix && !last:
			if i != 0 {
				return "", fmt.Errorf("%q: prefix must come first", key)
			}
			prefix = true
		case slices.Contains(modifierOrder, p) && !last:
			if slices.Contains(mods, p) {
				return "", fmt.Errorf("%q repeats %q", key, p)
			}
			mods = append(mods, p)
		case !last:
			return "", fmt.Errorf("%q: %q is not a modifier (ctrl, alt, shift, cmd or prefix)", key, p)
		}
	}

	base := parts[len(parts)-1]
	if utf8.RuneCountInString(base) != 1 && !namedKey.MatchString(base) {
		return "", fmt.Errorf("%q: %q is not a key", key, base)
	}
	if base == Prefix || slices.Contains(modifierOrder, base) {
		return "", fmt.Errorf("%q ends in a modifier, not a key", key)
	}

	slices.SortFunc(mods, func(a, b string) int {
		return slices.Index(modifierOrder, a) - slices.Index(modifierOrder, b)
	})
	out := append([]string{}, mods...)
	if prefix {
		out = append([]string{Prefix}, out...)
	}
	return strings.Join(append(out, base), "+"), nil
}

// IsSafeDirect dice si una tecla directa (sin prefijo) puede enlazarse sin
// estropear la escritura: una letra o un signo suelto sin modificador se come lo
// que se teclea en cualquier terminal. Las teclas especiales y de función sí valen.
func IsSafeDirect(key string) bool {
	n, err := Normalize(key)
	if err != nil {
		return false
	}
	parts := strings.Split(n, "+")
	if parts[0] == Prefix || len(parts) > 1 {
		return true
	}
	return utf8.RuneCountInString(parts[0]) > 1 // «f1», «home»… sí; «a», «?» no
}

// SameKey dice si dos teclas son la misma una vez normalizadas.
func SameKey(a, b string) bool {
	na, errA := Normalize(a)
	nb, errB := Normalize(b)
	return errA == nil && errB == nil && na == nb
}

// namedByRune pasa un carácter a su nombre cuando herdr lo escribe con nombre.
var namedByRune = map[rune]string{'-': "minus", ',': "comma", '&': "ampersand", '+': "plus", '`': "backtick", ' ': "space"}

// RuneName devuelve cómo se escribe una tecla de carácter en el config.toml.
func RuneName(r rune) string {
	if name, ok := namedByRune[r]; ok {
		return name
	}
	return strings.ToLower(string(r))
}
