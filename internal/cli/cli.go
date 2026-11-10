// Paquete cli implementa la interfaz de línea de comandos de `hum`.
// Recibe sus dependencias por Env para poder probarse sin herdr ni terminal.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/resolve"
	"github.com/SantosJMM/herdr-human/internal/trace"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// Códigos de salida.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
	// exitBlocked: el agente quedó esperando a la persona (wait).
	exitBlocked = 3
	// exitTimeout: venció el plazo de la espera (wait).
	exitTimeout = 4
)

// exitError lleva un código de salida propio; su mensaje, si lo hay, va a stderr.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

// Env agrupa las dependencias externas del CLI.
type Env struct {
	Runner herdr.Runner
	// Stdin alimenta los comandos interactivos; si es nil se trata como vacío.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Terminal indica que Stdout es una terminal interactiva (permite redibujar
	// en el sitio). Si es false se asume una tubería o un archivo.
	Terminal bool
	// StdinTerminal indica que Stdin es una terminal interactiva; solo entonces
	// se pregunta cuál de varios agentes se quiso decir.
	StdinTerminal bool
	// Sleep espera una duración o hasta que el contexto termine; si es nil se
	// usa un temporizador real. Existe para poder probar --watch sin esperar.
	Sleep func(ctx context.Context, d time.Duration) error
	// Getenv lee variables de entorno (NO_COLOR, HUM_ASCII, COLUMNS…); si es
	// nil se trata como un entorno vacío.
	Getenv func(string) string
	// GOOS es el sistema operativo; solo afecta a la detección de color de 24 bits.
	GOOS string
	// Width es el ancho de la terminal en celdas; 0 si se desconoce.
	Width int
	// Height son las filas de la terminal; 0 si se desconoce (entonces ls no
	// redibuja en su sitio).
	Height int
	// Theme es el tema de colores; el valor cero usa la paleta ANSI de la terminal.
	Theme ui.Theme
	// Copy escribe en el portapapeles; nulo si la plataforma no lo permite.
	Copy func(text string) error
	// Getwd devuelve la carpeta actual; nulo si se desconoce.
	Getwd func() (string, error)
	// CacheDir es la carpeta donde se guarda la caché del inventario; vacía la
	// desactiva. Now es el reloj de la caché (nulo = time.Now).
	CacheDir string
	Now      func() time.Time
	// ConfigDir es la carpeta donde se guardan los alias; vacía los desactiva.
	ConfigDir string
	// Detach lanza este mismo programa con esos argumentos como un proceso
	// independiente que sigue vivo cuando este termina; nulo si la plataforma no
	// lo permite. La paleta lo usa para ejecutar lo elegido tras cerrar su popup.
	Detach func(args []string) error
	// Trace anota los tiempos de arranque cuando la traza está encendida (ver
	// `hum trace`); nulo la apaga, y todas sus llamadas son seguras con nulo.
	Trace *trace.Tracer
	// CheckConfig ejecuta `herdr config check` sobre el archivo dado y devuelve lo
	// que imprime; nulo desactiva el cambio de atajos, que no se aplica sin validar.
	CheckConfig func(ctx context.Context, path string) (string, error)
	// Version es lo que imprime `hum version` (p. ej. «0.1.2 (windows/amd64)»).
	Version string
}

