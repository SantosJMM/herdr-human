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
	t := []rune(strings.ToLower(text))
	prev := -1
	for _, q := range strings.ToLower(query) {
		if unicode.IsSpace(q) {
			continue
		}
		at := indexRune(t, q, prev+1)
		if at < 0 {
			return 0, false
		}
		score += baseMatchScore
		if prev != -1 && at == prev+1 {
			score += consecutiveMatchBonus
		}
		if at == 0 || isSeparator(t[at-1]) {
			score += wordStartBonus
		}
		// El hueco inicial también cuenta: un prefijo exacto gana a la misma
		// coincidencia más adentro del texto.
		score -= (at - prev - 1) * gapPenaltyPerChar
		prev = at
	}
	return score, true
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
