package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

const newUsage = "usage: hum new <kind> [folder] [--name NAME] [--machine NAME] [--no-focus] [--timeout MS]"

// newAgent abre un tab, arranca un agente en él y lo nombra. El progreso va a
// stderr y el resultado (maquina/pane) a stdout, para poder capturarlo.
func (a *app) newAgent(ctx context.Context, args []string) error {
	var name, machine string
	var noFocus bool
	var timeout int
	fs := newFlagSet("new")
	fs.StringVar(&name, "name", "", "name for the agent and its pane (default: the kind)")
	fs.StringVar(&machine, "machine", "", "machine where the agent is created (default: this one)")
	fs.BoolVar(&noFocus, "no-focus", false, "do not move the focus to the new tab")
	fs.IntVar(&timeout, "timeout", 0, "milliseconds to wait for the agent to be ready")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) > 2 || timeout < 0 {
		return usagef("%s", newUsage)
	}

	var kind, dir string
	if len(rest) > 0 {
		kind = rest[0]
	}
	if len(rest) > 1 {
		dir = rest[1]
	}
	// "maquina/tipo" y --machine son la misma cosa; si discrepan, no se adivina.
	if m, k, found := strings.Cut(kind, "/"); found {
		if machine != "" && !strings.EqualFold(machine, m) {
			return usagef("conflicting machines: %q in the kind and --machine %q", m, machine)
		}
		machine, kind = m, k
	}
	if strings.EqualFold(machine, inventory.LocalLabel) {
		machine = ""
	}

	if kind == "" {
		if !a.interactive() {
			return usagef("%s", newUsage)
		}
		if kind, dir, name, err = a.promptNew(ctx, machine == "", dir, name); err != nil {
			return err
		}
	}
	if machine == "" {
		dir = a.localDir(dir)
	}

	style, err := a.newStyle("")
	if err != nil {
		return err
	}
	fmt.Fprintf(a.env.Stderr, "creating %s...\n", kind)
	created, err := actions.Service{Runner: a.env.Runner}.Create(ctx, machine, actions.CreateOptions{
		Kind: kind, Dir: dir, Name: name, Focus: !noFocus, TimeoutMillis: timeout,
	})
	a.invalidateCache() // haya salido bien o a medias, hay un agente o un tab que la caché no conoce
	a.reportCreated(style, created, kind, name)
	if err != nil {
		return newHint(err, machine, created, name)
	}
	ref := inventory.Target{Machine: machine, PaneID: created.PaneID}.Ref()
	fmt.Fprintln(a.env.Stdout, ref)
	return nil
}

// reportCreated cuenta en stderr los pasos que sí se hicieron.
func (a *app) reportCreated(st *ui.Style, c actions.Created, kind, name string) {
	ok := st.Paint(ui.RoleIdle, st.Glyphs().Check)
	if c.TabID != "" {
		fmt.Fprintf(a.env.Stderr, "%s tab created (%s)\n", ok, c.TabID)
	}
	if c.Started {
		fmt.Fprintf(a.env.Stderr, "%s %s is ready\n", ok, kind)
	}
	if c.Named {
		fmt.Fprintf(a.env.Stderr, "%s named %q\n", ok, name)
	}
}

// newHint traduce un fallo de Create a una pista de qué hacer; nada se deshace,
// así que se dice qué quedó y cómo limpiarlo.
func newHint(err error, machine string, c actions.Created, name string) error {
	if errors.Is(err, actions.ErrUnknownKind) || errors.Is(err, actions.ErrInvalidName) {
		return usagef("%v", err)
	}
	var ce *actions.CreateError
	if !errors.As(err, &ce) {
		return err
	}
	switch ce.Step {
	case "agent":
		prefix := "herdr"
		if machine != "" {
			prefix += " --machine " + shellQuote(machine)
		}
		return withHint(err, "the tab was left open; close it with: %s tab close %s", prefix, c.TabID)
	case "name":
		ref := inventory.Target{Machine: machine, PaneID: c.PaneID}.Ref()
		return withHint(err, "the agent is running; name it with: hum name %s %s", ref, name)
	}
	return err
}

// localDir resuelve la carpeta en este equipo: vacía es la actual, "~" es la
// carpeta personal y una ruta relativa cuelga de la actual. En una máquina
// remota la carpeta se envía tal cual, porque esta no existe allí.
func (a *app) localDir(dir string) string {
	wd := ""
	if a.env.Getwd != nil {
		wd, _ = a.env.Getwd()
	}
	switch {
	case dir == "":
		return wd
	case dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`):
		home := a.getenv("HOME")
		if home == "" {
			home = a.getenv("USERPROFILE")
		}
		if home == "" {
			return dir
		}
		return filepath.Join(home, strings.TrimLeft(dir[1:], `/\`))
	case filepath.IsAbs(dir) || wd == "":
		return dir
	}
	return filepath.Join(wd, dir)
}

// promptNew pregunta lo que falta cuando "hum new" se ejecuta sin argumentos en
// una terminal: tipo, carpeta (por defecto la actual) y nombre opcional.
func (a *app) promptNew(ctx context.Context, local bool, dir, name string) (string, string, string, error) {
	fmt.Fprintln(a.env.Stderr, "Kinds:")
	for i, k := range actions.Kinds {
		fmt.Fprintf(a.env.Stderr, "  %2d) %s\n", i+1, k)
	}
	fmt.Fprint(a.env.Stderr, "Kind (name or number): ")
	answer, err := a.readLine(ctx)
	if err != nil || answer == "" {
		return "", "", "", errCancelled
	}
	kind := answer
	if n, convErr := strconv.Atoi(answer); convErr == nil {
		if n < 1 || n > len(actions.Kinds) {
			return "", "", "", usagef("%d is not one of the listed kinds", n)
		}
		kind = actions.Kinds[n-1]
	}

	if dir == "" && local {
		def := a.localDir("")
		fmt.Fprintf(a.env.Stderr, "Folder [%s]: ", def)
		if dir, err = a.readLine(ctx); err != nil {
			return "", "", "", errCancelled
		}
	}
	if name == "" {
		fmt.Fprint(a.env.Stderr, "Name (optional): ")
		if name, err = a.readLine(ctx); err != nil {
			return "", "", "", errCancelled
		}
	}
	return kind, dir, name, nil
}

// readLine lee una línea de la entrada sin bloquear a Ctrl+C. El lector se
// comparte entre llamadas para no perder lo que ya se leyó por adelantado.
func (a *app) readLine(ctx context.Context) (string, error) {
	if a.in == nil {
		a.in = bufio.NewReader(a.env.Stdin)
	}
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := a.in.ReadString('\n')
		done <- result{line, err}
	}()
	select {
	case r := <-done:
		if r.err != nil && !(errors.Is(r.err, io.EOF) && r.line != "") {
			return "", r.err
		}
		return strings.TrimSpace(r.line), nil
	case <-ctx.Done():
		return "", errCancelled
	}
}
