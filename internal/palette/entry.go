// Paquete palette reúne la lógica de la paleta de comandos de `hum palette`:
// el catálogo de comandos nativos, las acciones de los plugins, la búsqueda
// difusa, los recientes y la resolución de argumentos. No dibuja nada.
//
// Es una adaptación a Go de https://github.com/barnuri/herdr-command-palette
// (MIT, © 2026 barnuri); ver THIRD_PARTY_NOTICES.md.
package palette

import (
	"strings"
	"unicode/utf8"
)

// Kind distingue de dónde viene una entrada de la paleta.
type Kind int

const (
	// KindPlugin es una acción declarada por un plugin de herdr.
	KindPlugin Kind = iota
	// KindNative es un comando del propio herdr descrito en commands.json.
	KindNative
	// KindHum es un comando de hum (no de herdr), con la misma descripción de
	// argumentos que los nativos; se ejecuta con `hum palette run --hum`.
	KindHum
)

// nativeSource es la etiqueta de origen de los comandos nativos.
const nativeSource = "Herdr"

// Fuentes de un argumento de un comando nativo.
const (
	SourceLiteral = "literal"
	SourceContext = "context"
	SourceInput   = "input"
	SourceSelect  = "select"
)

// Argument describe cómo se obtiene un argumento de un comando nativo.
type Argument struct {
	Source         string `json:"source"`
	Value          string `json:"value,omitempty"`
	Key            string `json:"key,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
	DefaultContext string `json:"default_context,omitempty"`
	Required       bool   `json:"required,omitempty"`
	Selector       string `json:"selector,omitempty"`
	// Flag, en un input, es la opción que lleva el valor: si la respuesta queda
	// vacía no se añade ni la opción ni el valor (un argumento opcional).
	Flag           string `json:"flag,omitempty"`
	ExcludeContext string `json:"exclude_context,omitempty"`
}

// Entry es una fila de la paleta: una acción de plugin o un comando nativo.
type Entry struct {
	Kind        Kind       `json:"kind"`
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Source      string     `json:"source"`
	PluginID    string     `json:"plugin_id,omitempty"`
	ActionID    string     `json:"action_id,omitempty"`
	Command     []string   `json:"command,omitempty"`
	Arguments   []Argument `json:"arguments,omitempty"`
	// Chord es el atajo de teclado enlazado en config.toml, si lo hay.
	Chord string `json:"-"`
}

// SearchText es el texto sobre el que busca el filtro difuso.
func (e Entry) SearchText() string {
	return strings.Join([]string{e.Source, e.Title, e.Description, e.ID}, " ")
}

// SearchOffsets dice dónde empiezan, en runas, el origen y el título dentro de
// SearchText. Sirven para llevar las coincidencias de la búsqueda de vuelta a
// las columnas que se dibujan.
func (e Entry) SearchOffsets() (source, title int) {
	return 0, utf8.RuneCountInString(e.Source) + 1
}

// RequiredContext lista las claves de contexto que el comando necesita.
func (e Entry) RequiredContext() []string {
	var keys []string
	for _, a := range e.Arguments {
		if a.Source == SourceContext {
			keys = append(keys, a.Key)
		}
	}
	return keys
}