const usageText = `hum - friendly commands for herdr

Usage:
  hum ls [--json] [--verbose] [--local] [--machine NAME] [--server-timeout 3s]
         [--fresh]
         [--color auto|always|never] [--full-names]
                                                list agents on all machines
  hum id <who> [--json] [--cmd]                  print the pane id of an agent
  hum say <who> <message...> [--wait] [--timeout MS]
                                                send a message to an agent
  hum ask <who> <message...> [--timeout MS] [--lines N]
                                                send a message, wait, print the reply
                                                (exit 3 if blocked, 4 on timeout)
  hum attention [--json] [--watch [--interval 5s] [--notify]] [--color M] [--full-names]
                                                who needs you: blocked/done first,
                                                then working; idle are only counted
  hum pick [--local] [--machine NAME] [--full-names] [--embedded]
                                                interactive selector: browse, search,
                                                filter and act on agents (needs a
                                                terminal). "hum" alone does the same
                                                in a terminal and prints this help
                                                otherwise
  hum palette [--embedded]                       command palette: fuzzy-search and run every
                                                herdr command and every action of every
                                                plugin (needs a terminal; bind the
                                                "palette" action of this plugin to a key)
  hum keys ls [--all] | set <entry> <key> [--force] | rm <entry>
                                                shortcuts of the palette entries in herdr's
                                                config.toml (the palette does the same with
                                                ctrl+k). Every change is checked with
                                                "herdr config check" on a copy first, backed
                                                up and applied atomically
  hum trace on|off|show|clear                    timing trace of the popups (to measure how long
                                                they take to draw their first frame); "show"
                                                prints the last lines of the log
  hum new <kind> [folder] [--name NAME] [--machine NAME] [--no-focus] [--timeout MS]
                                                open a tab, start an agent in it and
                                                name it; prints machine/pane. Without
                                                arguments, asks in a terminal
  hum completion bash|zsh|powershell             print the shell completion script
                                                (agents come from the cache, never the network)
  hum look <who> [--lines N]                     read an agent's recent output
  hum name <who> <new-name> | --clear            rename an agent and its pane
  hum rename --tab|--workspace <who> <label...>  rename the tab or the workspace of an agent
  hum close --tab|--workspace <who> [--yes]      close the tab or workspace of an agent
                                                (stops everything in it; asks first,
                                                --yes is required without a terminal)
  hum worktree ls [<who>]                        list the Git worktrees (of the agent's
                                                workspace, or of the current folder)
  hum worktree new <branch> [<who>] [--base REF] [--label TEXT] [--no-focus]
                                                create a worktree on a new branch
  hum worktree rm <who> [--force] [--yes]        delete the worktree checkout (asks first)
                                                hum never passes --trust-repository
  hum alias set <name> <who> | rm <name> | ls    short names for agents: "hum alias set docs
                                                odoous/odoo-dev", then "hum say docs ..."
                                                (an alias wins over any other match)
  hum go <who>                                   focus an agent
  hum wait <who> [--until STATUS]... [--timeout MS] [--notify]
                                                wait for an agent; exit 3 if it is
                                                blocked, 4 on timeout
  hum machines [--json]                          list saved machines (enabled/disabled)
  hum machines enable|disable|toggle <name>      change a saved machine
  hum machines menu                              interactive enable/disable menu
  hum version                                    print the version (also -v, --version)
  hum help

<who> is an agent name, "machine/name", a partial or fuzzy name, an agent
type, or a raw pane id such as w1:p2. Use "local/" to force this machine.
Flags may appear anywhere; use "--" so words starting with "-" in a message are not read as flags.
Shared flags: --fresh (ls and the commands taking <who>) ignores the 10 s cache of remote machines. --machine NAME (repeatable) limits which machines are queried; when looking up <who> it behaves like
the "machine/" prefix ("hum ask odoous/codex ..." == "hum ask codex ... --machine odoous"; NAME "local" is this
machine). For ls, this machine is always included. --local queries only this machine.
Colors follow the herdr theme in config.toml (catppuccin, catppuccin-latte; other themes use the terminal's
ANSI colors). NO_COLOR or a pipe turns them off, --color always|never forces it, HUM_ASCII=1 uses ASCII symbols.
Long titles such as "Task | project | 5h 8% left" show only their first segment; --full-names shows them whole.
`

// usageError marca un uso incorrecto del CLI (código de salida 2).
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

type app struct {
	env Env
	in  *bufio.Reader // entrada compartida por las preguntas interactivas
}

