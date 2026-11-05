package tui

import (
	"context"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/keybind"
	"github.com/SantosJMM/herdr-human/internal/palette"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// Fases de la captura de un atajo.
type capPhase int

const (
	capWaiting       capPhase = iota // esperando que se pulse la combinación
	capCaptured                      // hay una combinación, lista para guardar o con un problema
	capConfirmWarn                   // choca con un atajo por defecto: hay que confirmar a conciencia
	capConfirmRemove                 // se pidió quitar el atajo actual
	capSaving                        // guardando
)

// capture es el estado de la pantalla de asignar un atajo a la fila seleccionada.
type capture struct {
	active  bool
	entry   palette.Entry
	index   int    // posición de la entrada en m.entries
	current string // atajo que tiene ahora; vacío si ninguno
	prefix  bool   // si la combinación va después del prefijo de herdr
	base    string // combinación pulsada, sin el prefijo; vacía hasta que se pulsa
	check   KeyCheck
	phase   capPhase
	typing  bool   // se está escribiendo a mano
	text    picker // lo escrito a mano (solo query y qpos)
	problem string // fallo al guardar o al leer la tecla
}

// candidate es la tecla completa que se guardaría.
func (c capture) candidate() string {
	if c.base == "" {
		return ""
	}
	if c.prefix && !strings.HasPrefix(c.base, keybind.Prefix+"+") {
		return keybind.Prefix + "+" + c.base
	}
	return c.base
}

// keyDoneMsg es el resultado de guardar o quitar un atajo.
type keyDoneMsg struct {
	saved   KeySaved
	err     error
	removed bool
}

// reservadas son las teclas con que se maneja la propia pantalla: no se pueden
// capturar como atajo (se pueden escribir a mano con ctrl+t).
func reservedKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyEnter, tea.KeyTab, tea.KeyDelete, tea.KeyCtrlT, tea.KeyCtrlC:
		return !msg.Alt
	}
	return false
}

// renames pasa los nombres de Bubble Tea a los de herdr.
var renames = map[string]string{"pgup": "pageup", "pgdown": "pagedown"}

// chordFromKey convierte una pulsación en la tecla tal como se escribe en el
// config.toml de herdr. Si no se puede (o es ambigua en las terminales), devuelve
// el motivo.
func chordFromKey(msg tea.KeyMsg) (chord, why string) {
	var mods []string
	if msg.Alt {
		mods = append(mods, "alt")
	}
	var base string
	switch msg.Type {
	case tea.KeyRunes:
		if len(msg.Runes) != 1 {
			return "", "press one key at a time"
		}
		r := msg.Runes[0]
		base = keybind.RuneName(r)
		if unicode.IsUpper(r) {
			mods = append(mods, "shift")
		}
	case tea.KeySpace:
		base = "space"
	case tea.KeyCtrlH:
		return "", "ctrl+h is Backspace in many terminals, so it cannot be told apart"
	default:
		parts := strings.Split(strings.TrimPrefix(msg.String(), "alt+"), "+")
		base = parts[len(parts)-1]
		mods = append(mods, parts[:len(parts)-1]...)
		if r, ok := renames[base]; ok {
			base = r
		}
		if base == "" || base == "unknown" {
			return "", "that key cannot be read here"
		}
	}
	norm, err := keybind.Normalize(strings.Join(append(mods, base), "+"))
	if err != nil {
		return "", err.Error()
	}
	return norm, ""
}

// startCapture abre la pantalla de atajo de la fila seleccionada.
func (m PaletteModel) startCapture() PaletteModel {
	m.notice = ""
	i, ok := m.list.current()
	if !ok || m.opts.Keys == nil {
		return m
	}
	e := m.entries[i]
	current, bindable := m.opts.Keys.Bindable(e)
	if !bindable {
		m.notice = "This entry cannot have a shortcut: it asks questions or needs a computed value."
		return m
	}
	m.mode = palCapture
	m.cap = capture{active: true, entry: e, index: i, current: current}
	return m
}

