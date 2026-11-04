package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// mode es la pantalla que recibe las teclas.
type mode int

const (
	modeList  mode = iota // la lista de agentes
	modeMenu              // menú de acciones sobre el agente seleccionado
	modeInput             // línea de texto para Ask, Say o Rename
	modeBusy              // esperando el resultado de una acción
	modeView              // texto de solo lectura: Look o la respuesta de Ask
)

// Reply es la respuesta de un agente a Ask.
type Reply struct {
	Status string
	Text   string
	// AfterMessage dice si Text es lo que apareció tras el mensaje enviado; si
	// no, son las últimas Lines líneas de la pantalla.
	AfterMessage bool
	Lines        int
}

// Actions son las operaciones que el menú puede lanzar. Las implementa quien
// abre el selector, de modo que el modelo no conoce herdr y se prueba solo.
type Actions interface {
	Say(ctx context.Context, t inventory.Target, text string) error
	Look(ctx context.Context, t inventory.Target) (string, error)
	Rename(ctx context.Context, t inventory.Target, name string) error
	Ask(ctx context.Context, t inventory.Target, text string) (Reply, error)
	// WaitNotify espera a que el agente se asiente y avisa con una notificación;
	// devuelve el estado final.
	WaitNotify(ctx context.Context, t inventory.Target) (string, error)
	Copy(text string) error
}

// ActionMsg es el resultado de una acción lanzada desde el menú.
type ActionMsg struct {
	Seq    int
	Kind   string
	Target inventory.Target
	Err    error
	Text   string
	Reply  Reply
}

// BusyTickMsg refresca el indicador mientras se espera una acción.
type BusyTickMsg struct {
	Seq     int
	Elapsed time.Duration
}

type menuItem struct{ label, ascii, key, kind string }

var menuItems = []menuItem{
	{"Ask…", "Ask...", "a", "ask"},
	{"Say…", "Say...", "s", "say"},
	{"Look", "Look", "l", "look"},
	{"Rename…", "Rename...", "n", "rename"},
	{"Go to it", "Go to it", "g", "go"},
	{"Wait and notify", "Wait and notify", "w", "wait"},
	{"Copy pane id", "Copy pane id", "c", "copy"},
}

func busyTick(seq int, start time.Time) tea.Cmd {
	return tea.Tick(tickEvery, func(time.Time) tea.Msg {
		return BusyTickMsg{Seq: seq, Elapsed: time.Since(start)}
	})
}

func (m *Model) notify(text string, isErr bool) { m.notice, m.noticeErr = text, isErr }

func (m *Model) openMenu() {
	if m.selected() < 0 {
		return
	}
	m.mode, m.menuCursor = modeMenu, 0
}

// activate lanza una acción sobre el agente seleccionado.
func (m *Model) activate(kind string) tea.Cmd {
	t, ok := m.selectedTarget()
	if !ok {
		m.mode = modeList
		return nil
	}
	switch kind {
	case "ask", "say", "rename":
		m.mode, m.inputKind, m.input, m.ipos = modeInput, kind, "", 0
		return nil
	case "go":
		m.chosen, m.hasChosen = t, true
		return tea.Quit
	}
	m.mode = modeList
	if m.act == nil {
		m.notify(kind+": not available", true)
		return nil
	}
	act, ctx := m.act, m.ctx
	switch kind {
	case "copy":
		if err := act.Copy(t.PaneID); err != nil {
			m.notify(fmt.Sprintf("copy failed: %v (pane id: %s)", err, t.PaneID), true)
		} else {
			m.notify("copied "+t.PaneID, false)
		}
	case "look":
		return m.startBusy("look", "looking at "+t.Ref(), func(ctx context.Context) ActionMsg {
			text, err := act.Look(ctx, t)
			return ActionMsg{Text: text, Err: err}
		})
	case "wait":
		m.notify("waiting for "+t.Ref()+" (you will be notified)", false)
		return func() tea.Msg {
			status, err := act.WaitNotify(ctx, t)
			return ActionMsg{Kind: "wait", Target: t, Text: status, Err: err}
		}
	}
	return nil
}

