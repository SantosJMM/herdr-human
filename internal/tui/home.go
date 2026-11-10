package tui

import (
	"context"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// HomeResult es lo que dejó elegido la pantalla de inicio al cerrarse.
type HomeResult struct {
	// Chosen es true si se eligió un agente (con «Go to it»); Target es ese agente.
	Chosen bool
	Target inventory.Target
}

// Tab es una sección de la pantalla de inicio (agentes, atención, máquinas…).
// La pantalla de inicio las aloja y reparte las teclas; cada una dibuja solo su
// cuerpo.
type Tab interface {
	// Title es el nombre que lleva en la barra de pestañas.
	Title() string
	Init() tea.Cmd
	Update(msg tea.Msg) (Tab, tea.Cmd)
	// View devuelve las líneas del cuerpo para un hueco de w columnas por h filas.
	View(w, h int) []string
	// Captures indica que la pestaña está pidiendo texto (una búsqueda, un
	// nombre): entonces recibe todas las teclas, también q, esc, tab y los números.
	Captures() bool
}

// HomeOptions son los datos y el aspecto de la pantalla de inicio.
type HomeOptions struct {
	Style *ui.Style
	Tabs  []Tab
	// Embedded: el marco y el título los dibuja el popup de herdr (ver frame).
	Embedded bool
	// Trace anota un hito de arranque; nulo lo desactiva.
	Trace func(string)
}

const (
	homeDefaultWidth  = 80
	homeDefaultHeight = 24
	// Líneas que no son cuerpo: barra de pestañas, línea y pie; con marco propio
	// además el borde de arriba, la línea del pie y el borde de abajo.
	homeEmbeddedChrome = 3
	homeFramedChrome   = 6
)

// HomeModel es la pantalla de inicio: una barra de pestañas y el cuerpo de la
// pestaña activa.
type HomeModel struct {
	st     *ui.Style
	tabs   []Tab
	active int
	opts   HomeOptions
	result HomeResult

	width, height int
}

// Result devuelve lo elegido al cerrar la pantalla.
func (m HomeModel) Result() HomeResult { return m.result }

// NewHome crea la pantalla de inicio con la primera pestaña activa.
func NewHome(o HomeOptions) HomeModel {
	return HomeModel{st: o.Style, tabs: slices.Clone(o.Tabs), opts: o, width: homeDefaultWidth, height: homeDefaultHeight}
}

func (m HomeModel) note(label string) {
	if m.opts.Trace != nil {
		m.opts.Trace(label)
	}
}

func (m HomeModel) chrome() int {
	if m.opts.Embedded {
		return homeEmbeddedChrome
	}
	return homeFramedChrome
}

// size es el tamaño que se usa para dibujar: nunca menor que lo mínimo legible.
func (m HomeModel) size() (w, h int) {
	return max(m.width, minWidth), max(m.height, m.chrome()+1)
}

// bodySize es el hueco que se le da a la pestaña activa.
func (m HomeModel) bodySize() (w, h int) {
	w, h = m.size()
	return w - 4, h - m.chrome()
}

// Init arranca todas las pestañas: las que cargan datos empiezan a la vez.
func (m HomeModel) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.tabs))
	for _, t := range m.tabs {
		cmds = append(cmds, t.Init())
	}
	return tea.Batch(cmds...)
}

// Update reparte los mensajes. Las teclas de la propia pantalla (cambiar de
// pestaña, cerrar) solo valen si la pestaña activa no está pidiendo texto; el
// resto de mensajes llega a todas las pestañas, también a las ocultas, para que
// las que cargan datos no se queden sin su respuesta.
func (m HomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		bw, bh := m.bodySize()
		return m.broadcast(tea.WindowSizeMsg{Width: bw, Height: bh})
	}
	switch msg := msg.(type) {
	case LeaveMsg:
		// Una pestaña alojada terminó (eligió un agente o salió): se cierra todo.
		m.result = HomeResult{Chosen: msg.Chosen, Target: msg.Target}
		return m, tea.Quit
	case tea.MouseMsg:
		return m.onMouse(msg)
	}
	key, isKey := msg.(tea.KeyMsg)
	if !isKey {
		return m.broadcast(msg)
	}
	s := key.String()
	if s == "ctrl+c" {
		return m, tea.Quit
	}
	if len(m.tabs) == 0 {
		return m, nil
	}
	if !m.tabs[m.active].Captures() {
		switch s {
		case "q", "esc":
			return m, tea.Quit
		case "tab":
			m.active = (m.active + 1) % len(m.tabs)
			return m, nil
		case "shift+tab":
			m.active = (m.active + len(m.tabs) - 1) % len(m.tabs)
			return m, nil
		}
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(m.tabs) {
			m.active = n - 1
			return m, nil
		}
	}
	return m.forward(m.active, msg)
}

// top es la fila de pantalla donde empieza el cuerpo, y bar la de la barra de
// pestañas: con marco propio hay un borde encima.
func (m HomeModel) top() (bar, body int) {
	if m.opts.Embedded {
		return 0, 2
	}
	return 1, 3
}