// onCaptureKey maneja las teclas de la pantalla de atajo.
func (m PaletteModel) onCaptureKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := &m.cap
	if c.phase == capSaving {
		return m, nil // se está escribiendo el archivo: nada que hacer hasta que termine
	}
	if c.typing {
		return m.onTypingKey(msg)
	}

	switch {
	case msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC:
		return m.endCapture(), nil

	case msg.Type == tea.KeyTab && !msg.Alt:
		// El prefijo se activa o se quita; si ya hay una combinación, se vuelve a validar.
		c.prefix = !c.prefix
		if c.base != "" {
			c.base = strings.TrimPrefix(c.base, keybind.Prefix+"+")
			m.recheck()
		}
		return m, nil

	case msg.Type == tea.KeyCtrlT:
		c.typing = true
		c.text = picker{query: c.candidate(), qpos: len([]rune(c.candidate()))}
		c.problem = ""
		return m, nil

	case msg.Type == tea.KeyDelete && !msg.Alt:
		if c.current == "" {
			c.problem = "it has no shortcut to remove"
			return m, nil
		}
		c.phase, c.problem = capConfirmRemove, ""
		return m, nil

	case msg.Type == tea.KeyEnter && !msg.Alt:
		return m.confirm()
	}

	if reservedKey(msg) {
		return m, nil
	}
	chord, why := chordFromKey(msg)
	if why != "" {
		c.base, c.problem, c.phase = "", why, capWaiting
		return m, nil
	}
	c.base, c.problem = chord, ""
	m.recheck()
	return m, nil
}

// onTypingKey maneja las teclas mientras se escribe la combinación a mano.
func (m PaletteModel) onTypingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := &m.cap
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		c.typing = false
	case tea.KeyEnter:
		typed := strings.TrimSpace(c.text.query)
		c.typing = false
		if typed == "" {
			return m, nil
		}
		// Lo escrito manda: ya lleva el prefijo si lo necesita.
		c.base, c.prefix = typed, false
		m.recheck()
	default:
		c.text.edit(msg)
	}
	return m, nil
}

// recheck valida la combinación actual y fija la fase.
func (m *PaletteModel) recheck() {
	c := &m.cap
	c.check = m.opts.Keys.Check(c.entry, c.candidate())
	if c.check.Key != "" {
		c.base = c.check.Key
		c.prefix = strings.HasPrefix(c.base, keybind.Prefix+"+")
	}
	c.phase = capCaptured
}

// confirm actúa sobre Enter según la fase.
func (m PaletteModel) confirm() (tea.Model, tea.Cmd) {
	c := &m.cap
	switch c.phase {
	case capConfirmRemove:
		c.phase = capSaving
		return m, m.keyCmd(true)
	case capConfirmWarn:
		c.phase = capSaving
		return m, m.keyCmd(false)
	case capCaptured:
		if c.check.Problem != "" {
			return m, nil
		}
		if c.check.Warning != "" {
			c.phase = capConfirmWarn // primero se avisa; el segundo Enter lo guarda
			return m, nil
		}
		c.phase = capSaving
		return m, m.keyCmd(false)
	}
	return m, nil
}

// keyCmd guarda (o quita) el atajo en segundo plano.
func (m PaletteModel) keyCmd(remove bool) tea.Cmd {
	keys, ctx, e, key := m.opts.Keys, m.opts.Ctx, m.cap.entry, m.cap.check.Key
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}
		if remove {
			saved, err := keys.Remove(ctx, e)
			return keyDoneMsg{saved: saved, err: err, removed: true}
		}
		saved, err := keys.Save(ctx, e, key)
		return keyDoneMsg{saved: saved, err: err}
	}
}