// Run ejecuta el CLI con los argumentos dados (sin el nombre del programa) y
// devuelve el código de salida.
func Run(ctx context.Context, env Env, args []string) int {
	a := &app{env: env}
	err := a.dispatch(ctx, args)

	var ue *usageError
	var xe *exitError
	var he *hintError
	switch {
	case err == nil:
		return exitOK
	case errors.As(err, &xe):
		fmt.Fprintf(env.Stderr, "hum: %v\n", xe)
		return xe.code
	case errors.As(err, &ue):
		fmt.Fprintf(env.Stderr, "hum: %s\n\n%s", ue.msg, usageText)
		return exitUsage
	default:
		fmt.Fprintf(env.Stderr, "hum: %v\n", err)
		if errors.As(err, &he) {
			fmt.Fprintf(env.Stderr, "hint: %s\n", he.hint)
		}
		return exitFailure
	}
}

func (a *app) dispatch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		// En una terminal, "hum" a secas abre el selector; con una tubería (o sin
		// terminal en la entrada) no hay nadie que elija, así que muestra la ayuda.
		if a.interactive() {
			return a.selector(ctx, nil)
		}
		fmt.Fprint(a.env.Stdout, usageText)
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Fprint(a.env.Stdout, usageText)
		return nil
	case "version", "-v", "--version":
		if len(rest) > 0 {
			return usagef("version takes no arguments")
		}
		v := a.env.Version
		if v == "" {
			v = "unknown"
		}
		fmt.Fprintf(a.env.Stdout, "hum %s\n", v)
		return nil
	case "ls":
		return a.ls(ctx, rest)
	case "id":
		return a.id(ctx, rest)
	case "say":
		return a.say(ctx, rest)
	case "look":
		return a.look(ctx, rest)
	case "name":
		return a.name(ctx, rest)
	case "rename":
		return a.rename(ctx, rest)
	case "close":
		return a.closeContainer(ctx, rest)
	case "worktree":
		return a.worktree(ctx, rest)
	case "alias":
		return a.aliasCmd(rest)
	case "go":
		return a.focus(ctx, rest)
	case "wait":
		return a.wait(ctx, rest)
	case "ask":
		return a.ask(ctx, rest)
	case "attention":
		return a.attention(ctx, rest)
	case "machines":
		return a.machines(ctx, rest)
	case "pick":
		return a.selector(ctx, rest)
	case "new":
		return a.newAgent(ctx, rest)
	case "palette":
		return a.palette(ctx, rest)
	case "popup":
		return a.popup(ctx, rest) // interno: lo usa el manifest del plugin
	case "trace":
		return a.traceCmd(rest)
	case "keys":
		return a.keysCmd(ctx, rest)
	case "completion":
		return a.completion(ctx, rest)
	case "__complete":
		return a.complete(ctx, rest) // interno: lo llaman los scripts de autocompletado
	default:
		return usagef("unknown command %q", cmd)
	}
}

// commonFlags son las opciones compartidas por los comandos.
type commonFlags struct {
	json          bool
	local         bool
	machines      machineList
	serverTimeout time.Duration
	// fresh ignora la caché (--fresh); cached la activa para este comando.
	fresh  bool
	cached bool
}

type machineList []string

func (m *machineList) String() string { return strings.Join(*m, ",") }

func (m *machineList) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*m = append(*m, part)
		}
	}
	return nil
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func (c *commonFlags) register(fs *flag.FlagSet, withJSON bool) {
	fs.BoolVar(&c.local, "local", false, "query only this machine")
	fs.Var(&c.machines, "machine", "limit the machines queried (repeatable)")
	fs.DurationVar(&c.serverTimeout, "server-timeout", 0, "max time to wait for each server, e.g. 3s (default 8s)")
	if withJSON {
		fs.BoolVar(&c.json, "json", false, "print JSON")
	}
}

// registerFresh añade --fresh a los comandos que usan la caché del inventario.
func (c *commonFlags) registerFresh(fs *flag.FlagSet) {
	fs.BoolVar(&c.fresh, "fresh", false, "ignore the cached answers of remote machines (and refresh them)")
}

// parse admite opciones mezcladas con argumentos posicionales. Lo que sigue a
// un "--" se toma literalmente como posicional.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var literal []string
	for i, arg := range args {
		if arg == "--" {
			literal = args[i+1:]
			args = args[:i]
			break
		}
	}

	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, usagef("%v", err)
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
	return append(positional, literal...), nil
}

