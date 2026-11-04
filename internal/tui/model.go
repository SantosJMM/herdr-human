// Paquete tui es el selector interactivo de agentes (Bubble Tea). El modelo no
// toca la terminal: recibe mensajes y devuelve texto, así que se prueba sin
// pantalla. Dibuja con el kit de internal/ui para verse igual que ls y attention.
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// Dimensiones por defecto y mínimas: debajo del mínimo el marco no cabe.
const (
	defaultWidth  = 80
	defaultHeight = 24
	minWidth      = 14
	chromeLines   = 8 // borde, búsqueda, 2 separadores, 3 de pie y borde
	bodyTop       = 3 // líneas por encima del cuerpo (para traducir clics)
	wheelStep     = 3
)

// Options son los datos y el aspecto del selector.
type Options struct {
	Inventory inventory.Inventory
	Style     *ui.Style
	// FullNames muestra los títulos enteros en vez de su primer segmento.
	FullNames bool

	// Machines y Updates activan la carga progresiva: Machines son los
	// servidores que se consultan (el local es "") y cada respuesta llega por
	// Updates. Mientras no llega, la máquina se muestra como "connecting".
	// Con Updates nulo se usa Inventory, ya completo.
	Machines []string
	Updates  <-chan inventory.Update

	// Actions ejecuta las acciones del menú; nulo las deshabilita. Ctx acota las
	// acciones en curso (por defecto context.Background).
	Actions Actions
	Ctx     context.Context

	// Refresh vuelve a consultar una máquina ("" es el local). Con ella, tras una
	// acción se refresca la máquina del agente y la tecla r las refresca todas.
	// Solo se usa con carga progresiva.
	Refresh func(ctx context.Context, machine string) ([]inventory.Target, error)

	// Branch devuelve la rama de git de una carpeta del servidor local ("" si no
	// la hay). Solo se pregunta por workspaces locales y una vez por carpeta.
	Branch func(dir string) string
}

// RefreshMsg es el resultado de volver a consultar una máquina.
type RefreshMsg inventory.Update

// UpdateMsg es la respuesta de un servidor, que el selector recibe del canal.
type UpdateMsg = inventory.Update

// TickMsg refresca el tiempo transcurrido y el indicador de espera.
type TickMsg struct{ Elapsed time.Duration }

// DoneMsg indica que el canal de respuestas se cerró.
type DoneMsg struct{}

const tickEvery = 100 * time.Millisecond

type rowKind int

const (
	kindMachine rowKind = iota
	kindWorkspace
	kindAgent
)

// row es una línea del cuerpo, antes de dibujarla.
type row struct {
	kind    rowKind
	machine string // etiqueta, o inventory.LocalLabel
	text    string // nombre de la máquina o del workspace
	branch  string // rama git del workspace (solo local)
	target  int    // índice en Inventory.Targets (solo kindAgent)
	last    bool   // último agente de su workspace
	group   int    // grupo (workspace) al que pertenece el agente
}

// Model es el estado del selector.
type Model struct {
	inv  inventory.Inventory
	st   *ui.Style
	full bool

	width, height int

	filter    string // "", blocked, working, idle o done
	query     string
	qpos      int // posición del cursor en la búsqueda, en letras
	searching bool

	rows   []row
	agents []int // índices en rows de las filas seleccionables
	cursor int   // posición dentro de agents
	top    int   // primera línea visible del cuerpo

	aw, sw int // anchos de las columnas de tipo y estado

	// Carga progresiva. order son los servidores en orden ("" es el local).
	stream     <-chan inventory.Update
	order      []string
	pending    map[string]bool
	perMachine map[string][]inventory.Target
	timings    map[string]inventory.Timing
	start      time.Time
	elapsed    time.Duration
	ticking    bool // hay un reloj en marcha mientras alguna máquina esté pendiente
	refresh    func(ctx context.Context, machine string) ([]inventory.Target, error)
	branchOf   func(dir string) string
	branches   map[string]string // rama por carpeta, para no leer el disco en cada tecla

	// Menú de acciones y pantallas que sustituyen a la lista.
	mode        mode
	menuCursor  int
	inputKind   string // ask, say o rename
	input       string
	ipos        int    // posición del cursor en la línea de texto, en letras
	notice      string // último resultado, en la segunda línea del pie
	noticeErr   bool
	act         Actions
	ctx         context.Context
	cancel      context.CancelFunc
	seq         int
	busyLabel   string
	busyElapsed time.Duration
	busyStart   time.Time
	viewTitle   string
	viewLines   []string
	viewTop     int

	chosen    inventory.Target
	hasChosen bool
}

