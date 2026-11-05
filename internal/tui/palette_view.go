package tui

import (
	"fmt"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/palette"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

const (
	// Anchos máximos de las columnas de origen y atajo.
	palMaxSource = 22
	palMaxChord  = 18
)

// View dibuja el marco de la paleta con el tamaño de la ventana.
func (m PaletteModel) View() string {
	defer m.tr.note("tui: first view rendered (the terminal gets it within one frame)")
	w, _ := m.size()
	cw := w - 4
	fr := newFrame(m.st, w, m.opts.Embedded)
	b := fr.b
	g := m.st.Glyphs()
	dim := fr.dim
	fit := func(s string) string { return ui.PadRight(ui.Truncate(s, cw, g.Ellipsis), cw) }
	line := func(inner string) string { return fr.side() + " " + fit(inner) + " " + fr.side() }

	// Con el marco propio, el título es el del popup; incrustado, lo dibuja herdr
	// (con el título del pane) y aquí no hay marco.
	title := " Command palette "
	if m.mode == palInput || m.mode == palChoose {
		title = " " + m.prompt + " "
	}
	out := fr.top(title)

	left, right := m.searchLine(b)
	out = append(out, line(ui.FitRow(cw, left, right, g.Ellipsis)), fr.rule())

	body := m.bodyLines(cw, b)
	for i := 0; i < m.pageSize(); i++ {
		if i < len(body) {
			out = append(out, body[i](fr.side(), fr.side()))
			continue
		}
		out = append(out, line(""))
	}

	out = append(out, fr.footerRule()...)
	out = append(out, line(dim(m.describeSelected())), line(dim(m.keysText())))
	out = append(out, fr.bottom()...)
	return strings.Join(out, "\n")
}

// describeSelected es la línea de ayuda de la entrada seleccionada: su
// descripción, o, si la acción de un plugin no trae ninguna, de qué plugin viene
// y su id (que es lo que se enlaza a una tecla en config.toml).
func (m PaletteModel) describeSelected() string {
	if m.mode != palList {
		return ""
	}
	i, ok := m.list.current()
	if !ok {
		return ""
	}
	e := m.entries[i]
	switch {
	case e.Description != "":
		return e.Description
	case e.Kind == palette.KindPlugin:
		return "Plugin action " + e.ID
	}
	return ""
}

// searchLine es la barra de búsqueda o de entrada de texto, con el contador.
func (m PaletteModel) searchLine(b box) (left, right string) {
	dim := func(s string) string { return m.st.Paint(ui.RoleDim, s) }
	cursor := dim(b.cursor)
	switch m.mode {
	case palInput:
		return "❯ " + withCursor(m.input.query, m.input.qpos, cursor), ""
	case palChoose:
		return "❯ " + withCursor(m.choices.query, m.choices.qpos, cursor) + placeholder(m.choices.query, "type to filter", dim),
			dim(fmt.Sprintf("%d/%d", len(m.choices.vis), len(m.choices.rows)))
	case palBusy, palError:
		return dim("❯ working…"), ""
	}
	left = "❯ " + withCursor(m.list.query, m.list.qpos, cursor) + placeholder(m.list.query, "type to search commands and plugin actions", dim)
	count := fmt.Sprintf("%d/%d", len(m.list.vis), len(m.list.rows))
	switch {
	case m.loading:
		count += " " + b.sep + " loading plugins…"
	case m.pluginsErr != nil:
		return left, dim(count) + " " + m.st.Paint(ui.RoleBlocked, b.sep+" plugins unavailable")
	}
	return left, dim(count)
}

// placeholder es el texto tenue de ayuda que acompaña a una búsqueda vacía, como
// el de las vistas nativas de herdr; con algo escrito no hay nada que mostrar.
func placeholder(query, text string, dim func(string) string) string {
	if query != "" {
		return ""
	}
	return " " + dim(text)
}

// rowRender dibuja una línea del cuerpo dados los bordes izquierdo y derecho.
type rowRender func(leftEdge, rightEdge string) string

func (m PaletteModel) bodyLines(cw int, b box) []rowRender {
	plain := func(s string) rowRender {
		return func(l, r string) string {
			return l + " " + ui.PadRight(ui.Truncate(s, cw, m.st.Glyphs().Ellipsis), cw) + " " + r
		}
	}
	switch m.mode {
	case palInput:
		if m.status != "" {
			return []rowRender{plain(m.st.Paint(ui.RoleBlocked, m.status))}
		}
		return nil
	case palBusy:
		return []rowRender{plain(m.st.Paint(ui.RoleDim, "running…"))}
	case palError:
		return []rowRender{plain(m.st.Paint(ui.RoleBlocked, m.status))}
	}

	p := &m.list
	if m.mode == palChoose {
		p = &m.choices
	}
	if len(p.vis) == 0 {
		text := "Nothing to choose here."
		if len(p.rows) > 0 {
			text = fmt.Sprintf("No entry matches %q.", p.query)
		}
		return []rowRender{plain(m.st.Paint(ui.RoleDim, text))}
	}

	end := min(len(p.vis), p.top+m.pageSize())
	window := p.vis[p.top:end]
	sw, kw := 0, 0
	for _, idx := range window {
		sw = max(sw, ui.VisibleLen(p.rows[idx].source))
		kw = max(kw, ui.VisibleLen(p.rows[idx].chord))
	}
	sw, kw = min(sw, palMaxSource), min(kw, palMaxChord)

	lines := make([]rowRender, 0, len(window))
	for i, idx := range window {
		row, selected := p.rows[idx], p.top+i == p.sel
		lines = append(lines, func(l, r string) string {
			return l + m.rowContent(row, p.query, selected, cw, sw, kw, b) + r
		})
	}
	return lines
}

// rowContent dibuja una fila. La seleccionada ocupa todo el ancho: con color usa
// el fondo del tema y sin color lleva una marca al principio.
func (m PaletteModel) rowContent(row prow, query string, selected bool, cw, sw, kw int, b box) string {
	g := m.st.Glyphs()
	// Las letras resaltadas son las mismas que puntúa el filtro, calculadas sobre
	// el texto de búsqueda y llevadas de vuelta a la columna donde se dibujan.
	hit := map[int]bool{}
	for _, at := range palette.Positions(row.search, query) {
		hit[at] = true
	}
	title := m.markHits(row.title, row.titleOff, hit, func(s string) string { return s })
	right := ""
	if sw > 0 || kw > 0 {
		source := m.markHits(row.source, row.sourceOff, hit, func(s string) string { return m.st.Paint(ui.RoleDim, s) })
		right = ui.PadRight(ui.Truncate(source, sw, g.Ellipsis), sw) + "  " +
			m.st.Paint(ui.RoleAccent, ui.PadRight(ui.Truncate(row.chord, kw, g.Ellipsis), kw))
	}
	content := ui.Truncate(title, cw, g.Ellipsis)
	if right != "" {
		content = ui.FitRow(cw, title, right, g.Ellipsis)
	}
	content = ui.PadRight(content, cw)
	if !selected {
		return " " + content + " "
	}
	if m.st.Color() {
		return m.st.Selected(" " + content + " ")
	}
	return b.marker + content + " "
}

func (m PaletteModel) keysText() string {
	up, sep := "↑↓", " · "
	if m.st.ASCII() {
		up, sep = "up/down", " - "
	}
	var keys []string
	switch m.mode {
	case palInput:
		keys = []string{"enter confirm", "ctrl+u clear", "esc cancel"}
	case palBusy:
		keys = []string{"esc close"}
	case palError:
		keys = []string{"any key: close"}
	default:
		keys = []string{up + " select", "enter run", "ctrl+u clear", "esc close"}
	}
	return strings.Join(keys, sep)
}

// markHits pinta text con base y resalta (negrita y color de acento) las letras
// cuya posición, sumada a off, está en hit. Con off < 0 la columna no forma parte
// del texto de búsqueda y no se resalta nada.
func (m PaletteModel) markHits(text string, off int, hit map[int]bool, base func(string) string) string {
	if off < 0 || len(hit) == 0 || text == "" {
		return base(text)
	}
	runes := []rune(text)
	var sb strings.Builder
	for i := 0; i < len(runes); {
		j := i
		for j < len(runes) && hit[off+j] == hit[off+i] {
			j++
		}
		if run := string(runes[i:j]); hit[off+i] {
			sb.WriteString(m.st.Bold(m.st.Paint(ui.RoleAccent, run)))
		} else {
			sb.WriteString(base(run))
		}
		i = j
	}
	return sb.String()
}