// load consulta el inventario aplicando las opciones compartidas. Las máquinas
// inalcanzables se avisan por stderr sin interrumpir el comando.
func (a *app) load(ctx context.Context, cf commonFlags, scope string) (inventory.Inventory, error) {
	inv, err := a.loadQuiet(ctx, cf, scope)
	if err != nil {
		return inv, err
	}
	a.warn(inv)
	return inv, nil
}

// loadQuiet es load sin imprimir los avisos de las máquinas inalcanzables.
func (a *app) loadQuiet(ctx context.Context, cf commonFlags, scope string) (inventory.Inventory, error) {
	return inventory.LoadCached(ctx, a.env.Runner, a.options(cf, scope), a.cacheFor(cf))
}

// options traduce las opciones compartidas a las del inventario.
func (a *app) options(cf commonFlags, scope string) inventory.Options {
	return inventory.Options{
		Machines:  cf.machines,
		LocalOnly: cf.local,
		Scope:     scope,
		Timeout:   cf.serverTimeout,
	}
}

// cacheFile es el nombre del archivo de caché dentro de Env.CacheDir.
const cacheFile = "inventory.json"

// cacheFor devuelve la caché que corresponde a estas opciones: nula (siempre en
// vivo) si el comando no la usa o no hay directorio donde guardarla.
func (a *app) cacheFor(cf commonFlags) *inventory.Cache {
	if !cf.cached || a.env.CacheDir == "" {
		return nil
	}
	return &inventory.Cache{Path: filepath.Join(a.env.CacheDir, cacheFile), Now: a.env.Now, Bypass: cf.fresh}
}

// invalidateCache borra la caché tras un cambio (crear o renombrar un agente),
// para que lo siguiente no vea datos anteriores a él. Un fallo no es un error.
func (a *app) invalidateCache() {
	if a.env.CacheDir != "" {
		_ = (&inventory.Cache{Path: filepath.Join(a.env.CacheDir, cacheFile)}).Invalidate()
	}
}

// usedCache dice si alguna máquina del inventario salió de la caché.
func usedCache(inv inventory.Inventory) bool {
	for _, tm := range inv.Timings {
		if tm.Cached {
			return true
		}
	}
	return false
}

// resolveTarget carga el inventario y resuelve la consulta de una persona. Usa la
// caché; si el agente no aparece y parte de los datos eran de la caché, vuelve a
// consultar en vivo antes de darlo por inexistente (puede haber nacido hace poco).
func (a *app) resolveTarget(ctx context.Context, cf commonFlags, query string) (inventory.Target, error) {
	// Nivel 0: un alias gana sobre cualquier otra coincidencia. Se sigue una
	// sola vez: lo que apunta un alias se busca como texto, no como otro alias.
	target, ok := a.aliases().Lookup(query)
	if !ok {
		return a.resolveQuery(ctx, cf, query)
	}
	t, err := a.resolveQuery(ctx, cf, target)
	if err != nil {
		return inventory.Target{}, fmt.Errorf("alias %q -> %q: %w", query, target, err)
	}
	return t, nil
}

func (a *app) resolveQuery(ctx context.Context, cf commonFlags, query string) (inventory.Target, error) {
	scope := ""
	if prefix, _, found := strings.Cut(query, "/"); found {
		scope = prefix
	}
	wanted := cf.machines
	cf = scopeToMachines(cf)
	cf.cached = true
	inv, err := a.load(ctx, cf, scope)
	if err != nil {
		return inventory.Target{}, err
	}
	narrow := func(inv inventory.Inventory) inventory.Inventory {
		// --machine acota la búsqueda igual que el prefijo "maquina/": solo compiten
		// los agentes de las máquinas nombradas (incluido el servidor local si se
		// nombra "local"), no el local por defecto.
		if len(wanted) > 0 {
			inv.Targets = onlyMachines(inv.Targets, wanted)
		}
		return inv
	}
	inv = narrow(inv)
	t, err := resolve.Resolve(inv.Targets, query)

	var notFound *resolve.NotFoundError
	if errors.As(err, &notFound) && usedCache(inv) {
		cf.fresh = true
		live, loadErr := a.loadQuiet(ctx, cf, scope)
		if loadErr != nil {
			return inventory.Target{}, loadErr
		}
		inv = narrow(live)
		t, err = resolve.Resolve(inv.Targets, query)
	}
	if err != nil {
		return a.explainResolveError(ctx, err, inv)
	}
	return t, nil
}