// New crea el selector con el cursor en el primer agente.
func New(o Options) Model {
	m := Model{inv: o.Inventory, st: o.Style, full: o.FullNames, width: defaultWidth, height: defaultHeight,
		act: o.Actions, ctx: o.Ctx, refresh: o.Refresh, branchOf: o.Branch, branches: map[string]string{}}
	if m.ctx == nil {
		m.ctx = context.Background()
	}
	// Los nombres pueden cambiar con Rename: no se modifica la lista de quien llama.
	m.inv.Targets = slices.Clone(o.Inventory.Targets)
	if m.st == nil {
		m.st = ui.NewStyle(ui.Capability{}, ui.Theme{})
	}
	if o.Updates != nil {
		m.stream, m.start, m.ticking = o.Updates, time.Now(), true
		m.order = append([]string{""}, o.Machines...)
		m.pending = map[string]bool{}
		m.perMachine = map[string][]inventory.Target{}
		m.timings = map[string]inventory.Timing{}
		seen := map[string]bool{}
		order := m.order[:0:0]
		for _, label := range m.order {
			if !seen[label] {
				seen[label] = true
				order = append(order, label)
				m.pending[label] = true
			}
		}
		m.order = order
	}
	m.measure()
	m.rebuild("")
	return m
}

// measure calcula el ancho de las columnas de tipo y estado.
func (m *Model) measure() {
	m.aw, m.sw = 0, 0
	for _, t := range m.inv.Targets {
		m.aw = max(m.aw, ui.VisibleLen(t.Agent))
		m.sw = max(m.sw, ui.VisibleLen(t.Status))
	}
}

// applyUpdate incorpora la respuesta de un servidor sin mover la selección.
func (m *Model) applyUpdate(u inventory.Update) {
	keep := m.keepRef()
	delete(m.pending, u.Machine)
	m.perMachine[u.Machine] = u.Targets
	m.timings[u.Machine] = inventory.Timing{Machine: u.Machine, Elapsed: u.Elapsed, Err: u.Err}

	m.inv.Targets, m.inv.Timings = nil, nil
	for _, label := range m.order {
		m.inv.Targets = append(m.inv.Targets, m.perMachine[label]...)
		if tm, ok := m.timings[label]; ok {
			m.inv.Timings = append(m.inv.Timings, tm)
		}
	}
	m.measure()
	m.rebuild(keep)
}

// waitFor espera la siguiente respuesta del canal.
func waitFor(ch <-chan inventory.Update) tea.Cmd {
	return func() tea.Msg {
		u, ok := <-ch
		if !ok {
			return DoneMsg{}
		}
		return UpdateMsg(u)
	}
}

func tick(start time.Time) tea.Cmd {
	return tea.Tick(tickEvery, func(time.Time) tea.Msg { return TickMsg{Elapsed: time.Since(start)} })
}

// Result devuelve el agente elegido con Enter; false si se salió sin elegir.
func (m Model) Result() (inventory.Target, bool) { return m.chosen, m.hasChosen }

// Init empieza a escuchar las respuestas si la carga es progresiva.
func (m Model) Init() tea.Cmd {
	if m.stream == nil {
		return nil
	}
	return tea.Batch(waitFor(m.stream), tick(m.start))
}

// Run abre el selector a pantalla completa y devuelve el agente elegido, o
// false si la persona salió sin elegir o se canceló el contexto.
func Run(ctx context.Context, in io.Reader, out io.Writer, o Options) (inventory.Target, bool, error) {
	p := tea.NewProgram(New(o),
		tea.WithAltScreen(), tea.WithMouseCellMotion(),
		tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx))
	final, err := p.Run()
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, tea.ErrProgramKilled) {
			return inventory.Target{}, false, nil
		}
		return inventory.Target{}, false, err
	}
	t, ok := final.(Model).Result()
	return t, ok, nil
}

// rank ordena dentro de un workspace: lo que necesita a la persona primero.
func rank(status string) int {
	switch status {
	case "blocked":
		return 0
	case "done":
		return 1
	}
	return 2
}

// machineOrder devuelve las etiquetas de máquina: primero el local, luego las
// consultadas en su orden, y por último las que solo aparecen en los agentes.
func (m *Model) machineOrder() []string {
	seen := map[string]bool{}
	var order []string
	add := func(label string) {
		if !seen[label] {
			seen[label] = true
			order = append(order, label)
		}
	}
	add(inventory.LocalLabel)
	for _, raw := range m.order {
		if raw == "" {
			add(inventory.LocalLabel)
		} else {
			add(raw)
		}
	}
	for _, tm := range m.inv.Timings {
		if tm.Machine == "" {
			add(inventory.LocalLabel)
		} else {
			add(tm.Machine)
		}
	}
	for _, t := range m.inv.Targets {
		add(t.MachineLabel())
	}
	return order
}

