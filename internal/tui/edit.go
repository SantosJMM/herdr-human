package tui

import (
	"slices"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/ui"
)

// edit aplica una tecla de edición a una línea de texto. pos es la posición del
// cursor en letras (no en bytes). Devuelve el texto, el cursor y si el texto cambió;
// una tecla que no edita deja todo igual.
func edit(text string, pos int, msg tea.KeyMsg) (string, int, bool) {
	r := []rune(text)
	pos = max(0, min(pos, len(r)))
	switch msg.Type {
	case tea.KeyLeft:
		pos = max(0, pos-1)
	case tea.KeyRight:
		pos = min(len(r), pos+1)
	case tea.KeyHome:
		pos = 0
	case tea.KeyEnd:
		pos = len(r)
	case tea.KeyBackspace:
		if pos > 0 {
			r = slices.Delete(r, pos-1, pos)
			pos--
		}
	case tea.KeyDelete:
		if pos < len(r) {
			r = slices.Delete(r, pos, pos+1)
		}
	case tea.KeyCtrlU:
		r, pos = nil, 0
	case tea.KeyRunes, tea.KeySpace:
		r = slices.Insert(r, pos, msg.Runes...)
		pos += len(msg.Runes)
	default:
		return text, pos, false
	}
	out := string(r)
	return out, pos, out != text
}

// withCursor dibuja el texto con el cursor intercalado en su posición.
func withCursor(text string, pos int, cursor string) string {
	r := []rune(text)
	pos = max(0, min(pos, len(r)))
	return string(r[:pos]) + cursor + string(r[pos:])
}

// mark resalta en text las letras que coinciden con la búsqueda: cada palabra
// como trozo seguido (sin distinguir mayúsculas) o, si no está entera, letra a
// letra en orden, igual que el filtro. Lo que no coincide se pinta con base.
func (m Model) mark(text string, base func(string) string) string {
	words := strings.Fields(strings.ToLower(m.query))
	if len(words) == 0 || text == "" {
		return base(text)
	}
	runes := []rune(text)
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}

	hit := make([]bool, len(runes))
	for _, w := range words {
		markWord(hit, lower, []rune(w))
	}

	var sb strings.Builder
	for i := 0; i < len(runes); {
		j := i
		for j < len(runes) && hit[j] == hit[i] {
			j++
		}
		if run := string(runes[i:j]); hit[i] {
			sb.WriteString(m.st.Bold(m.st.Paint(ui.RoleAccent, run)))
		} else {
			sb.WriteString(base(run))
		}
		i = j
	}
	return sb.String()
}

// markWord marca en hit cada aparición seguida de w en text; si no hay ninguna,
// las letras de w en orden (la primera posibilidad).
func markWord(hit []bool, text, w []rune) {
	if len(w) == 0 {
		return
	}
	found := false
	for i := 0; i+len(w) <= len(text); i++ {
		if slices.Equal(text[i:i+len(w)], w) {
			for k := i; k < i+len(w); k++ {
				hit[k] = true
			}
			found = true
			i += len(w) - 1
		}
	}
	if found {
		return
	}
	var picked []int
	next := 0
	for _, want := range w {
		for next < len(text) && text[next] != want {
			next++
		}
		if next == len(text) {
			return // no es una subsecuencia: nada que resaltar
		}
		picked = append(picked, next)
		next++
	}
	for _, p := range picked {
		hit[p] = true
	}
}
