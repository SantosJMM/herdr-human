package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/gitinfo"
	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/tui"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// selector abre el selector interactivo y, si se elige un agente, lleva el foco de
// herdr hasta él. Necesita una terminal en la entrada y en la salida.
func (a *app) selector(ctx context.Context, args []string) error {
	var cf commonFlags
	var vf viewFlags
	var embedded bool
	fs := newFlagSet("pick")
	cf.register(fs, false)
	vf.register(fs)
	fs.BoolVar(&embedded, "embedded", false, "draw no frame or title: the herdr popup already has its own")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usagef("usage: hum pick [--local] [--machine NAME] [--full-names] [--embedded]")
	}
	if !a.interactive() {
		return withHint(errors.New("pick needs an interactive terminal"), `use "hum ls" for a list you can pipe`)
	}

	style, err := a.newStyle(vf.color)
	if err != nil {
		return err
	}

	// El selector se abre al instante: cada servidor aparece cuando responde y
	// los que fallan se explican en su encabezado, así que no hay avisos por
	// stderr. Al salir se cancelan las consultas que sigan en curso.
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Siempre en vivo, pero lo que responda renueva la caché para ls y el autocompletado.
	cf.cached, cf.fresh = true, true
	stream, err := inventory.StartCached(sctx, a.env.Runner, a.options(cf, ""), a.cacheFor(cf))
	if err != nil {
		return err
	}

	t, ok, err := tui.Run(sctx, a.env.Stdin, a.env.Stdout, tui.Options{
		Style: style, FullNames: vf.fullNames, Machines: stream.Machines, Updates: stream.Updates,
		Actions:  pickActions{svc: actions.Service{Runner: a.env.Runner}, copy: a.env.Copy},
		Ctx:      sctx,
		Branch:   gitinfo.Branch,
		Embedded: embedded,
		Refresh: func(ctx context.Context, machine string) ([]inventory.Target, error) {
			return inventory.LoadMachine(ctx, a.env.Runner, machine, cf.serverTimeout)
		},
	})
	if err != nil {
		return fmt.Errorf("selector: %w", err)
	}
	if !ok {
		return nil
	}
	if err := (actions.Service{Runner: a.env.Runner}).Focus(ctx, t); err != nil {
		return targetHint(err, t)
	}
	fmt.Fprintf(a.env.Stdout, "focused %s (%s)\n", t.Ref(), t.Name)
	return nil
}

// lookLines es cuántas líneas del terminal muestra la acción Look del selector.
const lookLines = 200

// pickActions conecta el menú del selector con las acciones de herdr.
type pickActions struct {
	svc  actions.Service
	copy func(string) error
}

func (p pickActions) Say(ctx context.Context, t inventory.Target, text string) error {
	return p.svc.Say(ctx, t, text, actions.SayOptions{})
}

func (p pickActions) Look(ctx context.Context, t inventory.Target) (string, error) {
	return p.svc.Look(ctx, t, lookLines)
}

func (p pickActions) Rename(ctx context.Context, t inventory.Target, name string) error {
	return p.svc.Name(ctx, t, name)
}

func (p pickActions) Ask(ctx context.Context, t inventory.Target, text string) (tui.Reply, error) {
	a, err := p.svc.Ask(ctx, t, text, actions.AskOptions{Lines: actions.DefaultReplyLines})
	if err != nil {
		return tui.Reply{}, err
	}
	return tui.Reply{Status: a.Status, Text: a.Reply, AfterMessage: a.AfterMessage, Lines: actions.DefaultReplyLines}, nil
}

// WaitNotify espera a que el agente se asiente y avisa con una notificación de
// herdr; si el aviso falla, el estado se devuelve igual.
func (p pickActions) WaitNotify(ctx context.Context, t inventory.Target) (string, error) {
	status, err := p.svc.Wait(ctx, t, actions.WaitOptions{})
	if err != nil {
		return "", err
	}
	_ = p.svc.Notify(ctx, ui.ShortName(t.Name, false)+" is "+status, t.Ref())
	return status, nil
}

func (p pickActions) Copy(text string) error {
	if p.copy == nil {
		return errors.New("no clipboard available")
	}
	return p.copy(text)
}
