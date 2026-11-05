package tui

import (
	"context"
	"errors"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/palette"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// PluginUpdate es la lista fresca de acciones de plugins que llega después de
// abrir la paleta. La paleta se pinta antes, con lo embebido y la caché.
type PluginUpdate struct {
	Entries []palette.Entry
	Err     error
}

// PaletteOptions configura la paleta.
type PaletteOptions struct {
	Style *ui.Style
	// Entries se dibujan en el primer cuadro, sin esperar a nadie.
	Entries []palette.Entry
	// Updates entrega, una sola vez, las acciones de plugins recién leídas. Nil
	// si no hay nada que esperar.
	Updates <-chan PluginUpdate
	// Merge combina las acciones de plugins con el resto (atajos, recientes,
	// orden) y devuelve la lista completa.
	Merge func(plugins []palette.Entry) []palette.Entry
	// Resolve arma los argumentos de un comando nativo; puede preguntar a la
	// persona por medio del Prompter.
	Resolve func(ctx context.Context, e palette.Entry, p palette.Prompter) (argv []string, ok bool, err error)
	Ctx     context.Context
}

// PaletteResult es lo que eligió la persona.
type PaletteResult struct {
	Entry  palette.Entry
	Argv   []string
	Chosen bool
	// Err es el fallo al resolver el comando, si lo hubo.
	Err error
}

type palMode int

const (
	palList palMode = iota
	palInput
	palChoose
	palBusy
	palError
)

// Mensajes internos de la paleta.
type (
	askInputMsg struct {
		spec  palette.InputSpec
		reply chan<- inputReply
	}
	askChooseMsg struct {
		prompt  string
		choices []palette.Choice
		reply   chan<- chooseReply
	}
	resolvedMsg struct {
		entry palette.Entry
		argv  []string
		ok    bool
		err   error
	}
)

type (
	inputReply struct {
		text string
		ok   bool
	}
	chooseReply struct {
		value string
		ok    bool
	}
)

// palBridge deja que la goroutine que resuelve argumentos pregunte por la
// pantalla. Implementa palette.Prompter.
type palBridge struct{ send func(tea.Msg) }

func (b *palBridge) Input(ctx context.Context, spec palette.InputSpec) (string, bool, error) {
	reply := make(chan inputReply, 1)
	b.send(askInputMsg{spec: spec, reply: reply})
	select {
	case r := <-reply:
		return r.text, r.ok, nil
	case <-ctx.Done():
		return "", false, nil
	}
}

func (b *palBridge) Choose(ctx context.Context, prompt string, choices []palette.Choice) (string, bool, error) {
	reply := make(chan chooseReply, 1)
	b.send(askChooseMsg{prompt: prompt, choices: choices, reply: reply})
	select {
	case r := <-reply:
		return r.value, r.ok, nil
	case <-ctx.Done():
		return "", false, nil
	}
}

// prow es una fila de la lista: lo que se ve y el texto sobre el que se busca.
// sourceOff y titleOff dicen dónde empiezan el origen y el título dentro de
// search (en runas; -1 si esa columna no está en el texto de búsqueda).
type prow struct {
	title, source, chord, search string
	sourceOff, titleOff          int
}

// picker es una lista con búsqueda, selección y desplazamiento.
type picker struct {
	rows  []prow
	vis   []int
	query string
	qpos  int
	sel   int
	top   int
}

func (p *picker) refilter(keep string, keyOf func(int) string) {
	all := make([]int, len(p.rows))
	for i := range all {
		all[i] = i
	}
	p.vis = palette.Filter(all, p.query, func(i int) string { return p.rows[i].search })
	p.sel, p.top = 0, 0
	if keep == "" {
		return
	}
	for i, idx := range p.vis {
		if keyOf(idx) == keep {
			p.sel = i
			return
		}
	}
}

func (p *picker) move(to, page int) {
	if len(p.vis) == 0 {
		return
	}
	p.sel = max(0, min(len(p.vis)-1, to))
	if p.sel < p.top {
		p.top = p.sel
	}
	if p.sel >= p.top+page {
		p.top = p.sel - page + 1
	}
}

// current devuelve el índice, dentro de rows, de la fila seleccionada.
func (p *picker) current() (int, bool) {
	if p.sel < 0 || p.sel >= len(p.vis) {
		return 0, false
	}
	return p.vis[p.sel], true
}

// edit aplica una tecla de edición a la consulta. Devuelve true si cambió.
func (p *picker) edit(msg tea.KeyMsg) bool {
	// Unix manda DEL para Backspace; ConPTY y Windows Terminal pueden mandar BS
	// (Ctrl+H) para la misma tecla, y Bubble Tea lo lee como Ctrl+H. El plugin
	// original ya distinguía este caso: sin él, Backspace no borra en esas terminales.
	if msg.Type == tea.KeyCtrlH {
		msg.Type = tea.KeyBackspace
	}
	if msg.Type == tea.KeyCtrlW {
		r := []rune(strings.TrimRight(string([]rune(p.query)[:p.qpos]), " "))
		for len(r) > 0 && r[len(r)-1] != ' ' {
			r = r[:len(r)-1]
		}
		p.query = string(r) + string([]rune(p.query)[p.qpos:])
		p.qpos = len(r)
		return true
	}
	text, pos, changed := edit(p.query, p.qpos, msg)
	p.query, p.qpos = text, pos
	return changed
}

// PaletteModel es la paleta de comandos.
type PaletteModel struct {
	st     *ui.Style
	opts   PaletteOptions
	bridge *palBridge

	entries []palette.Entry
	list    picker

	mode    palMode
	prompt  string
	choices picker
	values  []string
	input   picker // se reutiliza solo query y qpos
	spec    palette.InputSpec
	status  string

	inputReply  chan<- inputReply
	chooseReply chan<- chooseReply

	loading    bool
	pluginsErr error
	width      int
	height     int
	result     PaletteResult
}

// NewPalette crea el modelo con las entradas ya listas para el primer cuadro.
func NewPalette(o PaletteOptions) PaletteModel {
	m := PaletteModel{st: o.Style, opts: o, bridge: &palBridge{send: func(tea.Msg) {}}, loading: o.Updates != nil}
	if m.st == nil {
		m.st = ui.NewStyle(ui.Capability{}, ui.Theme{})
	}
	m.setEntries(o.Entries, "")
	return m
}

func (m *PaletteModel) setEntries(entries []palette.Entry, keep string) {
	m.entries = entries
	m.list.rows = make([]prow, len(entries))
	for i, e := range entries {
		so, to := e.SearchOffsets()
		m.list.rows[i] = prow{title: e.Title, source: e.Source, chord: e.Chord, search: e.SearchText(), sourceOff: so, titleOff: to}
	}
	m.list.refilter(keep, func(i int) string { return m.entries[i].ID })
}

func (m PaletteModel) selectedID() string {
	if i, ok := m.list.current(); ok {
		return m.entries[i].ID
	}
	return ""
}

// Result devuelve lo elegido.
func (m PaletteModel) Result() PaletteResult { return m.result }

// Tamaño que se asume mientras la terminal no informa el suyo.
const (
	palDefaultWidth  = 80
	palDefaultHeight = 24
)

func (m PaletteModel) size() (w, h int) {
	w, h = m.width, m.height
	if w == 0 {
		w = palDefaultWidth
	}
	if h == 0 {
		h = palDefaultHeight
	}
	return max(w, minWidth), max(h, palChrome+1)
}

func (m PaletteModel) pageSize() int {
	_, h := m.size()
	return h - palChrome
}

// Init espera las acciones de plugins, si las hay.
func (m PaletteModel) Init() tea.Cmd {
	if m.opts.Updates == nil {
		return nil
	}
	ch := m.opts.Updates
	return func() tea.Msg {
		return <-ch
	}
}

// Update procesa un mensaje.
func (m PaletteModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.move(m.list.sel, m.pageSize())
		return m, nil

	case PluginUpdate:
		m.loading = false
		m.pluginsErr = msg.Err
		if msg.Err == nil && m.opts.Merge != nil {
			// Si la persona no ha movido la selección, sigue al mejor resultado,
			// que puede cambiar al llegar las acciones nuevas; si ya navegó, se
			// queda en lo que tenía.
			keep := ""
			if m.list.sel > 0 {
				keep = m.selectedID()
			}
			m.setEntries(m.opts.Merge(msg.Entries), keep)
		}
		return m, nil

	case askInputMsg:
		m.mode, m.prompt, m.spec, m.status = palInput, msg.spec.Prompt, msg.spec, ""
		m.input = picker{query: msg.spec.Initial, qpos: len([]rune(msg.spec.Initial))}
		m.inputReply = msg.reply
		return m, nil

	case askChooseMsg:
		m.mode, m.prompt, m.chooseReply = palChoose, msg.prompt, msg.reply
		m.values = make([]string, len(msg.choices))
		m.choices = picker{rows: make([]prow, len(msg.choices))}
		for i, c := range msg.choices {
			m.values[i] = c.Value
			m.choices.rows[i] = prow{title: c.Label, search: c.Label, sourceOff: -1}
		}
		m.choices.refilter("", nil)
		return m, nil

	case resolvedMsg:
		if msg.err != nil {
			m.mode, m.status = palError, msg.err.Error()
			m.result.Err = msg.err
			return m, nil
		}
		if msg.ok {
			m.result.Entry, m.result.Argv, m.result.Chosen = msg.entry, msg.argv, true
		}
		return m, tea.Quit

	case tea.MouseMsg:
		return m.onMouse(msg), nil

	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m PaletteModel) onMouse(msg tea.MouseMsg) PaletteModel {
	p := &m.list
	if m.mode == palChoose {
		p = &m.choices
	} else if m.mode != palList {
		return m
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		p.move(p.sel-1, m.pageSize())
	case tea.MouseButtonWheelDown:
		p.move(p.sel+1, m.pageSize())
	}
	return m
}

func (m PaletteModel) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
		return m.cancel()
	}
	switch m.mode {
	case palError:
		return m, tea.Quit
	case palBusy:
		return m, nil
	case palInput:
		return m.onInputKey(msg)
	case palChoose:
		return m.onListKey(msg, &m.choices, func(m *PaletteModel, row int) (tea.Model, tea.Cmd) {
			m.chooseReply <- chooseReply{value: m.values[row], ok: true}
			m.mode = palBusy
			return *m, nil
		})
	}
	return m.onListKey(msg, &m.list, func(m *PaletteModel, row int) (tea.Model, tea.Cmd) {
		return m.pick(m.entries[row])
	})
}

