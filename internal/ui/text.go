package ui

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// runeWidth devuelve cuántas celdas ocupa r: 0 para marcas combinantes, 2 para
// caracteres anchos (CJK, emoji) y 1 para el resto. Es una aproximación
// suficiente para títulos de agentes.
func runeWidth(r rune) int {
	switch {
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == 0x200d:
		return 0
	case r >= 0x1100 && r <= 0x115f,
		r >= 0x2e80 && r <= 0xa4cf,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe30 && r <= 0xfe6f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f64f,
		r >= 0x1f680 && r <= 0x1f6ff,
		r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x20000 && r <= 0x3fffd:
		return 2
	}
	return 1
}

// escapeLen devuelve la longitud de la secuencia CSI (ESC [ … final) que
// empieza en s[0], o 0 si s no empieza por una.
func escapeLen(s string) int {
	if len(s) < 2 || s[0] != 0x1b || s[1] != '[' {
		return 0
	}
	for i := 2; i < len(s); i++ {
		if s[i] >= 0x40 && s[i] <= 0x7e {
			return i + 1
		}
	}
	return len(s)
}

// VisibleLen devuelve las celdas que ocupa s al mostrarse, sin contar las
// secuencias ANSI.
func VisibleLen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if l := escapeLen(s[i:]); l > 0 {
			i += l
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		n += runeWidth(r)
		i += size
	}
	return n
}

// Truncate recorta s a width celdas añadiendo ellipsis si hubo que cortar. Las
// secuencias ANSI se conservan y, si se corta con un estilo abierto, se cierra.
func Truncate(s string, width int, ellipsis string) string {
	if width <= 0 {
		return ""
	}
	if VisibleLen(s) <= width {
		return s
	}
	ell := VisibleLen(ellipsis)
	if ell >= width {
		return Truncate(ellipsis, width, "")
	}
	limit := width - ell

	var b strings.Builder
	used, styled := 0, false
	for i := 0; i < len(s); {
		if l := escapeLen(s[i:]); l > 0 {
			seq := s[i : i+l]
			styled = seq != "\x1b[0m" && seq != "\x1b[m"
			b.WriteString(seq)
			i += l
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w := runeWidth(r)
		if used+w > limit {
			break
		}
		b.WriteString(s[i : i+size])
		used += w
		i += size
	}
	if styled {
		b.WriteString(reset)
	}
	b.WriteString(ellipsis)
	return b.String()
}

// Gap es el espacio mínimo entre la parte izquierda y la derecha de una fila.
const Gap = 2

// FitRow compone una fila de width celdas con left a la izquierda y right
// pegado a la derecha. Si no cabe, se recorta left y nunca right. Con width <= 0
// solo une las dos partes.
func FitRow(width int, left, right, ellipsis string) string {
	if width <= 0 {
		return left + strings.Repeat(" ", Gap) + right
	}
	room := width - Gap - VisibleLen(right)
	if room < 1 {
		// No cabe ni una celda a la izquierda: se recorta la derecha.
		return Truncate(right, width, ellipsis)
	}
	left = Truncate(left, room, ellipsis)
	pad := width - VisibleLen(left) - VisibleLen(right)
	return left + strings.Repeat(" ", pad) + right
}

// PadRight rellena s con espacios hasta width celdas; no recorta.
func PadRight(s string, width int) string {
	if pad := width - VisibleLen(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// PadLeft rellena s con espacios por la izquierda hasta width celdas, para
// alinear a la derecha; no recorta.
func PadLeft(s string, width int) string {
	if pad := width - VisibleLen(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

// defaultWidth es el ancho cuando no se puede averiguar.
const defaultWidth = 80

// WidthFrom decide el ancho de pantalla: el detectado en la terminal (si es
// > 0), o COLUMNS, o 80.
func WidthFrom(detected int, getenv func(string) string) int {
	if detected > 0 {
		return detected
	}
	if n, err := strconv.Atoi(getenv("COLUMNS")); err == nil && n > 0 {
		return n
	}
	return defaultWidth
}