// submit envía lo escrito si es válido; si no, deja la línea como está.
func (m *Model) submit() tea.Cmd {
	t, ok := m.selectedTarget()
	if !ok {
		m.mode = modeList
		return nil
	}
	text, kind := m.input, m.inputKind
	switch kind {
	case "say", "ask":
		if strings.TrimSpace(text) == "" {
			return nil
		}
	case "rename":
		if text != "" && actions.ValidName(text) != nil {
			return nil
		}
	}
	if m.act == nil {
		m.mode = modeList
		m.notify(kind+": not available", true)
		return nil
	}
	act := m.act
	switch kind {
	case "say":
		return m.startBusy("say", "sending to "+t.Ref(), func(ctx context.Context) ActionMsg {
			return ActionMsg{Err: act.Say(ctx, t, text)}
		})
	case "rename":
		return m.startBusy("rename", "renaming "+t.Ref(), func(ctx context.Context) ActionMsg {
			return ActionMsg{Text: text, Err: act.Rename(ctx, t, text)}
		})
	}
	return m.startBusy("ask", "asking "+t.Ref(), func(ctx context.Context) ActionMsg {
		r, err := act.Ask(ctx, t, text)
		return ActionMsg{Reply: r, Err: err}
	})
}

// startBusy lanza run en segundo plano y deja la pantalla esperando; Esc deja de
// esperar (el agente sigue trabajando) y descarta el resultado si llega tarde.
func (m *Model) startBusy(kind, label string, run func(context.Context) ActionMsg) tea.Cmd {
	t, _ := m.selectedTarget()
	if m.cancel != nil {
		m.cancel()
	}
	m.seq++
	seq := m.seq
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.mode, m.busyLabel, m.busyElapsed, m.busyStart = modeBusy, label, 0, time.Now()

	work := func() tea.Msg {
		msg := run(ctx)
		msg.Seq, msg.Kind, msg.Target = seq, kind, t
		return msg
	}
	return tea.Batch(work, busyTick(seq, m.busyStart))
}

// onAction incorpora el resultado de una acción. Si cambió algo en el agente
// (Say, Ask, Rename, Wait) devuelve el comando que refresca su máquina.
func (m *Model) onAction(msg ActionMsg) tea.Cmd {
	if msg.Kind == "wait" {
		if msg.Err != nil {
			m.notify("wait failed: "+msg.Err.Error(), true)
			return nil
		}
		m.notify(ui.ShortName(msg.Target.Name, m.full)+" is "+msg.Text, false)
		return m.refreshMachines(msg.Target.Machine)
	}
	if msg.Seq != m.seq || m.mode != modeBusy {
		return nil // se dejó de esperar: el resultado ya no interesa
	}
	m.mode = modeList
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if msg.Err != nil {
		m.notify(msg.Kind+" failed: "+msg.Err.Error(), true)
		return nil
	}
	ref := msg.Target.Ref()
	switch msg.Kind {
	case "say":
		m.notify("sent to "+ref, false)
	case "rename":
		m.setName(ref, msg.Text)
		if msg.Text == "" {
			m.notify("name cleared for "+ref, false)
		} else {
			m.notify("renamed "+ref+" to "+msg.Text, false)
		}
	case "look":
		m.showText("look", strings.Split(msg.Text, "\n"))
		return nil
	case "ask":
		m.showReply(msg.Reply)
	}
	return m.refreshMachines(msg.Target.Machine)
}

// machineName es cómo se nombra una máquina en los avisos.
func machineName(raw string) string {
	if raw == "" {
		return inventory.LocalLabel
	}
	return raw
}

