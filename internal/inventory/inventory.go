// Paquete inventory construye una vista única de los agentes de herdr en el
// servidor local y en las máquinas guardadas.
package inventory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/SantosJMM/herdr-human/internal/herdr"
)

// DefaultTimeout limita cada consulta a un servidor, para que una máquina
// lenta no bloquee el resto.
const DefaultTimeout = 8 * time.Second

// Target es un agente direccionable, local o remoto.
type Target struct {
	// Machine es la etiqueta de la máquina; vacía significa el servidor local.
	Machine     string `json:"machine,omitempty"`
	Workspace   string `json:"workspace"`
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	PaneID      string `json:"pane_id"`
	Agent       string `json:"agent"`
	// Name es el nombre visible: el asignado por el usuario o, si no hay,
	// el título del terminal.
	Name    string `json:"name"`
	Status  string `json:"status"`
	Cwd     string `json:"cwd,omitempty"`
	Focused bool   `json:"focused"`
}

// LocalLabel es cómo se muestra el servidor local.
const LocalLabel = "local"

// MachineLabel devuelve la etiqueta de la máquina, o "local".
func (t Target) MachineLabel() string {
	if t.Machine == "" {
		return LocalLabel
	}
	return t.Machine
}

// Ref identifica el agente sin ambigüedad entre máquinas: "maquina/pane".
func (t Target) Ref() string {
	return t.MachineLabel() + "/" + t.PaneID
}

// Warning registra una máquina que no pudo consultarse.
type Warning struct {
	Machine string
	Err     error
}

func (w Warning) Error() string {
	return fmt.Sprintf("machine %q: %v", w.Machine, w.Err)
}

// Unwrap permite usar errors.Is/As sobre la causa.
func (w Warning) Unwrap() error { return w.Err }

// Timing registra cuánto tardó consultar un servidor y cómo terminó.
type Timing struct {
	// Machine es la etiqueta de la máquina; vacía significa el servidor local.
	Machine string
	Elapsed time.Duration
	Err     error
	// Cached dice que la respuesta salió de la caché y no de una consulta; Age es
	// su antigüedad.
	Cached bool
	Age    time.Duration
}

// Inventory es el resultado de Load.
type Inventory struct {
	Targets  []Target
	Warnings []Warning
	// Timings tiene una entrada por servidor consultado, en el mismo orden que
	// los agentes: primero el local y luego las máquinas.
	Timings []Timing
}

// Options controla qué servidores se consultan.
type Options struct {
	// Machines restringe la consulta a estas etiquetas (sin distinguir
	// mayúsculas). Nombrar una máquina deshabilitada la incluye. Si está
	// vacío se consultan todas las máquinas habilitadas.
	Machines []string
	// LocalOnly consulta solo el servidor local.
	LocalOnly bool
	// Scope es una pista, normalmente el prefijo de una consulta del tipo
	// "odoous/codex": si coincide con "local" o con una máquina guardada se
	// consulta solo ese servidor; si no coincide con nada se ignora, porque
	// puede ser parte de un nombre. Evita consultar máquinas que la persona
	// no mencionó.
	Scope string
	// Timeout acota cada servidor; cero usa DefaultTimeout.
	Timeout time.Duration
}

// Update es la respuesta de un servidor: sus agentes o el motivo del fallo.
type Update struct {
	// Machine es la etiqueta de la máquina; vacía significa el servidor local.
	Machine string
	Targets []Target
	Err     error
	Elapsed time.Duration
	// Cached dice que Targets salió de la caché; Age es su antigüedad.
	Cached bool
	Age    time.Duration
}

// Stream entrega los servidores según van respondiendo.
type Stream struct {
	// Machines son los servidores que se consultan, en orden: primero el local
	// ("") y luego las máquinas en el orden de `herdr machine list`.
	Machines []string
	// Updates recibe una respuesta por servidor, en el orden en que llegan, y se
	// cierra al llegar la última. Tiene búfer suficiente para que quien lo
	// consume pueda abandonarlo sin bloquear las consultas en curso.
	Updates <-chan Update
}