// scopeToMachines quita "local" de --machine, que no es una máquina guardada:
// si era la única, se consulta solo el servidor local.
func scopeToMachines(cf commonFlags) commonFlags {
	if len(cf.machines) == 0 {
		return cf
	}
	var saved machineList
	for _, m := range cf.machines {
		if !strings.EqualFold(m, inventory.LocalLabel) {
			saved = append(saved, m)
		}
	}
	cf.machines = saved
	if len(saved) == 0 {
		cf.local = true
	}
	return cf
}

// onlyMachines conserva los agentes de las máquinas nombradas.
func onlyMachines(targets []inventory.Target, wanted []string) []inventory.Target {
	var kept []inventory.Target
	for _, t := range targets {
		for _, w := range wanted {
			if strings.EqualFold(t.MachineLabel(), w) {
				kept = append(kept, t)
				break
			}
		}
	}
	return kept
}

func (a *app) ls(ctx context.Context, args []string) error {
	var cf commonFlags
	var vf viewFlags
	var verbose bool
	fs := newFlagSet("ls")
	cf.register(fs, true)
	cf.registerFresh(fs)
	vf.register(fs)
	fs.BoolVar(&verbose, "verbose", false, "print how long each server took to stderr")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usagef("ls takes no arguments, got %q", rest)
	}
	style, err := a.newStyle(vf.color)
	if err != nil {
		return err
	}

	cf.cached = true
	if a.canRedraw(cf) {
		// En una terminal el árbol se dibuja ya y se completa según responden las máquinas.
		inv, err := a.lsProgressive(ctx, cf, style, vf.fullNames)
		if err != nil {
			return err
		}
		a.warn(inv)
		if verbose {
			a.printTimings(inv.Timings)
		}
		return nil
	}
	inv, err := a.load(ctx, cf, "")
	if err != nil {
		return err
	}
	if verbose {
		defer a.printTimings(inv.Timings)
	}
	if cf.json {
		return a.printLsJSON(inv)
	}
	a.printTree(inv, style, vf.fullNames)
	return nil
}

// printTimings avisa por stderr cuánto tardó cada servidor, para saber cuál
// hace lento el listado sin tocar el formato de stdout.
func (a *app) printTimings(timings []inventory.Timing) {
	parts := make([]string, 0, len(timings))
	for _, t := range timings {
		name := t.Machine
		if name == "" {
			name = inventory.LocalLabel
		}
		part := fmt.Sprintf("%s %s", name, t.Elapsed.Round(time.Millisecond))
		if t.Err != nil {
			part += " (failed)"
		}
		parts = append(parts, part)
	}
	fmt.Fprintf(a.env.Stderr, "timings: %s\n", strings.Join(parts, ", "))
}

