package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// viewFlags son las opciones de presentación de los comandos que dibujan listas.
type viewFlags struct {
	color     string
	fullNames bool
}

func (v *viewFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&v.color, "color", "auto", "colorize the output: auto, always or never")
	fs.BoolVar(&v.fullNames, "full-names", false, "show whole agent titles instead of their first segment")
}

func (a *app) getenv(key string) string {
	if a.env.Getenv == nil {
		return ""
	}
	return a.env.Getenv(key)
}

// newStyle decide qué se puede mostrar y devuelve el estilo. Un valor vacío de
// --color equivale a auto.
func (a *app) newStyle(color string) (*ui.Style, error) {
	mode := ui.ColorAuto
	if color != "" {
		var err error
		if mode, err = ui.ParseMode(color); err != nil {
			return nil, usagef("%v", err)
		}
	}
	return ui.NewStyle(ui.Detect(mode, a.env.Terminal, a.env.GOOS, a.getenv), a.env.Theme), nil
}

// viewWidth es el ancho al que se alinea la salida: el de la terminal, o 0
// ("el que pida el contenido") si la salida va a una tubería.
func (a *app) viewWidth() int {
	if !a.env.Terminal {
		return 0
	}
	return ui.WidthFrom(a.env.Width, a.getenv)
}

// machineGroup reúne lo que se dibuja de una máquina.
type machineGroup struct {
	label      string
	timing     *inventory.Timing
	workspaces []*workspaceGroup
}

type workspaceGroup struct {
	name string
	rows []inventory.Target
}

// groupByMachine ordena por máquina (primero el local) y workspace, conservando
// el orden de herdr dentro de cada uno. Las máquinas sin agentes también
// aparecen, para poder mostrar su estado.
func groupByMachine(inv inventory.Inventory) []*machineGroup {
	var groups []*machineGroup
	find := func(label string) *machineGroup {
		for _, g := range groups {
			if g.label == label {
				return g
			}
		}
		g := &machineGroup{label: label}
		groups = append(groups, g)
		return g
	}

	for i := range inv.Timings {
		label := inv.Timings[i].Machine
		if label == "" {
			label = inventory.LocalLabel
		}
		find(label).timing = &inv.Timings[i]
	}
	for _, t := range inv.Targets {
		mg := find(t.MachineLabel())
		var wg *workspaceGroup
		for _, existing := range mg.workspaces {
			if existing.name == t.Workspace {
				wg = existing
			}
		}
		if wg == nil {
			wg = &workspaceGroup{name: t.Workspace}
			mg.workspaces = append(mg.workspaces, wg)
		}
		wg.rows = append(wg.rows, t)
	}
	return groups
}

// headerText es el nombre de la máquina tal como se muestra.
func headerText(label string) string {
	if label == inventory.LocalLabel {
		return "Local"
	}
	return label
}

// machineStatus es lo que va a la derecha del encabezado de una máquina: el
// tiempo que tardó o el motivo por el que falló. El servidor local no muestra
// tiempo porque responde al instante.
func machineStatus(t *inventory.Timing, st *ui.Style) string {
	if t == nil {
		return ""
	}
	fail := st.Glyphs().Fail
	switch {
	case errors.Is(t.Err, errPending):
		return st.Paint(ui.RoleDim, ui.Spinner(st.ASCII(), t.Elapsed)+" connecting "+ui.FormatElapsed(t.Elapsed))
	case t.Cached:
		return st.Paint(ui.RoleDim, cachedText(t.Age))
	case errors.Is(t.Err, context.DeadlineExceeded):
		return st.Paint(ui.RoleBlocked, fail+" timeout "+ui.FormatElapsed(t.Elapsed))
	case t.Err != nil:
		return st.Paint(ui.RoleBlocked, fail+" failed")
	case t.Machine == "":
		return ""
	}
	return st.Paint(ui.RoleDim, ui.FormatElapsed(t.Elapsed))
}

// printTree dibuja máquina → workspace → agentes como el pane nativo de herdr:
// árbol con ramas, columnas alineadas a la derecha y el estado de cada máquina
// en su encabezado. En una terminal ocupa todo el ancho; en una tubería, solo
// el que necesita el contenido, sin recortar nada.
func (a *app) printTree(inv inventory.Inventory, st *ui.Style, full bool) {
	for _, l := range a.treeLines(inv, st, full) {
		fmt.Fprintln(a.env.Stdout, l)
	}
}

// treeLines dibuja el árbol como líneas ya ajustadas al ancho.
func (a *app) treeLines(inv inventory.Inventory, st *ui.Style, full bool) []string {
	g := st.Glyphs()
	groups := groupByMachine(inv)

	// Anchos de columna comunes a todos los grupos para que se alineen entre sí.
	var idw, aw, sw int
	for _, t := range inv.Targets {
		idw = max(idw, ui.VisibleLen(t.PaneID))
		aw = max(aw, ui.VisibleLen(t.Agent))
		sw = max(sw, ui.VisibleLen(t.Status))
	}

	type line struct{ left, right string }
	var out []line
	for _, mg := range groups {
		out = append(out, line{left: st.Bold(headerText(mg.label)), right: machineStatus(mg.timing, st)})
		for _, wg := range mg.workspaces {
			out = append(out, line{left: " " + st.Bold(wg.name)})
			for i, t := range wg.rows {
				branch := g.Branch
				if i == len(wg.rows)-1 {
					branch = g.LastBranch
				}
				mark := st.Paint(ui.StatusRole(t.Status), g.Agent)
				if t.Focused {
					mark = st.Paint(ui.RoleAccent, g.Current)
				}
				out = append(out, line{
					left: " " + st.Paint(ui.RoleDim, branch) + mark + " " + ui.PadRight(t.PaneID, idw) + "  " + ui.ShortName(t.Name, full),
					right: st.Paint(ui.RoleDim, ui.PadRight(t.Agent, aw)) + strings.Repeat(" ", ui.Gap) +
						st.Paint(ui.StatusRole(t.Status), ui.PadLeft(t.Status, sw)),
				})
			}
		}
	}

	width := a.viewWidth()
	if width == 0 {
		for _, l := range out {
			w := ui.VisibleLen(l.left)
			if l.right != "" {
				w += ui.Gap + ui.VisibleLen(l.right)
			}
			width = max(width, w)
		}
	}
	lines := make([]string, 0, len(out))
	for _, l := range out {
		if l.right == "" {
			lines = append(lines, ui.Truncate(l.left, width, g.Ellipsis))
			continue
		}
		lines = append(lines, ui.FitRow(width, l.left, l.right, g.Ellipsis))
	}
	return lines
}

// cachedText dice de cuándo es una respuesta guardada: "cached 3s".
func cachedText(age time.Duration) string {
	if age < time.Second {
		return "cached <1s"
	}
	return fmt.Sprintf("cached %ds", int(age.Seconds()))
}