// refreshMachines vuelve a consultar las máquinas indicadas ("" es el local) sin
// quitar las filas que ya se ven: su encabezado pasa a "refreshing" hasta que
// llega la respuesta. Sin función de refresco o sin carga progresiva no hace nada.
func (m *Model) refreshMachines(raws ...string) tea.Cmd {
	if m.refresh == nil || m.stream == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, raw := range raws {
		if m.pending[raw] {
			continue // ya se está consultando
		}
		m.pending[raw] = true
		if raw == "" {
			clear(m.branches) // la carpeta puede estar ya en otra rama
		}
		refresh, ctx := m.refresh, m.ctx
		cmds = append(cmds, func() tea.Msg {
			start := time.Now()
			targets, err := refresh(ctx, raw)
			return RefreshMsg{Machine: raw, Targets: targets, Err: err, Elapsed: time.Since(start)}
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	if !m.ticking {
		m.ticking = true
		cmds = append(cmds, tick(m.start))
	}
	return tea.Batch(cmds...)
}

// setName cambia el nombre visible de un agente en la lista y en la caché por máquina.
func (m *Model) setName(ref, name string) {
	if name == "" {
		return // herdr vuelve al título del terminal, que no conocemos
	}
	for i := range m.inv.Targets {
		if m.inv.Targets[i].Ref() == ref {
			m.inv.Targets[i].Name = name
		}
	}
	for _, targets := range m.perMachine {
		for i := range targets {
			if targets[i].Ref() == ref {
				targets[i].Name = name
			}
		}
	}
	m.rebuild(ref)
}

func (m *Model) showText(title string, lines []string) {
	m.mode, m.viewTitle, m.viewTop = modeView, title, 0
	m.viewLines = m.viewLines[:0:0]
	for _, l := range lines {
		m.viewLines = append(m.viewLines, sanitize(l))
	}
}

func (m *Model) showReply(r Reply) {
	var head string
	switch {
	case r.Status == "blocked":
		head = "waiting for you: " + lastLine(r.Text)
	case r.AfterMessage:
		head = r.Status + " · after your message"
	case r.Lines > 0:
		head = fmt.Sprintf("%s · (last %d lines)", r.Status, r.Lines)
	default:
		head = r.Status + " · (last lines)"
	}
	if m.st.ASCII() {
		head = strings.ReplaceAll(head, "·", "-")
	}
	m.showText("reply", append([]string{head, ""}, strings.Split(r.Text, "\n")...))
}

// sanitize quita del texto de un agente todo lo que pueda mover el cursor o
// cambiar la terminal (escapes y demás caracteres de control).
func sanitize(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			sb.WriteString("    ")
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\r\n \t"), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// onModeKey procesa las teclas de las pantallas que no son la lista.
func (m Model) onModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	switch m.mode {
	case modeMenu:
		return m.onMenuKey(key)
	case modeInput:
		return m.onInputKey(msg)
	case modeBusy:
		if key == "esc" {
			if m.cancel != nil {
				m.cancel()
				m.cancel = nil
			}
			m.mode = modeList
			m.notify("stopped waiting (the agent keeps working)", false)
		}
		return m, nil
	}
	return m.onViewKey(key)
}

func (m Model) onMenuKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.mode = modeList
	case "up", "k":
		m.menuCursor = max(0, m.menuCursor-1)
	case "down", "j":
		m.menuCursor = min(len(menuItems)-1, m.menuCursor+1)
	case "enter":
		return m, m.activate(menuItems[m.menuCursor].kind)
	default:
		for _, it := range menuItems {
			if key == it.key {
				return m, m.activate(it.kind)
			}
		}
	}
	return m, nil
}

func (m Model) onInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeList
	case tea.KeyEnter:
		return m, m.submit()
	default:
		m.input, m.ipos, _ = edit(m.input, m.ipos, msg)
	}
	return m, nil
}

func (m Model) onViewKey(key string) (tea.Model, tea.Cmd) {
	page := max(1, m.bodyHeight()-1)
	last := max(0, len(m.viewLines)-page)
	switch key {
	case "up", "k":
		m.viewTop--
	case "down", "j":
		m.viewTop++
	case "pgup":
		m.viewTop -= page
	case "pgdown":
		m.viewTop += page
	case "home", "g":
		m.viewTop = 0
	case "end", "G":
		m.viewTop = last
	default:
		m.mode = modeList
		return m, nil
	}
	m.viewTop = max(0, min(m.viewTop, last))
	return m, nil
}

// --- dibujo ---

