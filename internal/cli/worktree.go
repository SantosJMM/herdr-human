package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/inventory"
)

const worktreeUsage = `usage: hum worktree ls [<who>]
       hum worktree new <branch> [<who>] [--base REF] [--label TEXT] [--no-focus]
       hum worktree rm <who> [--force] [--yes]`

// worktree agrupa las ayudas de worktrees de Git de herdr. Con <who> opera
// sobre el workspace de ese agente (local o remoto); sin él, sobre la carpeta
// actual de esta máquina.
func (a *app) worktree(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usagef("%s", worktreeUsage)
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "ls":
		return a.worktreeLs(ctx, rest)
	case "new":
		return a.worktreeNew(ctx, rest)
	case "rm":
		return a.worktreeRm(ctx, rest)
	}
	return usagef("%s", worktreeUsage)
}

// worktreeScope arma el ámbito: el workspace del agente si se nombra uno, o la
// carpeta actual. Devuelve también el agente resuelto (vacío sin <who>).
func (a *app) worktreeScope(ctx context.Context, cf commonFlags, who string) (actions.WorktreeScope, inventory.Target, error) {
	if who != "" {
		t, err := a.resolveTarget(ctx, cf, who)
		if err != nil {
			return actions.WorktreeScope{}, t, err
		}
		return actions.WorktreeScope{Machine: t.Machine, Workspace: t.WorkspaceID}, t, nil
	}
	if a.env.Getwd == nil {
		return actions.WorktreeScope{}, inventory.Target{}, fmt.Errorf("the current folder is unknown; name an agent instead")
	}
	wd, err := a.env.Getwd()
	if err != nil || wd == "" {
		return actions.WorktreeScope{}, inventory.Target{}, fmt.Errorf("the current folder is unknown; name an agent instead")
	}
	return actions.WorktreeScope{Cwd: wd}, inventory.Target{}, nil
}

func (a *app) worktreeLs(ctx context.Context, args []string) error {
	var cf commonFlags
	fs := newFlagSet("worktree ls")
	cf.register(fs, false)
	cf.registerFresh(fs)
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) > 1 {
		return usagef("%s", worktreeUsage)
	}
	who := ""
	if len(rest) == 1 {
		who = rest[0]
	}
	sc, t, err := a.worktreeScope(ctx, cf, who)
	if err != nil {
		return err
	}
	list, err := (actions.Service{Runner: a.env.Runner}).WorktreeList(ctx, sc)
	if err != nil {
		return worktreeHint(targetHint(err, t))
	}
	for _, w := range list {
		kind := "main"
		switch {
		case w.Bare:
			kind = "bare"
		case w.Linked:
			kind = "linked"
		}
		branch := w.Branch
		if branch == "" || w.Detached {
			branch = "(detached)"
		}
		note := ""
		if w.Prunable {
			note = "  (prunable)"
		}
		fmt.Fprintf(a.env.Stdout, "%-8s %-24s %s%s\n", kind, branch, w.Path, note)
	}
	return nil
}

func (a *app) worktreeNew(ctx context.Context, args []string) error {
	var cf commonFlags
	var opts actions.WorktreeNewOptions
	fs := newFlagSet("worktree new")
	cf.register(fs, false)
	cf.registerFresh(fs)
	fs.StringVar(&opts.Base, "base", "", "create the branch from this reference")
	fs.StringVar(&opts.Label, "label", "", "label of the new workspace")
	fs.BoolVar(&opts.NoFocus, "no-focus", false, "do not move the focus to the new workspace")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) < 1 || len(rest) > 2 {
		return usagef("%s", worktreeUsage)
	}
	opts.Branch = rest[0]
	// Se valida antes de resolver al agente: un error de uso no debe tocar la red.
	if err := validateWorktreeNew(opts); err != nil {
		return usagef("%s\n%v", worktreeUsage, err)
	}
	who := ""
	if len(rest) == 2 {
		who = rest[1]
	}
	sc, t, err := a.worktreeScope(ctx, cf, who)
	if err != nil {
		return err
	}
	res, err := (actions.Service{Runner: a.env.Runner}).WorktreeNew(ctx, sc, opts)
	a.invalidateCache() // aparece un workspace nuevo
	if err != nil {
		return worktreeHint(targetHint(err, t))
	}
	fmt.Fprintf(a.env.Stdout, "created worktree %s", opts.Branch)
	if path := resultPath(res); path != "" {
		fmt.Fprintf(a.env.Stdout, " at %s", path)
	} else if len(res) > 0 && string(res) != "null" {
		fmt.Fprintf(a.env.Stdout, ": %s", res)
	}
	fmt.Fprintln(a.env.Stdout)
	return nil
}

func validateWorktreeNew(opts actions.WorktreeNewOptions) error {
	if err := actions.ValidBranch(opts.Branch); err != nil {
		return err
	}
	if opts.Base != "" {
		if err := actions.ValidBranch(opts.Base); err != nil {
			return err
		}
	}
	if opts.Label != "" {
		return actions.ValidLabel(opts.Label)
	}
	return nil
}

// resultPath busca "path" en la respuesta de herdr, que no tiene forma fija.
func resultPath(res json.RawMessage) string {
	var m struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(res, &m) != nil {
		return ""
	}
	return m.Path
}

func (a *app) worktreeRm(ctx context.Context, args []string) error {
	var cf commonFlags
	var force, yes bool
	fs := newFlagSet("worktree rm")
	cf.register(fs, false)
	cf.registerFresh(fs)
	fs.BoolVar(&force, "force", false, "remove even with uncommitted changes")
	fs.BoolVar(&yes, "yes", false, "do not ask for confirmation")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usagef("%s", worktreeUsage)
	}
	sc, t, err := a.worktreeScope(ctx, cf, rest[0])
	if err != nil {
		return err
	}
	if !yes {
		if !a.interactive() {
			return fmt.Errorf("refusing to remove the worktree of workspace %s without confirmation; pass --yes to confirm", sc.Workspace)
		}
		fmt.Fprintf(a.env.Stderr, "Remove the worktree checkout of workspace %s on %s? Its files are deleted. [y/N] ", sc.Workspace, t.MachineLabel())
		answer, err := a.readLine(ctx)
		if err != nil || (!strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes")) {
			return errCancelled
		}
	}
	err = (actions.Service{Runner: a.env.Runner}).WorktreeRemove(ctx, sc, force)
	a.invalidateCache()
	if err != nil {
		return worktreeHint(targetHint(err, t))
	}
	fmt.Fprintf(a.env.Stdout, "removed the worktree of workspace %s on %s\n", sc.Workspace, t.MachineLabel())
	return nil
}

// worktreeHint añade una pista cuando herdr pide confiar en el repositorio:
// hum nunca lo hace por la persona.
func worktreeHint(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "trust") {
		return withHint(err, "verify the repository and, if you trust it, repeat the command with herdr directly using --trust-repository; hum never passes it")
	}
	return err
}