func (a *app) printLsJSON(inv inventory.Inventory) error {
	type warning struct {
		Machine string `json:"machine"`
		Error   string `json:"error"`
	}
	payload := struct {
		Targets  []inventory.Target `json:"targets"`
		Warnings []warning          `json:"warnings"`
	}{Targets: inv.Targets, Warnings: []warning{}}
	if payload.Targets == nil {
		payload.Targets = []inventory.Target{}
	}
	for _, w := range inv.Warnings {
		payload.Warnings = append(payload.Warnings, warning{Machine: w.Machine, Error: w.Err.Error()})
	}
	return writeJSON(a.env.Stdout, payload)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (a *app) id(ctx context.Context, args []string) error {
	var cf commonFlags
	var asCommand bool
	fs := newFlagSet("id")
	cf.register(fs, true)
	cf.registerFresh(fs)
	fs.BoolVar(&asCommand, "cmd", false, "print a ready-to-use herdr command prefix")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usagef("usage: hum id <who> [--json] [--cmd]")
	}

	t, err := a.resolveTarget(ctx, cf, rest[0])
	if err != nil {
		return err
	}
	switch {
	case cf.json:
		return writeJSON(a.env.Stdout, struct {
			Machine string `json:"machine"`
			PaneID  string `json:"pane_id"`
		}{t.Machine, t.PaneID})
	case asCommand:
		prefix := "herdr"
		if t.Machine != "" {
			prefix += " --machine " + shellQuote(t.Machine)
		}
		fmt.Fprintf(a.env.Stdout, "%s agent prompt %s\n", prefix, t.PaneID)
	default:
		fmt.Fprintln(a.env.Stdout, t.PaneID)
	}
	return nil
}

// shellQuote entrecomilla un argumento solo si contiene espacios.
func shellQuote(s string) string {
	if !strings.ContainsAny(s, " \t") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (a *app) say(ctx context.Context, args []string) error {
	var cf commonFlags
	var opts actions.SayOptions
	fs := newFlagSet("say")
	cf.register(fs, false)
	cf.registerFresh(fs)
	fs.BoolVar(&opts.Wait, "wait", false, "wait until the agent finishes")
	fs.IntVar(&opts.TimeoutMillis, "timeout", 0, "milliseconds to wait with --wait")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) < 2 {
		return usagef("usage: hum say <who> <message...> [--wait] [--timeout MS]")
	}
	if opts.TimeoutMillis != 0 && !opts.Wait {
		return usagef("--timeout only applies together with --wait")
	}

	t, err := a.resolveTarget(ctx, cf, rest[0])
	if err != nil {
		return err
	}
	svc := actions.Service{Runner: a.env.Runner}
	if err := svc.Say(ctx, t, strings.Join(rest[1:], " "), opts); err != nil {
		return targetHint(err, t)
	}
	fmt.Fprintf(a.env.Stdout, "sent to %s (%s)\n", t.Ref(), t.Name)
	return nil
}

func (a *app) look(ctx context.Context, args []string) error {
	var cf commonFlags
	var lines int
	fs := newFlagSet("look")
	cf.register(fs, false)
	cf.registerFresh(fs)
	fs.IntVar(&lines, "lines", 0, "number of lines to read")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usagef("usage: hum look <who> [--lines N]")
	}

	t, err := a.resolveTarget(ctx, cf, rest[0])
	if err != nil {
		return err
	}
	text, err := actions.Service{Runner: a.env.Runner}.Look(ctx, t, lines)
	if err != nil {
		return targetHint(err, t)
	}
	fmt.Fprint(a.env.Stdout, text)
	return nil
}

func (a *app) name(ctx context.Context, args []string) error {
	var cf commonFlags
	var clearName bool
	fs := newFlagSet("name")
	cf.register(fs, false)
	cf.registerFresh(fs)
	fs.BoolVar(&clearName, "clear", false, "remove the assigned name")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}

	var newName string
	switch {
	case clearName && len(rest) == 1:
	case !clearName && len(rest) == 2:
		newName = rest[1]
	default:
		return usagef("usage: hum name <who> <new-name> | hum name <who> --clear")
	}

	t, err := a.resolveTarget(ctx, cf, rest[0])
	if err != nil {
		return err
	}
	err = (actions.Service{Runner: a.env.Runner}).Name(ctx, t, newName)
	a.invalidateCache() // aunque falle a medias, lo guardado ya no es fiable
	if err != nil {
		return targetHint(err, t)
	}
	if clearName {
		fmt.Fprintf(a.env.Stdout, "cleared the name of %s\n", t.Ref())
	} else {
		fmt.Fprintf(a.env.Stdout, "named %s -> %s\n", t.Ref(), newName)
	}
	return nil
}

