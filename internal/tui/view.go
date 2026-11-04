package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// box son los caracteres del marco; con HUM_ASCII=1 se usan los de respaldo.
type box struct {
	tl, tr, bl, br, h, v, lt, rt, thumb, marker, sep, cursor string
}

func boxFor(st *ui.Style) box {
	if st.ASCII() {
		return box{"+", "+", "+", "+", "-", "|", "+", "+", "#", ">", "-", "_"}
	}
	return box{"┌", "┐", "└", "┘", "─", "│", "├", "┤", "┃", "▸", "·", "▌"}
}

// View dibuja el marco completo con el tamaño de la ventana.
func (m Model) View() string {
	w := max(m.width, minWidth)
	cw := w - 4
	bh := m.bodyHeight()
	b := boxFor(m.st)
	g := m.st.Glyphs()
	dim := func(s string) string { return m.st.Paint(ui.RoleDim, s) }

	fit := func(s string) string { return ui.PadRight(ui.Truncate(s, cw, g.Ellipsis), cw) }
	line := func(inner string) string { return dim(b.v) + " " + fit(inner) + " " + dim(b.v) }
	rule := func(l, r string) string { return dim(l + strings.Repeat(b.h, w-2) + r) }

	title := " Talk to "
	out := []string{dim(b.tl + title + strings.Repeat(b.h, max(0, w-2-ui.VisibleLen(title))) + b.tr)}

	left, right := m.searchLine(b)
	out = append(out, line(ui.FitRow(cw, left, right, g.Ellipsis)), rule(b.lt, b.rt))

	// Barra de desplazamiento: un tramo del borde derecho del cuerpo.
	thumbPos, thumbLen := -1, 0
	if n := len(m.rows); n > bh {
		thumbLen = max(1, bh*bh/n)
		thumbPos = m.top * (bh - thumbLen) / (n - bh)
	}
	sel := m.selected()
	if m.mode != modeList && m.mode != modeInput {
		for _, l := range m.overlayBody(cw, bh, b) {
			out = append(out, line(l))
		}
		bh = 0
	}
	for i := 0; i < bh; i++ {
		edge := b.v
		if thumbPos >= 0 && i >= thumbPos && i < thumbPos+thumbLen {
			edge = b.thumb
		}
		idx := m.top + i
		switch {
		case idx < len(m.rows):
			out = append(out, m.bodyLine(m.rows[idx], idx == sel, cw, b, dim(b.v), dim(edge)))
		case idx == len(m.rows) && len(m.agents) == 0:
			out = append(out, line(dim(m.emptyText())))
		default:
			out = append(out, line(""))
		}
	}

	out = append(out, rule(b.lt, b.rt))
	crumb, path := m.footerInfo()
	out = append(out, line(m.st.Bold(crumb)), line(m.infoLine(cw, path)), line(dim(m.keysText())), rule(b.bl, b.br))
	return strings.Join(out, "\n")
}

func (m Model) emptyText() string {
	if len(m.pending) > 0 {
		return "loading..."
	}
	if m.query != "" || m.filter != "" {
		return "no agents match"
	}
	return "no agents (run hum ls to check the machines)"
}

// searchLine devuelve la barra de búsqueda: lo escrito y el contador.
func (m Model) searchLine(b box) (left, right string) {
	dim := func(s string) string { return m.st.Paint(ui.RoleDim, s) }
	switch {
	case m.query != "" || m.searching:
		if m.searching {
			left = "/ " + withCursor(m.query, m.qpos, dim(b.cursor))
		} else {
			left = "/ " + m.query
		}
	default:
		left = dim("/ search agents")
	}
	if m.filter != "" {
		left += "  " + m.st.Paint(ui.StatusRole(m.filter), "["+m.filter+"]")
	}

	total, need := len(m.inv.Targets), 0
	for _, t := range m.inv.Targets {
		if t.Status == "blocked" || t.Status == "done" {
			need++
		}
	}
	count := fmt.Sprintf("%d agents", total)
	if m.query != "" || m.filter != "" {
		count = fmt.Sprintf("%d of %d agents", len(m.agents), total)
	}
	if need > 0 {
		count += " " + b.sep + fmt.Sprintf(" %d need you", need)
	}
	return left, dim(count)
}

// bodyLine dibuja una fila del cuerpo. La seleccionada ocupa todo el ancho: con
// color usa el fondo del tema y sin color lleva una marca al principio.
func (m Model) bodyLine(r row, selected bool, cw int, b box, leftEdge, rightEdge string) string {
	g := m.st.Glyphs()
	l, rt := m.rowText(r)
	content := ui.FitRow(cw, l, rt, g.Ellipsis)
	if rt == "" {
		content = ui.Truncate(l, cw, g.Ellipsis)
	}
	content = ui.PadRight(content, cw)
	if !selected {
		return leftEdge + " " + content + " " + rightEdge
	}
	if m.st.Color() {
		return leftEdge + m.st.Selected(" "+content+" ") + rightEdge
	}
	return leftEdge + b.marker + content + " " + rightEdge
}

