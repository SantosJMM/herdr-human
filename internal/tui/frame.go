package tui

import (
	"strings"

	"github.com/SantosJMM/herdr-human/internal/ui"
)

// frame decide cómo se dibujan los bordes de una vista.
//
// Normalmente la vista lleva su propio marco con título (para usarla en una
// terminal cualquiera). Dentro de un popup de herdr ese marco ya lo dibuja herdr,
// con el título del pane, y un segundo marco queda como una caja dentro de otra:
// en modo incrustado (embedded) no se dibuja marco ni título, solo la línea que
// separa la búsqueda y un margen de una columna a cada lado, como las vistas
// nativas de herdr.
type frame struct {
	b        box
	dim      func(string) string
	w        int
	embedded bool
}

func newFrame(st *ui.Style, w int, embedded bool) frame {
	return frame{b: boxFor(st), dim: func(s string) string { return st.Paint(ui.RoleDim, s) }, w: w, embedded: embedded}
}

// sideChar es el carácter del borde lateral: el del marco o un espacio.
func (f frame) sideChar() string {
	if f.embedded {
		return " "
	}
	return f.b.v
}

// side es el borde lateral ya pintado.
func (f frame) side() string { return f.dim(f.sideChar()) }

// rule es una línea horizontal que separa partes de la vista.
func (f frame) rule() string {
	if f.embedded {
		return " " + f.dim(strings.Repeat(f.b.h, f.w-2)) + " "
	}
	return f.dim(f.b.lt + strings.Repeat(f.b.h, f.w-2) + f.b.rt)
}

// top son las líneas de encima del contenido: el borde con el título o, si herdr
// ya dibuja el suyo, ninguna.
func (f frame) top(title string) []string {
	if f.embedded {
		return nil
	}
	return []string{f.dim(f.b.tl + title + strings.Repeat(f.b.h, max(0, f.w-2-ui.VisibleLen(title))) + f.b.tr)}
}

// footerRule es la línea que separa el cuerpo del pie. Las vistas nativas de
// herdr no la llevan; el marco propio sí.
func (f frame) footerRule() []string {
	if f.embedded {
		return nil
	}
	return []string{f.rule()}
}

// bottom es el borde de abajo, o nada si herdr dibuja el suyo.
func (f frame) bottom() []string {
	if f.embedded {
		return nil
	}
	return []string{f.dim(f.b.bl + strings.Repeat(f.b.h, f.w-2) + f.b.br)}
}
