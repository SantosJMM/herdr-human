package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/SantosJMM/herdr-human/internal/complete"
	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/inventory"
)

// completionTimeout acota cada consulta local del autocompletado: una shell no
// debe quedarse esperando a hum.
const completionTimeout = 800 * time.Millisecond

// completion imprime el script de autocompletado de una shell.
func (a *app) completion(_ context.Context, args []string) error {
	fs := newFlagSet("completion")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usagef("usage: hum completion bash|zsh|powershell")
	}
	script, err := complete.Script(rest[0])
	if err != nil {
		return usagef("%v", err)
	}
	fmt.Fprint(a.env.Stdout, script)
	return nil
}

// complete es el comando interno que llaman los scripts: recibe la línea escrita
// hasta el cursor y escribe las sugerencias. Nunca falla ni imprime errores,
// porque lo que saliera por stderr se vería en medio del terminal.
func (a *app) complete(ctx context.Context, args []string) error {
	var shell, line string
	fs := newFlagSet("__complete")
	fs.StringVar(&shell, "shell", "bash", "")
	fs.StringVar(&line, "line", "", "")
	if _, err := parse(fs, args); err != nil {
		return nil
	}
	src := &completionSource{ctx: ctx, a: a}
	fmt.Fprint(a.env.Stdout, complete.Format(shell, complete.Complete(line, src)))
	return nil
}

// completionSource da los nombres al autocompletado sin esperar a la red: los
// agentes locales se consultan en vivo (milisegundos) y los remotos salen de lo
// último que guardó la caché, tenga la edad que tenga (hasta una hora).
type completionSource struct {
	ctx context.Context
	a   *app
}

func (s *completionSource) Targets() []inventory.Target {
	targets, _ := inventory.LoadMachine(s.ctx, s.a.env.Runner, "", completionTimeout)
	if s.a.env.CacheDir != "" {
		cache := &inventory.Cache{Path: filepath.Join(s.a.env.CacheDir, cacheFile), Now: s.a.env.Now}
		targets = append(targets, cache.Peek()...)
	}
	return targets
}

func (s *completionSource) Aliases() map[string]string {
	all, _ := s.a.aliases().List() // un archivo ilegible no suma sugerencias
	return all
}

func (s *completionSource) Machines() []string {
	ctx, cancel := context.WithTimeout(s.ctx, completionTimeout)
	defer cancel()
	machines, err := herdr.ListMachines(ctx, s.a.env.Runner)
	if err != nil {
		return nil
	}
	labels := make([]string, len(machines))
	for i, m := range machines {
		labels[i] = m.Label
	}
	return labels
}