// rename cambia la etiqueta del tab o del workspace de un agente. A diferencia
// de "name", que nombra al agente, aquí se renombra el contenedor.
func (a *app) rename(ctx context.Context, args []string) error {
	const usage = "usage: hum rename --tab|--workspace <who> <label...>"
	var cf commonFlags
	var tab, workspace bool
	fs := newFlagSet("rename")
	cf.register(fs, false)
	cf.registerFresh(fs)
	fs.BoolVar(&tab, "tab", false, "rename the tab of the agent")
	fs.BoolVar(&workspace, "workspace", false, "rename the workspace of the agent")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if tab == workspace || len(rest) < 2 {
		return usagef("%s", usage)
	}
	label := strings.Join(rest[1:], " ")
	if err := actions.ValidLabel(label); err != nil {
		return err
	}

	t, err := a.resolveTarget(ctx, cf, rest[0])
	if err != nil {
		return err
	}
	svc := actions.Service{Runner: a.env.Runner}
	kind, id := "tab", t.TabID
	if tab {
		err = svc.RenameTab(ctx, t, label)
	} else {
		kind, id = "workspace", t.WorkspaceID
		err = svc.RenameWorkspace(ctx, t, label)
	}
	a.invalidateCache() // las etiquetas guardadas en la caché ya no valen
	if err != nil {
		return targetHint(err, t)
	}
	fmt.Fprintf(a.env.Stdout, "renamed %s %s on %s -> %s\n", kind, id, t.MachineLabel(), label)
	return nil
}

// closeContainer cierra el tab o el workspace de un agente. Es destructivo:
// detiene todo lo que corre dentro, así que en una terminal pregunta y sin
// terminal exige --yes. Cualquier respuesta que no sea "y" o "yes" cancela.
func (a *app) closeContainer(ctx context.Context, args []string) error {
	const usage = "usage: hum close --tab|--workspace <who> [--yes]"
	var cf commonFlags
	var tab, workspace, yes bool
	fs := newFlagSet("close")
	cf.register(fs, false)
	cf.registerFresh(fs)
	fs.BoolVar(&tab, "tab", false, "close the tab of the agent")
	fs.BoolVar(&workspace, "workspace", false, "close the workspace of the agent")
	fs.BoolVar(&yes, "yes", false, "do not ask for confirmation")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if tab == workspace || len(rest) != 1 {
		return usagef("%s", usage)
	}

	t, err := a.resolveTarget(ctx, cf, rest[0])
	if err != nil {
		return err
	}
	kind, id := "tab", t.TabID
	if workspace {
		kind, id = "workspace", t.WorkspaceID
	}
	if !yes {
		if !a.interactive() {
			return fmt.Errorf("refusing to close %s %s without confirmation; pass --yes to confirm", kind, id)
		}
		fmt.Fprintf(a.env.Stderr, "Close %s %s on %s? Every agent in it will stop. [y/N] ", kind, id, t.MachineLabel())
		answer, err := a.readLine(ctx)
		if err != nil || (!strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes")) {
			return errCancelled
		}
	}
	svc := actions.Service{Runner: a.env.Runner}
	if tab {
		err = svc.CloseTab(ctx, t)
	} else {
		err = svc.CloseWorkspace(ctx, t)
	}
	a.invalidateCache() // los agentes del contenedor ya no existen
	if err != nil {
		return targetHint(err, t)
	}
	fmt.Fprintf(a.env.Stdout, "closed %s %s on %s\n", kind, id, t.MachineLabel())
	return nil
}

func (a *app) focus(ctx context.Context, args []string) error {
	var cf commonFlags
	fs := newFlagSet("go")
	cf.register(fs, false)
	cf.registerFresh(fs)
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usagef("usage: hum go <who>")
	}

	t, err := a.resolveTarget(ctx, cf, rest[0])
	if err != nil {
		return err
	}
	if err := (actions.Service{Runner: a.env.Runner}).Focus(ctx, t); err != nil {
		return targetHint(err, t)
	}
	fmt.Fprintf(a.env.Stdout, "focused %s (%s)\n", t.Ref(), t.Name)
	return nil
}
