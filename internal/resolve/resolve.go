// Paquete resolve convierte lo que escribe una persona ("importer",
// "odoous/codex", "w1:p2") en un único agente del inventario.
package resolve

import (
	"errors"
	"fmt"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/inventory"
)

// localScope es el prefijo que fuerza el servidor local ("local/importer").
const localScope = "local"

// NotFoundError indica que ningún agente coincide con la consulta.
type NotFoundError struct {
	Query string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("no agent matches %q", e.Query)
}

// AmbiguousError indica que varios agentes coinciden en el mismo nivel.
type AmbiguousError struct {
	Query      string
	Candidates []inventory.Target
}

func (e *AmbiguousError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d agents:", e.Query, len(e.Candidates))
	for _, c := range e.Candidates {
		fmt.Fprintf(&b, "\n  %s  %s  %s", c.Ref(), c.Agent, c.Name)
	}
	return b.String()
}

// Resolve elige el agente que mejor responde a la consulta.
//
// Una consulta puede llevar el prefijo "maquina/" (o "local/") que restringe
// la búsqueda a un servidor. El resto se compara sin distinguir mayúsculas en
// niveles de precisión decreciente, y gana el primer nivel con coincidencias:
//
//  1. igualdad exacta con el ID del pane, el nombre o el tipo de agente;
//  2. el nombre empieza por la consulta;
//  3. el nombre contiene la consulta;
//  4. las letras de la consulta aparecen en orden dentro del nombre.
//
// Si ese nivel tiene más de una coincidencia devuelve *AmbiguousError.
func Resolve(targets []inventory.Target, query string) (inventory.Target, error) {
	query = strings.TrimSpace(query)
	scope, rest, scoped := splitScope(targets, query)
	if strings.TrimSpace(rest) == "" {
		return inventory.Target{}, errors.New("empty query")
	}

	pool := targets
	if scoped {
		pool = pool[:0:0]
		for _, t := range targets {
			if strings.EqualFold(t.Machine, scope) {
				pool = append(pool, t)
			}
		}
	}

	needle := strings.ToLower(strings.TrimSpace(rest))
	for _, match := range matchers {
		var hits []inventory.Target
		for _, t := range pool {
			if match(t, needle) {
				hits = append(hits, t)
			}
		}
		switch len(hits) {
		case 0:
			continue
		case 1:
			return hits[0], nil
		default:
			return inventory.Target{}, &AmbiguousError{Query: query, Candidates: hits}
		}
	}
	return inventory.Target{}, &NotFoundError{Query: query}
}

// splitScope separa un prefijo "maquina/" solo si es una máquina conocida,
// para no confundirlo con nombres que contienen una barra.
func splitScope(targets []inventory.Target, query string) (scope, rest string, scoped bool) {
	prefix, after, found := strings.Cut(query, "/")
	if !found {
		return "", query, false
	}
	if strings.EqualFold(prefix, localScope) {
		return "", after, true
	}
	for _, t := range targets {
		if t.Machine != "" && strings.EqualFold(t.Machine, prefix) {
			return t.Machine, after, true
		}
	}
	return "", query, false
}

type matcher func(t inventory.Target, needle string) bool

var matchers = []matcher{
	func(t inventory.Target, n string) bool {
		return strings.ToLower(t.PaneID) == n || strings.ToLower(t.Name) == n || strings.ToLower(t.Agent) == n
	},
	func(t inventory.Target, n string) bool {
		return strings.HasPrefix(strings.ToLower(t.Name), n)
	},
	func(t inventory.Target, n string) bool {
		return strings.Contains(strings.ToLower(t.Name), n)
	},
	func(t inventory.Target, n string) bool {
		return isSubsequence(n, strings.ToLower(t.Name))
	},
}

// isSubsequence indica si las letras de needle aparecen en orden en haystack.
func isSubsequence(needle, haystack string) bool {
	rest := []rune(needle)
	for _, r := range haystack {
		if len(rest) == 0 {
			break
		}
		if r == rest[0] {
			rest = rest[1:]
		}
	}
	return len(rest) == 0
}