// rowText devuelve el texto izquierdo y la columna derecha de una fila.
func (m Model) rowText(r row) (left, right string) {
	g := m.st.Glyphs()
	switch r.kind {
	case kindMachine:
		label := r.text
		if label == inventory.LocalLabel {
			label = "Local"
		}
		return m.st.Bold(label), m.machineStatus(r.machine)
	case kindWorkspace:
		return " " + m.mark(r.text, m.st.Bold), m.st.Paint(ui.RoleDim, r.branch)
	}
	t := m.inv.Targets[r.target]
	branch := g.Branch
	if r.last {
		branch = g.LastBranch
	}
	mark := m.st.Paint(ui.StatusRole(t.Status), g.Agent)
	if t.Focused {
		mark = m.st.Paint(ui.RoleAccent, g.Current) + " " + mark
	}
	plainText := func(s string) string { return s }
	dimText := func(s string) string { return m.st.Paint(ui.RoleDim, s) }
	left = " " + m.st.Paint(ui.RoleDim, branch) + mark + " " + m.mark(ui.ShortName(t.Name, m.full), plainText)
	right = m.mark(t.Agent, dimText) + strings.Repeat(" ", max(0, m.aw-ui.VisibleLen(t.Agent))) + strings.Repeat(" ", ui.Gap) +
		m.st.Paint(ui.StatusRole(t.Status), ui.PadLeft(t.Status, m.sw))
	return left, right
}

// machineStatus es lo que va a la derecha del encabezado de una máquina.
func (m Model) machineStatus(label string) string {
	want := label
	if label == inventory.LocalLabel {
		want = ""
	}
	if m.pending[want] {
		if _, had := m.perMachine[want]; had {
			return m.st.Paint(ui.RoleDim, m.spinner(m.elapsed)+" refreshing")
		}
		return m.st.Paint(ui.RoleDim, m.spinner(m.elapsed)+" connecting "+ui.FormatElapsed(m.elapsed))
	}
	for _, tm := range m.inv.Timings {
		if tm.Machine != want {
			continue
		}
		fail := m.st.Glyphs().Fail
		switch {
		case errors.Is(tm.Err, context.DeadlineExceeded):
			return m.st.Paint(ui.RoleBlocked, fail+" timeout "+ui.FormatElapsed(tm.Elapsed))
		case tm.Err != nil:
			return m.st.Paint(ui.RoleBlocked, fail+" failed")
		case want == "":
			return ""
		}
		return m.st.Paint(ui.RoleDim, ui.FormatElapsed(tm.Elapsed))
	}
	return ""
}

// footerInfo devuelve las migas "maquina / workspace / pane" y la ruta.
func (m Model) footerInfo() (crumb, path string) {
	t, ok := m.selectedTarget()
	if !ok {
		return "no agent selected", ""
	}
	return t.MachineLabel() + " / " + t.Workspace + " / " + t.PaneID, t.Cwd
}

// keysText es la línea de teclas del pie, según la pantalla.
func (m Model) keysText() string {
	up, sep := "↑↓", " · "
	if m.st.ASCII() {
		up, sep = "up/down", " - "
	}
	var keys []string
	switch {
	case m.mode == modeMenu:
		keys = []string{up + " select", "enter run", "a/s/l/n/g/w/c shortcut", "esc close"}
	case m.mode == modeInput && m.inputKind == "rename":
		keys = []string{"enter rename", "ctrl+u clear", "esc cancel"}
	case m.mode == modeInput:
		keys = []string{"enter send", "ctrl+u clear", "esc cancel"}
	case m.mode == modeBusy:
		keys = []string{"esc stop waiting (the agent keeps working)"}
	case m.mode == modeView:
		keys = []string{up + "/PgUp/PgDn scroll", "any other key: back"}
	case m.searching:
		keys = []string{"type to search", up + " rows", "enter actions", "ctrl+u clear", "esc cancel"}
	default:
		keys = []string{up + "/j/k rows", "←→ group", "/ search", "b/w/i/d/a filter", "r refresh", "enter actions", "q quit"}
		if m.st.ASCII() {
			keys[1] = "left/right group"
		}
	}
	return strings.Join(keys, sep)
}

// spinner devuelve el cuadro del indicador de espera para un tiempo transcurrido.
func (m Model) spinner(elapsed time.Duration) string {
	return ui.Spinner(m.st.ASCII(), elapsed)
}
