package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// agentsTab es la pestaña de agentes: el selector de `hum pick` alojado en la
// pantalla de inicio, con su búsqueda, sus filtros y su menú de acciones.
type agentsTab struct{ m Model }

// NewAgentsTab crea la pestaña de agentes. El selector se dibuja sin marco (la
// pantalla de inicio ya lo tiene) y, al elegir un agente, avisa con LeaveMsg.
func NewAgentsTab(o Options) Tab {
	o.Hosted, o.Embedded = true, true
	return agentsTab{m: New(o)}
}

func (agentsTab) Title() string { return "Agents" }

func (t agentsTab) Init() tea.Cmd { return t.m.Init() }

func (t agentsTab) Captures() bool { return t.m.Capturing() }

func (t agentsTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	next, cmd := t.m.Update(msg)
	t.m = next.(Model)
	return t, cmd
}

// View devuelve el selector entero: ya viene con el tamaño del hueco, que la
// pantalla de inicio le dio con WindowSizeMsg.
func (t agentsTab) View(int, int) []string { return strings.Split(t.m.View(), "\n") }