// Start empieza a consultar el servidor local y las máquinas elegidas en
// paralelo y devuelve enseguida. Un fallo, también el local, llega como una
// Update con Err; solo falla Start si no se puede decidir qué máquinas consultar.
func Start(ctx context.Context, r herdr.Runner, opts Options) (Stream, error) {
	return StartCached(ctx, r, opts, nil)
}

// StartCached es Start con una caché opcional (nula = siempre en vivo): las
// máquinas remotas con una entrada vigente llegan al instante, marcadas como
// Cached, y no se consultan; las demás se consultan y, si responden, se guardan.
// El servidor local siempre es en vivo.
func StartCached(ctx context.Context, r herdr.Runner, opts Options, cache *Cache) (Stream, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	if strings.EqualFold(opts.Scope, LocalLabel) {
		opts.LocalOnly = true
	}

	machines := []string{""}
	if !opts.LocalOnly {
		picked, err := selectMachines(ctx, r, opts.Machines, opts.Scope)
		if err != nil {
			return Stream{}, err
		}
		for _, m := range picked {
			machines = append(machines, m.Label)
		}
	}

	updates := make(chan Update, len(machines))
	var wg sync.WaitGroup
	for _, machine := range machines {
		if targets, age, ok := cache.get(machine); ok {
			updates <- Update{Machine: machine, Targets: targets, Cached: true, Age: age}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			start := time.Now()
			targets, err := loadSource(sctx, r, machine)
			if err == nil {
				cache.put(machine, targets) // un fallo nunca se cachea
			}
			updates <- Update{Machine: machine, Targets: targets, Err: err, Elapsed: time.Since(start)}
		}()
	}
	go func() {
		wg.Wait()
		_ = cache.save() // de mejor esfuerzo: sin caché, la siguiente consulta es en vivo
		close(updates)
	}()
	return Stream{Machines: machines, Updates: updates}, nil
}

// LoadMachine vuelve a consultar un solo servidor ("" es el local) con su propio
// plazo; sirve para refrescar una máquina sin tocar las demás.
func LoadMachine(ctx context.Context, r herdr.Runner, machine string, timeout time.Duration) ([]Target, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return loadSource(ctx, r, machine)
}

// Load consulta el servidor local y las máquinas elegidas en paralelo y espera
// a todas. Un fallo local es fatal; el de una máquina remota se devuelve como
// Warning. El orden es determinista: primero el local y luego las máquinas en
// el orden de `herdr machine list`.
func Load(ctx context.Context, r herdr.Runner, opts Options) (Inventory, error) {
	return LoadCached(ctx, r, opts, nil)
}

// LoadCached es Load con una caché opcional; ver StartCached.
func LoadCached(ctx context.Context, r herdr.Runner, opts Options, cache *Cache) (Inventory, error) {
	stream, err := StartCached(ctx, r, opts, cache)
	if err != nil {
		return Inventory{}, err
	}
	byMachine := make(map[string]Update, len(stream.Machines))
	for u := range stream.Updates {
		byMachine[u.Machine] = u
	}
	return Assemble(stream.Machines, byMachine)
}

// Assemble arma el inventario a partir de las respuestas recibidas, en el orden
// de machines. Un fallo local es fatal; el de una máquina remota es un Warning.
func Assemble(machines []string, updates map[string]Update) (Inventory, error) {
	var inv Inventory
	for _, machine := range machines {
		u := updates[machine]
		inv.Timings = append(inv.Timings, Timing{Machine: machine, Elapsed: u.Elapsed, Err: u.Err, Cached: u.Cached, Age: u.Age})
		if u.Err != nil {
			if machine == "" {
				return Inventory{}, u.Err
			}
			inv.Warnings = append(inv.Warnings, Warning{Machine: machine, Err: u.Err})
			continue
		}
		inv.Targets = append(inv.Targets, u.Targets...)
	}
	return inv, nil
}