// rebuild recalcula las filas según el filtro y la búsqueda. keep es el Ref que
// debe conservar la selección; vacío deja el cursor en el primer agente.
func (m *Model) rebuild(keep string) {
	m.rows, m.agents = nil, nil
	group := 0
	filtering := m.filter != "" || m.query != ""
	for _, label := range m.machineOrder() {
		var wsOrder []string
		byWS := map[string][]int{}
		for i, t := range m.inv.Targets {
			if t.MachineLabel() != label || !m.visible(t) {
				continue
			}
			if _, ok := byWS[t.Workspace]; !ok {
				wsOrder = append(wsOrder, t.Workspace)
			}
			byWS[t.Workspace] = append(byWS[t.Workspace], i)
		}
		// Las máquinas que cargan o fallaron se muestran siempre, para que se vea por qué.
		if len(wsOrder) == 0 && filtering && !m.unsettled(label) {
			continue
		}
		m.rows = append(m.rows, row{kind: kindMachine, machine: label, text: label})
		for _, ws := range wsOrder {
			idx := byWS[ws]
			sort.SliceStable(idx, func(a, b int) bool {
				return rank(m.inv.Targets[idx[a]].Status) < rank(m.inv.Targets[idx[b]].Status)
			})
			m.rows = append(m.rows, row{kind: kindWorkspace, machine: label, text: ws, branch: m.workspaceBranch(label, idx)})
			for n, ti := range idx {
				m.agents = append(m.agents, len(m.rows))
				m.rows = append(m.rows, row{kind: kindAgent, machine: label, target: ti, last: n == len(idx)-1, group: group})
			}
			group++
		}
	}

	m.cursor = 0
	if keep != "" {
		for n, ri := range m.agents {
			if m.inv.Targets[m.rows[ri].target].Ref() == keep {
				m.cursor = n
				break
			}
		}
	}
	m.ensureVisible()
}

// workspaceBranch devuelve la rama git de un workspace local, a partir de la
// carpeta del primer agente que la tenga. En una máquina remota costaría otra
// llamada por SSH, así que no se busca.
func (m *Model) workspaceBranch(label string, idx []int) string {
	if m.branchOf == nil || label != inventory.LocalLabel {
		return ""
	}
	for _, ti := range idx {
		dir := m.inv.Targets[ti].Cwd
		if dir == "" {
			continue
		}
		name, known := m.branches[dir]
		if !known {
			name = m.branchOf(dir)
			m.branches[dir] = name
		}
		return name
	}
	return ""
}

// unsettled dice si la máquina está cargando o falló.
func (m *Model) unsettled(label string) bool {
	raw := label
	if label == inventory.LocalLabel {
		raw = ""
	}
	return m.pending[raw] || m.timings[raw].Err != nil
}

// visible dice si el agente pasa el filtro de estado y la búsqueda.
func (m *Model) visible(t inventory.Target) bool {
	if m.filter != "" && t.Status != m.filter {
		return false
	}
	return matches(t, m.query)
}

// matches comprueba cada palabra de q contra el nombre, tipo, workspace,
// máquina, tab, ruta y pane; además admite letras en orden dentro del nombre.
func matches(t inventory.Target, q string) bool {
	words := strings.Fields(strings.ToLower(q))
	if len(words) == 0 {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{t.Name, t.Agent, t.Workspace, t.MachineLabel(), t.TabID, t.Cwd, t.PaneID}, " "))
	fuzzy := strings.ToLower(t.Name + " " + t.Workspace)
	for _, w := range words {
		if !strings.Contains(hay, w) && !subsequence(fuzzy, w) {
			return false
		}
	}
	return true
}

// subsequence dice si las letras de needle aparecen en orden dentro de hay.
func subsequence(hay, needle string) bool {
	h := []rune(hay)
	i := 0
	for _, r := range needle {
		for i < len(h) && h[i] != r {
			i++
		}
		if i == len(h) {
			return false
		}
		i++
	}
	return true
}

func (m Model) bodyHeight() int { return max(m.height, chromeLines+1) - chromeLines }

// selected devuelve la posición en rows del agente seleccionado, o -1.
func (m Model) selected() int {
	if len(m.agents) == 0 {
		return -1
	}
	return m.agents[m.cursor]
}

func (m Model) selectedTarget() (inventory.Target, bool) {
	if s := m.selected(); s >= 0 {
		return m.inv.Targets[m.rows[s].target], true
	}
	return inventory.Target{}, false
}

// ensureVisible desplaza el cuerpo para que la fila seleccionada se vea, junto
// con los encabezados que tiene justo encima.
func (m *Model) ensureVisible() {
	bh := m.bodyHeight()
	if s := m.selected(); s >= 0 {
		want := s
		for want > 0 && m.rows[want-1].kind != kindAgent {
			want--
		}
		if want < m.top {
			m.top = want
		}
		if s >= m.top+bh {
			m.top = s - bh + 1
		}
	}
	m.top = max(0, min(m.top, len(m.rows)-bh))
}

