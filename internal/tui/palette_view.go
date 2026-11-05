package tui

import (
	"fmt"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/ui"
)

const (
	// palChrome son las líneas que no son filas: borde, búsqueda, 2 separadores,
	// pie y borde.
	palChrome = 6
	// Anchos máximos de las columnas de origen y atajo.
	palMaxSource = 22
	palMaxChord  = 18
)

// View dibuja el marco de la paleta con el tamaño de la ventana.
func (m PaletteModel) View() string {
	w, _ := m.size()
	cw := w - 4
	b := boxFor(m.st)
	g := m.st.Glyphs()
	dim := func(s string) string { return m.st.Paint(ui.RoleDim, s) }
	fit := func(s string) string { return ui.PadRight(ui.Truncate(s, cw, g.Ellipsis), cw) }
	line := func(inner string) string { return dim(b.v) + " " + fit(inner) + " " + dim(b.v) }
	rule := func(l, r string) string { return dim(l + strings.Repeat(b.h, w-2) + r) }

	title := " Command palette "
	if m.mode == palInput || m.mode == palChoose {
		title = " " + m.prompt + " "
	}
	out := []string{dim(b.tl + title + strings.Repeat(b.h, max(0, w-2-ui.VisibleLen(title))) + b.tr)}

	left, right := m.searchLine(b)
	out = append(out, line(ui.FitRow(cw, left, right, g.Ellipsis)), rule(b.lt, b.rt))

	body := m.bodyLines(cw, b)
	for i := 0; i < m.pageSize(); i++ {
		if i < len(body) {
			out = append(out, body[i](dim(b.v), dim(b.v)))
			continue
		}
		out = append(out, line(""))
	}

	out = append(out, rule(b.lt, b.rt), line(dim(m.keysText())), rule(b.bl, b.br))
	return strings.Join(out, "\n")
}

// searchLine es la barra de búsqueda o de entrada de texto, con el contador.
func (m PaletteModel) searchLine(b box) (left, right string) {
	dim := func(s string) string { return m.st.Paint(ui.RoleDim, s) }
	cursor := dim(b.cursor)
	switch m.mode {
	case palInput:
		return "❯ " + withCursor(m.input.query, m.input.qpos, cursor), ""
	case palChoose:
		return "❯ " + withCursor(m.choices.query, m.choices.qpos, cursor),
			dim(fmt.Sprintf("%d/%d", len(m.choices.vis), len(m.choices.rows)))
	case palBusy, palError:
		return dim("❯ working…"), ""
	}
	left = "❯ " + withCursor(m.list.query, m.list.qpos, cursor)
	count := fmt.Sprintf("%d/%d", len(m.list.vis), len(m.list.rows))
	switch {
	case m.loading:
		count += " " + b.sep + " loading plugins…"
	case m.pluginsErr != nil:
		return left, dim(count) + " " + m.st.Paint(ui.RoleBlocked, b.sep+" plugins unavailable")
	}
	return left, dim(count)
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
			return l + m.rowContent(row, selected, cw, sw, kw, b) + r
		})
	}
	return lines
}

// rowContent dibuja una fila. La seleccionada ocupa todo el ancho: con color usa
// el fondo del tema y sin color lleva una marca al principio.
func (m PaletteModel) rowContent(row prow, selected bool, cw, sw, kw int, b box) string {
	g := m.st.Glyphs()
	right := ""
	if sw > 0 || kw > 0 {
		right = m.st.Paint(ui.RoleDim, ui.PadRight(ui.Truncate(row.source, sw, g.Ellipsis), sw)) + "  " +
			m.st.Paint(ui.RoleAccent, ui.PadRight(ui.Truncate(row.chord, kw, g.Ellipsis), kw))
	}
	content := ui.Truncate(row.title, cw, g.Ellipsis)
	if right != "" {
		content = ui.FitRow(cw, row.title, right, g.Ellipsis)
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