// selectMachines devuelve las máquinas a consultar según el filtro.
func selectMachines(ctx context.Context, r herdr.Runner, wanted []string, scope string) ([]herdr.Machine, error) {
	all, err := herdr.ListMachines(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}

	if len(wanted) == 0 && scope != "" {
		for _, m := range all {
			if strings.EqualFold(m.Label, scope) {
				return []herdr.Machine{m}, nil
			}
		}
	}

	if len(wanted) == 0 {
		var enabled []herdr.Machine
		for _, m := range all {
			if m.Enabled {
				enabled = append(enabled, m)
			}
		}
		return enabled, nil
	}

	var picked []herdr.Machine
	for _, label := range wanted {
		found := false
		for _, m := range all {
			if strings.EqualFold(m.Label, label) {
				picked = append(picked, m)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown machine %q", label)
		}
	}
	return picked, nil
}

type rawAgent struct {
	Agent       string `json:"agent"`
	Status      string `json:"agent_status"`
	Cwd         string `json:"cwd"`
	Focused     bool   `json:"focused"`
	PaneID      string `json:"pane_id"`
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Title       string `json:"terminal_title_stripped"`
}

func (a rawAgent) displayName() string {
	for _, candidate := range []string{a.Name, a.Label, a.Title, a.Agent} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

type rawWorkspace struct {
	ID    string `json:"workspace_id"`
	Label string `json:"label"`
}

// loadSource consulta un servidor. Cada llamada remota cuesta ~1,5 s por SSH,
// así que se prefiere `api snapshot`, que trae agentes y workspaces juntos,
// a dos listas seguidas. Solo si herdr contesta que no lo soporta (un error
// de la API) se usa el camino de las listas; un fallo de transporte NO se
// reintenta, para no duplicar la espera de una máquina caída.
func loadSource(ctx context.Context, r herdr.Runner, machine string) ([]Target, error) {
	var snap struct {
		Snapshot struct {
			Agents     []rawAgent     `json:"agents"`
			Workspaces []rawWorkspace `json:"workspaces"`
		} `json:"snapshot"`
	}
	err := herdr.Call(ctx, r, machine, []string{"api", "snapshot"}, &snap)
	if err == nil {
		return buildTargets(machine, snap.Snapshot.Agents, labelsOf(snap.Snapshot.Workspaces)), nil
	}
	if _, ok := errors.AsType[*herdr.APIError](err); !ok {
		return nil, err
	}
	return loadViaLists(ctx, r, machine)
}

// loadViaLists es el camino para servidores sin `api snapshot`. Las dos
// consultas son independientes y se hacen a la vez.
func loadViaLists(ctx context.Context, r herdr.Runner, machine string) ([]Target, error) {
	var (
		wg     sync.WaitGroup
		labels map[string]string
	)
	wg.Go(func() {
		// Las etiquetas de workspace son solo cosmética: si fallan, se usa el ID.
		labels = workspaceLabels(ctx, r, machine)
	})

	var agents struct {
		Agents []rawAgent `json:"agents"`
	}
	err := herdr.Call(ctx, r, machine, []string{"agent", "list"}, &agents)
	wg.Wait()
	if err != nil {
		return nil, err
	}
	return buildTargets(machine, agents.Agents, labels), nil
}

func labelsOf(workspaces []rawWorkspace) map[string]string {
	labels := make(map[string]string, len(workspaces))
	for _, w := range workspaces {
		labels[w.ID] = w.Label
	}
	return labels
}

func buildTargets(machine string, agents []rawAgent, labels map[string]string) []Target {
	targets := make([]Target, 0, len(agents))
	for _, a := range agents {
		workspace := labels[a.WorkspaceID]
		if workspace == "" {
			workspace = a.WorkspaceID
		}
		targets = append(targets, Target{
			Machine:     machine,
			Workspace:   workspace,
			WorkspaceID: a.WorkspaceID,
			TabID:       a.TabID,
			PaneID:      a.PaneID,
			Agent:       a.Agent,
			Name:        a.displayName(),
			Status:      a.Status,
			Cwd:         a.Cwd,
			Focused:     a.Focused,
		})
	}
	return targets
}

func workspaceLabels(ctx context.Context, r herdr.Runner, machine string) map[string]string {
	var out struct {
		Workspaces []rawWorkspace `json:"workspaces"`
	}
	if err := herdr.Call(ctx, r, machine, []string{"workspace", "list"}, &out); err != nil {
		return nil
	}
	return labelsOf(out.Workspaces)
}