// cancel cierra sin ejecutar nada. Si hay una pregunta abierta se le avisa a
// quien resuelve los argumentos, para que no quede esperando.
func (m PaletteModel) cancel() (tea.Model, tea.Cmd) {
	switch m.mode {
	case palInput:
		m.inputReply <- inputReply{}
	case palChoose:
		m.chooseReply <- chooseReply{}
	}
	return m, tea.Quit
}

// pick actúa sobre la entrada elegida: una acción de plugin se devuelve tal cual
// y un comando nativo pasa a resolver sus argumentos.
func (m *PaletteModel) pick(e palette.Entry) (tea.Model, tea.Cmd) {
	if e.Kind != palette.KindNative || m.opts.Resolve == nil {
		m.result = PaletteResult{Entry: e, Chosen: true}
		return *m, tea.Quit
	}
	m.mode = palBusy
	resolve, ctx, bridge := m.opts.Resolve, m.opts.Ctx, m.bridge
	return *m, func() tea.Msg {
		argv, ok, err := resolve(ctx, e, bridge)
		return resolvedMsg{entry: e, argv: argv, ok: ok, err: err}
	}
}

// onListKey maneja las teclas de una lista. p apunta a un campo de m, así que
// m es un puntero: con una copia, los cambios en la lista se perderían.
func (m *PaletteModel) onListKey(msg tea.KeyMsg, p *picker, choose func(*PaletteModel, int) (tea.Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	page := m.pageSize()
	switch msg.Type {
	case tea.KeyEnter:
		if row, ok := p.current(); ok {
			return choose(m, row)
		}
		return *m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		p.move(p.sel-1, page)
	case tea.KeyDown, tea.KeyCtrlN:
		p.move(p.sel+1, page)
	case tea.KeyPgUp:
		p.move(p.sel-page, page)
	case tea.KeyPgDown:
		p.move(p.sel+page, page)
	case tea.KeyHome:
		p.move(0, page)
	case tea.KeyEnd:
		p.move(len(p.vis)-1, page)
	default:
		if p.edit(msg) {
			p.refilter("", nil)
		}
	}
	return *m, nil
}

func (m PaletteModel) onInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		if m.spec.Required && strings.TrimSpace(m.input.query) == "" {
			m.status = "This value is required."
			return m, nil
		}
		m.inputReply <- inputReply{text: m.input.query, ok: true}
		m.mode = palBusy
		return m, nil
	}
	if m.input.edit(msg) {
		m.status = ""
	}
	return m, nil
}

// RunPalette abre la paleta en pantalla completa y devuelve lo elegido.
func RunPalette(ctx context.Context, in io.Reader, out io.Writer, o PaletteOptions) (PaletteResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	o.Ctx = ctx
	m := NewPalette(o)
	p := tea.NewProgram(m,
		tea.WithAltScreen(), tea.WithMouseCellMotion(),
		tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx))
	m.bridge.send = p.Send
	final, err := p.Run()
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, tea.ErrProgramKilled) {
			return PaletteResult{}, nil
		}
		return PaletteResult{}, err
	}
	return final.(PaletteModel).Result(), nil
}
