package palette

import (
	"sort"
	"strings"
	"unicode"
)

// Puntuación de la búsqueda difusa: sube con las letras seguidas y con las que
// empiezan palabra; baja con los saltos entre coincidencias.
const (
	baseMatchScore        = 1
	consecutiveMatchBonus = 5
	wordStartBonus        = 10
	gapPenaltyPerChar     = 1
)

func isSeparator(r rune) bool {
	switch r {
	case '/', '-', '_', ' ', '.', ':':
		return true
	}
	return false
}

// Score puntúa cuánto se parece text a query como subsecuencia, sin distinguir
// mayúsculas. Cada letra de la consulta se busca en la primera aparición tras la
// anterior, así que el resultado es determinista. ok es false si no hay coincidencia.
func Score(text, query string) (score int, ok bool) {
	t := lowerRunes(text)
	at, ok := match(t, query)
	if !ok {
		return 0, false
	}
	prev := -1
	for _, i := range at {
		score += baseMatchScore
		if prev != -1 && i == prev+1 {
			score += consecutiveMatchBonus
		}
		if i == 0 || isSeparator(t[i-1]) {
			score += wordStartBonus
		}
		// El hueco inicial también cuenta: un prefijo exacto gana a la misma
		// coincidencia más adentro del texto.
		score -= (i - prev - 1) * gapPenaltyPerChar
		prev = i
	}
	return score, true
}

// Positions devuelve en qué letras de text (posiciones en runas) cae la
// coincidencia de query, las mismas que puntúa Score, para poder resaltarlas. Es
// nil si text no coincide; con la consulta vacía no hay nada que resaltar.
func Positions(text, query string) []int {
	at, ok := match(lowerRunes(text), query)
	if !ok {
		return nil
	}
	return at
}

// lowerRunes pasa a minúsculas letra a letra, de modo que las posiciones sigan
// siendo las del texto original (strings.ToLower puede cambiar el largo).
func lowerRunes(s string) []rune {
	rs := []rune(s)
	for i, r := range rs {
		rs[i] = unicode.ToLower(r)
	}
	return rs
}

// match busca cada letra de la consulta (sin espacios) en la primera aparición
// tras la anterior y devuelve sus posiciones en t, que ya va en minúsculas.
func match(t []rune, query string) (at []int, ok bool) {
	prev := -1
	for _, q := range lowerRunes(query) {
		if unicode.IsSpace(q) {
			continue
		}
		i := indexRune(t, q, prev+1)
		if i < 0 {
			return nil, false
		}
		at = append(at, i)
		prev = i
	}
	return at, true
}

func indexRune(rs []rune, r rune, from int) int {
	for i := from; i < len(rs); i++ {
		if rs[i] == r {
			return i
		}
	}
	return -1
}

// Filter deja los elementos que coinciden con query, del mejor al peor. Con la
// consulta vacía devuelve todos en su orden. Los empates conservan el orden de
// entrada, que ya trae recientes y alfabético, así que filtrar no reordena lo
// igual de bueno.
func Filter[T any](items []T, query string, key func(T) string) []T {
	if strings.TrimSpace(query) == "" {
		return items
	}
	type scored struct {
		item  T
		score int
		index int
	}
	var hits []scored
	for i, it := range items {
		if s, ok := Score(key(it), query); ok {
			hits = append(hits, scored{it, s, i})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].score != hits[b].score {
			return hits[a].score > hits[b].score
		}
		return hits[a].index < hits[b].index
	})
	out := make([]T, len(hits))
	for i, h := range hits {
		out[i] = h.item
	}
	return out
}