// tabAt devuelve la pestaña que hay en la columna x de la barra, o -1.
func (m HomeModel) tabAt(x int) int {
	x -= 2 // el borde y el margen
	for i, t := range m.tabs {
		w := ui.VisibleLen(strconv.Itoa(i+1)+" "+t.Title()) + 2
		if x >= 0 && x < w {
			return i
		}
		x -= w + 1
	}
	return -1
}

// onMouse atiende el ratón: un clic en la barra cambia de pestaña, y lo que cae en
// el cuerpo llega a la pestaña activa con las coordenadas de su propio hueco.
func (m HomeModel) onMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	bar, body := m.top()
	_, bodyH := m.bodySize()
	switch {
	case msg.Y == bar:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if i := m.tabAt(msg.X); i >= 0 {
				m.active = i
			}
		}
		return m, nil
	case msg.Y >= body && msg.Y < body+bodyH && len(m.tabs) > 0:
		msg.Y -= body
		msg.X -= 2
		return m.forward(m.active, msg)
	}
	return m, nil
}

// forward entrega un mensaje a una pestaña.
func (m HomeModel) forward(i int, msg tea.Msg) (tea.Model, tea.Cmd) {
	m.tabs = slices.Clone(m.tabs)
	var cmd tea.Cmd
	m.tabs[i], cmd = m.tabs[i].Update(msg)
	return m, cmd
}

// broadcast entrega un mensaje a todas las pestañas.
func (m HomeModel) broadcast(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.tabs = slices.Clone(m.tabs)
	cmds := make([]tea.Cmd, 0, len(m.tabs))
	for i := range m.tabs {
		var cmd tea.Cmd
		m.tabs[i], cmd = m.tabs[i].Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// tabBar dibuja la barra de pestañas: la activa entre corchetes (así se ve
// también sin color) y pintada con el color de acento.
func (m HomeModel) tabBar() string {
	parts := make([]string, 0, len(m.tabs))
	for i, t := range m.tabs {
		label := strconv.Itoa(i+1) + " " + t.Title()
		if i == m.active {
			parts = append(parts, m.st.Paint(ui.RoleAccent, "["+label+"]"))
		} else {
			parts = append(parts, " "+label+" ")
		}
	}
	return strings.Join(parts, " ")
}

// View dibuja la pantalla con el tamaño de la ventana, ni una línea más ni menos.
func (m HomeModel) View() string {
	defer m.note("tui: home first view rendered")
	w, h := m.size()
	cw, bodyH := w-4, h-m.chrome()
	fr := newFrame(m.st, w, m.opts.Embedded)
	g := m.st.Glyphs()
	fit := func(s string) string { return ui.PadRight(ui.Truncate(s, cw, g.Ellipsis), cw) }
	line := func(inner string) string { return fr.side() + " " + fit(inner) + " " + fr.side() }

	out := fr.top(" hum home ")
	out = append(out, line(m.tabBar()), fr.rule())
	var body []string
	if len(m.tabs) > 0 {
		body = m.tabs[m.active].View(cw, bodyH)
	}
	for i := 0; i < bodyH; i++ {
		if i < len(body) {
			out = append(out, line(body[i]))
		} else {
			out = append(out, line(""))
		}
	}
	out = append(out, fr.footerRule()...)
	out = append(out, line(fr.dim(m.hint())))
	out = append(out, fr.bottom()...)
	return strings.Join(out, "\n")
}

// hint es la línea de ayuda del pie.
func (m HomeModel) hint() string {
	if len(m.tabs) > 0 && m.tabs[m.active].Captures() {
		return "ctrl+c close"
	}
	return "tab/shift+tab or 1-" + strconv.Itoa(len(m.tabs)) + " switch · esc close"
}

// RunHome abre la pantalla de inicio a pantalla completa.
func RunHome(ctx context.Context, in io.Reader, out io.Writer, o HomeOptions) (HomeResult, error) {
	p := tea.NewProgram(NewHome(o),
		tea.WithAltScreen(), tea.WithMouseCellMotion(),
		tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx))
	final, err := p.Run()
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, tea.ErrProgramKilled) {
			return HomeResult{}, nil
		}
		return HomeResult{}, err
	}
	return final.(HomeModel).Result(), nil
}

// textTab es una pestaña que solo muestra un texto fijo.
type textTab struct {
	title string
	lines []string
}

// NewTextTab crea una pestaña con un texto fijo, de una línea por renglón.
func NewTextTab(title, text string) Tab {
	return textTab{title: title, lines: strings.Split(text, "\n")}
}

func (t textTab) Title() string                 { return t.title }
func (t textTab) Init() tea.Cmd                 { return nil }
func (t textTab) Captures() bool                { return false }
func (t textTab) View(w, h int) []string        { return t.lines }
func (t textTab) Update(tea.Msg) (Tab, tea.Cmd) { return t, nil }
