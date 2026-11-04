package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/inventory"
)

// statusList acumula los valores repetidos de --until.
type statusList []string

func (s *statusList) String() string { return strings.Join(*s, ",") }

func (s *statusList) Set(v string) error {
	if !validStatus(v) {
		return fmt.Errorf("invalid status %q (want idle, working, blocked, done or unknown)", v)
	}
	*s = append(*s, v)
	return nil
}

func validStatus(v string) bool {
	switch v {
	case "idle", "working", "blocked", "done", "unknown":
		return true
	}
	return false
}

func (s statusList) contains(v string) bool {
	for _, have := range s {
		if have == v {
			return true
		}
	}
	return false
}

// wait espera a que un agente termine o necesite a la persona. Sale con 3 si
// el agente quedó blocked sin que se pidiera ese estado, y con 4 si venció el plazo.
func (a *app) wait(ctx context.Context, args []string) error {
	var cf commonFlags
	var until statusList
	var timeoutMillis int
	var notify bool
	fs := newFlagSet("wait")
	cf.register(fs, false)
	cf.registerFresh(fs)
	fs.Var(&until, "until", "status that ends the wait (repeatable)")
	fs.IntVar(&timeoutMillis, "timeout", 0, "milliseconds to wait before giving up")
	fs.BoolVar(&notify, "notify", false, "show a herdr notification when the wait ends")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usagef("usage: hum wait <who> [--until STATUS]... [--timeout MS] [--notify]")
	}

	t, err := a.resolveTarget(ctx, cf, rest[0])
	if err != nil {
		return err
	}
	svc := actions.Service{Runner: a.env.Runner}
	status, err := svc.Wait(ctx, t, actions.WaitOptions{Until: until, TimeoutMillis: timeoutMillis})
	if errors.Is(err, actions.ErrTimeout) {
		a.notifyEnd(ctx, svc, notify, t, "timed out")
		return &exitError{code: exitTimeout, err: fmt.Errorf("timed out waiting for %s (%s)", t.Ref(), t.Name)}
	}
	if err != nil {
		return targetHint(err, t)
	}

	fmt.Fprintf(a.env.Stdout, "%s (%s) is %s\n", t.Ref(), t.Name, status)
	a.notifyEnd(ctx, svc, notify, t, "is "+status)
	if status == "blocked" && !until.contains("blocked") {
		return &exitError{code: exitBlocked, err: fmt.Errorf(
			"%s is waiting for you; run: hum go %s", t.Ref(), t.Ref())}
	}
	return nil
}

// notifyEnd avisa con una notificación de herdr; un fallo es solo un aviso.
func (a *app) notifyEnd(ctx context.Context, svc actions.Service, enabled bool, t inventory.Target, what string) {
	if !enabled {
		return
	}
	if err := svc.Notify(ctx, t.Name+" "+what, t.Ref()); err != nil {
		fmt.Fprintf(a.env.Stderr, "warning: notification failed: %v\n", err)
	}
}