func (m *Model) move(to int) {
	if len(m.agents) == 0 {
		return
	}
	m.cursor = max(0, min(to, len(m.agents)-1))
	m.ensureVisible()
}

// jumpGroup salta al primer agente del workspace anterior (-1) o siguiente (+1).
func (m *Model) jumpGroup(dir int) {
	if len(m.agents) == 0 {
		return
	}
	cur := m.rows[m.agents[m.cursor]].group
	target := cur + dir
	if target < 0 {
		m.move(0)
		return
	}
	for n, ri := range m.agents {
		if m.rows[ri].group == target {
			m.move(n)
			return
		}
	}
}

// Update procesa teclas, ratón y cambios de tamaño.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ensureVisible()
		return m, nil
	case UpdateMsg:
		m.applyUpdate(msg)
		return m, waitFor(m.stream)
	case TickMsg:
		m.elapsed = msg.Elapsed
		if len(m.pending) > 0 {
			return m, tick(m.start)
		}
		m.ticking = false
		return m, nil
	case DoneMsg:
		return m, nil
	case RefreshMsg:
		if msg.Err != nil {
			delete(m.pending, msg.Machine)
			m.notify(fmt.Sprintf("refresh failed for %s: %v", machineName(msg.Machine), msg.Err), true)
			return m, nil
		}
		m.applyUpdate(inventory.Update(msg))
		return m, nil
	case ActionMsg:
		return m, m.onAction(msg)
	case BusyTickMsg:
		if m.mode == modeBusy && msg.Seq == m.seq {
			m.busyElapsed = msg.Elapsed
			return m, busyTick(m.seq, m.busyStart)
		}
		return m, nil
	case tea.KeyMsg:
		return m.onKey(msg)
	case tea.MouseMsg:
		return m.onMouse(msg), nil
	}
	return m, nil
}

func (m Model) keepRef() string {
	if t, ok := m.selectedTarget(); ok {
		return t.Ref()
	}
	return ""
}

func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeList {
		return m.onModeKey(msg)
	}
	key := msg.String()
	m.notice = ""
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.openMenu()
		return m, nil
	case "ctrl+a":
		return m, m.activate("ask")
	case "ctrl+s":
		return m, m.activate("say")
	case "ctrl+l":
		return m, m.activate("look")
	case "ctrl+n":
		return m, m.activate("rename")
	case "up":
		m.move(m.cursor - 1)
		return m, nil
	case "down":
		m.move(m.cursor + 1)
		return m, nil
	case "pgup":
		m.move(m.cursor - m.bodyHeight())
		return m, nil
	case "pgdown":
		m.move(m.cursor + m.bodyHeight())
		return m, nil
	}

	if m.searching {
		return m.onSearchKey(msg)
	}

	switch key {
	case "q", "esc":
		return m, tea.Quit
	case "j":
		m.move(m.cursor + 1)
	case "k":
		m.move(m.cursor - 1)
	case "g", "home":
		m.move(0)
	case "G", "end":
		m.move(len(m.agents) - 1)
	case "left", "h":
		m.jumpGroup(-1)
	case "right", "l":
		m.jumpGroup(1)
	case "/":
		m.searching, m.qpos = true, len([]rune(m.query))
	case "r":
		if m.refresh == nil || m.stream == nil {
			m.notify("refresh: not available", true)
			return m, nil
		}
		return m, m.refreshMachines(m.order...)
	case "b", "w", "i", "d", "a":
		m.filter = map[string]string{"b": "blocked", "w": "working", "i": "idle", "d": "done", "a": ""}[key]
		m.rebuild(m.keepRef())
	}
	return m, nil
}

// onSearchKey edita la búsqueda: todo lo imprimible es texto, también b/w/i/d/a.
// El cursor se mueve con ←→, Inicio y Fin; solo se recalcula la lista si el
// texto cambió.
func (m Model) onSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.searching, m.query, m.qpos = false, "", 0
		m.rebuild("")
		return m, nil
	}
	var changed bool
	m.query, m.qpos, changed = edit(m.query, m.qpos, msg)
	if changed {
		m.rebuild("")
	}
	return m, nil
}

func (m Model) onMouse(msg tea.MouseMsg) Model {
	if m.mode != modeList || msg.Action != tea.MouseActionPress {
		return m
	}
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		m.move(m.cursor + wheelStep)
	case tea.MouseButtonWheelUp:
		m.move(m.cursor - wheelStep)
	case tea.MouseButtonLeft:
		line := m.top + msg.Y - bodyTop
		if msg.Y < bodyTop || msg.Y-bodyTop >= m.bodyHeight() || line < 0 || line >= len(m.rows) {
			return m
		}
		for n, ri := range m.agents {
			if ri == line {
				m.move(n)
				break
			}
		}
	}
	return m
}
