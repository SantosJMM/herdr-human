package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/resolve"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// hintError acompaña un error con una pista de qué hacer; se imprime en una
// línea "hint:" aparte. Solo se usa donde la causa es conocida: un fallo sin
// explicación no inventa pistas.
type hintError struct {
	err  error
	hint string
}

func (e *hintError) Error() string { return e.err.Error() }
func (e *hintError) Unwrap() error { return e.err }

func withHint(err error, format string, args ...any) error {
	return &hintError{err: err, hint: fmt.Sprintf(format, args...)}
}

// errCancelled indica que la persona decidió no elegir (Enter, q o Ctrl+C).
var errCancelled = errors.New("cancelled")

// interactive indica si se puede preguntar a la persona: hacen falta una
// terminal en la entrada (para leer la respuesta) y en la salida (para que vea
// la pregunta).
func (a *app) interactive() bool {
	return a.env.Terminal && a.env.StdinTerminal && a.env.Stdin != nil
}

// explainResolveError convierte un fallo del resolutor en algo accionable:
// ofrece elegir entre los candidatos si hay una persona delante, o explica cómo
// precisar la consulta.
func (a *app) explainResolveError(ctx context.Context, err error, inv inventory.Inventory) (inventory.Target, error) {
	var amb *resolve.AmbiguousError
	var nf *resolve.NotFoundError
	switch {
	case errors.As(err, &amb):
		t, asked, perr := a.pick(ctx, amb)
		if perr != nil {
			return inventory.Target{}, perr
		}
		if asked {
			return t, nil
		}
		return inventory.Target{}, withHint(err, `use "machine/pane", e.g. "%s"`, amb.Candidates[0].Ref())
	case errors.As(err, &nf):
		if len(inv.Warnings) > 0 {
			names := make([]string, len(inv.Warnings))
			for i, w := range inv.Warnings {
				names[i] = w.Machine
			}
			return inventory.Target{}, withHint(err, `could not reach %s; the agent may be there (see the warnings above)`,
				strings.Join(names, ", "))
		}
		return inventory.Target{}, withHint(err, `run "hum ls" to see every agent`)
	}
	return inventory.Target{}, err
}

// pick pregunta cuál de los candidatos se quiso decir. Devuelve ok=true con el
// elegido; ok=false si no hay una persona delante o la respuesta no es un
// número válido (se conserva el error original). Enter, "q", el fin de la
// entrada y Ctrl+C cancelan con errCancelled: la lectura corre aparte para que
// Ctrl+C no tenga que esperar a que se pulse Enter.
func (a *app) pick(ctx context.Context, amb *resolve.AmbiguousError) (t inventory.Target, ok bool, err error) {
	if !a.interactive() {
		return inventory.Target{}, false, nil
	}
	n := len(amb.Candidates)
	fmt.Fprintf(a.env.Stderr, "hum: %q matches %d agents\n", amb.Query, n)
	refW := 0
	for _, c := range amb.Candidates {
		refW = max(refW, ui.VisibleLen(c.Ref()))
	}
	for i, c := range amb.Candidates {
		fmt.Fprintf(a.env.Stderr, "  %d) %s  %s  %s\n", i+1, ui.PadRight(c.Ref(), refW), c.Agent, ui.ShortName(c.Name, false))
	}
	fmt.Fprintf(a.env.Stderr, "Which one? [1-%d, Enter to cancel] ", n)

	lines := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(a.env.Stdin).ReadString('\n')
		lines <- line
	}()

	var line string
	select {
	case line = <-lines:
	case <-ctx.Done():
		fmt.Fprintln(a.env.Stderr)
		return inventory.Target{}, false, errCancelled
	}

	answer := strings.TrimSpace(line)
	if answer == "" || strings.EqualFold(answer, "q") {
		fmt.Fprintln(a.env.Stderr)
		return inventory.Target{}, false, errCancelled
	}
	choice, convErr := strconv.Atoi(answer)
	if convErr != nil || choice < 1 || choice > n {
		return inventory.Target{}, false, nil
	}
	return amb.Candidates[choice-1], true, nil
}

// targetHint añade la pista que corresponde a los errores frecuentes de herdr
// al actuar sobre un agente ya resuelto.
func targetHint(err error, t inventory.Target) error {
	if err == nil {
		return nil
	}
	var api *herdr.APIError
	if errors.As(err, &api) {
		switch api.Code {
		case "agent_blocked":
			return withHint(err, "the agent is waiting for you; run: hum go %s", t.Ref())
		case "workspace_group_close_required":
			return withHint(err, "this workspace has linked worktree workspaces; hum does not close the whole group. Close the linked ones first (hum close --workspace <who>) or use herdr workspace close --group yourself")
		case "agent_not_found":
			return withHint(err, `the agent may have been closed; run "hum ls"`)
		}
	}
	if errors.Is(err, actions.ErrInvalidName) {
		return withHint(err, "for example: hum name %s docs-agent", t.Ref())
	}
	return err
}