// onKeyDone recoge el resultado: si salió bien, vuelve a la lista con el atajo
// nuevo a la vista; si no, se queda en la pantalla para corregirlo.
func (m PaletteModel) onKeyDone(msg keyDoneMsg) PaletteModel {
	c := &m.cap
	if msg.err != nil {
		c.problem = msg.err.Error()
		c.phase = capCaptured
		if msg.removed {
			c.phase = capConfirmRemove
		}
		return m
	}
	e := c.entry
	if c.index < len(m.entries) && m.entries[c.index].ID == e.ID {
		m.entries[c.index].Chord = msg.saved.Key
		m.setEntries(m.entries, e.ID)
	}
	switch {
	case msg.removed:
		m.notice = "Shortcut removed from " + e.Title + "."
	default:
		m.notice = "Shortcut " + msg.saved.Key + " saved for " + e.Title + "."
	}
	if msg.saved.Note != "" {
		m.notice += " " + msg.saved.Note
	}
	return m.endCapture()
}

// endCapture vuelve a la lista.
func (m PaletteModel) endCapture() PaletteModel {
	m.cap = capture{}
	m.mode = palList
	return m
}

// captureLines son las líneas del cuerpo mientras se asigna un atajo.
func (m PaletteModel) captureLines(plain func(string) rowRender) []rowRender {
	c := m.cap
	dim := func(s string) string { return m.st.Paint(ui.RoleDim, s) }
	accent := func(s string) string { return m.st.Bold(m.st.Paint(ui.RoleAccent, s)) }

	now := "none"
	if c.current != "" {
		now = c.current
	}
	lines := []rowRender{
		plain(accent(c.entry.Title) + "  " + dim("("+c.entry.Source+")")),
		plain(dim("now: ") + now),
		plain(""),
	}

	switch {
	case c.phase == capSaving:
		lines = append(lines, plain(dim("saving to config.toml and reloading herdr…")))
	case c.phase == capConfirmRemove:
		lines = append(lines, plain("Remove the shortcut "+accent(c.current)+"?  "+dim("enter removes it, esc cancels")))
	case c.typing:
		lines = append(lines, plain(dim("type it: ")+withCursor(c.text.query, c.text.qpos, dim(boxFor(m.st).cursor))))
	case c.base == "":
		prefix := "off"
		if c.prefix {
			prefix = "on"
		}
		lines = append(lines, plain("Press the shortcut you want…  "+dim("(prefix: "+prefix+")")))
	default:
		lines = append(lines, plain(dim("new: ")+accent(c.candidate())))
	}

	if c.check.Problem != "" && c.phase == capCaptured {
		lines = append(lines, plain(m.st.Paint(ui.RoleBlocked, "✗ "+c.check.Problem)))
	}
	if c.check.Warning != "" && (c.phase == capCaptured || c.phase == capConfirmWarn) {
		lines = append(lines, plain(m.st.Paint(ui.RoleWorking, "⚠ "+c.check.Warning)))
		if c.phase == capConfirmWarn {
			// En su propia línea: en una ventana estrecha un mensaje largo se corta, y
			// esta es la instrucción que no puede perderse.
			lines = append(lines, plain(accent("enter again to bind it anyway")+dim("  ·  esc cancels")))
		}
	}
	if c.problem != "" {
		lines = append(lines, plain(m.st.Paint(ui.RoleBlocked, "✗ "+c.problem)))
		if c.phase == capWaiting {
			lines = append(lines, plain(dim("ctrl+t lets you type it by hand")))
		}
	}
	return lines
}

// captureKeysText es la línea de teclas de la pantalla de atajo.
func (m PaletteModel) captureKeysText(sep string) string {
	c := m.cap
	var keys []string
	switch {
	case c.phase == capSaving:
		keys = []string{"please wait"}
	case c.phase == capConfirmRemove, c.phase == capConfirmWarn:
		keys = []string{"enter confirm", "esc cancel"}
	case c.typing:
		keys = []string{"enter use it", "esc back"}
	case c.phase == capCaptured && c.check.Problem == "":
		keys = []string{"enter save", "tab prefix", "ctrl+t type it", "esc cancel"}
	default:
		keys = []string{"press the keys", "tab prefix", "ctrl+t type it"}
		if c.current != "" {
			keys = append(keys, "delete remove")
		}
		keys = append(keys, "esc cancel")
	}
	return strings.Join(keys, sep)
}