// overlayBody dibuja el cuerpo de las pantallas que sustituyen a la lista.
func (m Model) overlayBody(cw, bh int, b box) []string {
	var lines []string
	switch m.mode {
	case modeMenu:
		lines = append([]string{""}, m.menuBox(cw, b)...)
	case modeBusy:
		text := m.spinner(m.busyElapsed) + " " + m.busyLabel + " " + ui.FormatElapsed(m.busyElapsed)
		lines = []string{"", "", centered(cw, text)}
	case modeView:
		head := "── " + m.viewTitle + " "
		fill := "─"
		if m.st.ASCII() {
			head, fill = "-- "+m.viewTitle+" ", "-"
		}
		lines = []string{m.st.Bold(head) + m.st.Paint(ui.RoleDim, strings.Repeat(fill, max(0, cw-ui.VisibleLen(head))))}
		page := max(0, bh-1)
		end := min(len(m.viewLines), m.viewTop+page)
		lines = append(lines, m.viewLines[min(m.viewTop, end):end]...)
	}
	for len(lines) < bh {
		lines = append(lines, "")
	}
	return lines[:bh]
}

func centered(cw int, s string) string {
	return strings.Repeat(" ", max(0, (cw-ui.VisibleLen(s))/2)) + s
}

// menuBox dibuja el menú de acciones centrado.
func (m Model) menuBox(cw int, b box) []string {
	t, _ := m.selectedTarget()
	title := t.MachineLabel() + " / " + ui.ShortName(t.Name, m.full)

	labelW := 0
	for _, it := range menuItems {
		labelW = max(labelW, ui.VisibleLen(m.itemLabel(it)))
	}
	iw := max(ui.VisibleLen(title), 2+labelW+2+1)
	iw = min(iw, max(cw-6, 4))
	title = ui.Truncate(title, iw, m.st.Glyphs().Ellipsis)

	pad := strings.Repeat(" ", max(0, (cw-(iw+4))/2))
	dim := func(s string) string { return m.st.Paint(ui.RoleDim, s) }
	out := []string{pad + dim(b.tl+" ") + m.st.Bold(title) + dim(" "+strings.Repeat(b.h, max(0, iw-ui.VisibleLen(title)))+b.tr)}
	for i, it := range menuItems {
		mark := "  "
		if i == m.menuCursor {
			mark = b.marker + " "
		}
		text := mark + ui.PadRight(m.itemLabel(it), labelW) + "  " + it.key
		text = ui.PadRight(ui.Truncate(text, iw, m.st.Glyphs().Ellipsis), iw)
		if i == m.menuCursor && m.st.Color() {
			text = m.st.Selected(text)
		}
		out = append(out, pad+dim(b.v)+" "+text+" "+dim(b.v))
	}
	out = append(out, pad+dim(b.bl+strings.Repeat(b.h, iw+2)+b.br))
	return out
}

func (m Model) itemLabel(it menuItem) string {
	if m.st.ASCII() {
		return it.ascii
	}
	return it.label
}

// infoLine es la segunda línea del pie: la entrada de texto, el último aviso o la ruta.
func (m Model) infoLine(cw int, path string) string {
	dim := func(s string) string { return m.st.Paint(ui.RoleDim, s) }
	switch {
	case m.mode == modeInput:
		b := boxFor(m.st)
		prompt := "message >"
		if m.inputKind == "rename" {
			prompt = "name >"
		}
		left := prompt + " " + withCursor(m.input, m.ipos, dim(b.cursor))
		right := ""
		switch {
		case m.inputKind == "rename" && m.input == "":
			right = dim("(empty clears the name)")
		case m.inputKind == "rename" && actions.ValidName(m.input) != nil:
			right = m.st.Paint(ui.RoleBlocked, actions.ErrInvalidName.Error())
		}
		return ui.FitRow(cw, left, right, m.st.Glyphs().Ellipsis)
	case m.notice != "":
		role := ui.RoleAccent
		if m.noticeErr {
			role = ui.RoleBlocked
		}
		return m.st.Paint(role, m.notice)
	}
	return dim(path)
}
