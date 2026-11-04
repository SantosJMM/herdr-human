package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// defaultWatchInterval es cada cuánto se sondea con --watch; cada sondeo
// remoto cuesta ~1,3 s, así que no conviene bajar mucho de aquí.
const defaultWatchInterval = 5 * time.Second

// attentionView reparte los agentes por urgencia.
type attentionView struct {
	// NeedsYou son los agentes blocked (primero) y done.
	NeedsYou []inventory.Target `json:"needs_you"`
	Working  []inventory.Target `json:"working"`
	// Idle solo se cuenta: no hay nada que hacer con ellos.
	Idle int `json:"idle"`
}

func classify(targets []inventory.Target) attentionView {
	view := attentionView{NeedsYou: []inventory.Target{}, Working: []inventory.Target{}}
	var done []inventory.Target
	for _, t := range targets {
		switch t.Status {
		case "blocked":
			view.NeedsYou = append(view.NeedsYou, t)
		case "done":
			done = append(done, t)
		case "working":
			view.Working = append(view.Working, t)
		default:
			view.Idle++
		}
	}
	view.NeedsYou = append(view.NeedsYou, done...)
	return view
}

// attention muestra quién necesita a la persona. Con --watch redibuja cada
// pocos segundos y, con --notify, avisa una vez por cada transición a
// blocked o done.
func (a *app) attention(ctx context.Context, args []string) error {
	var cf commonFlags
	var vf viewFlags
	var watch, notify bool
	var interval time.Duration
	fs := newFlagSet("attention")
	cf.register(fs, true)
	vf.register(fs)
	fs.BoolVar(&watch, "watch", false, "keep running and redraw every --interval")
	fs.BoolVar(&notify, "notify", false, "with --watch, show a herdr notification when an agent becomes blocked or done")
	fs.DurationVar(&interval, "interval", defaultWatchInterval, "time between checks with --watch")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(rest) != 0:
		return usagef("attention takes no arguments, got %q", rest)
	case notify && !watch:
		return usagef("--notify only applies together with --watch")
	case watch && cf.json:
		return usagef("--json cannot be combined with --watch")
	case watch && interval <= 0:
		return usagef("--interval must be positive")
	}

	style, err := a.newStyle(vf.color)
	if err != nil {
		return err
	}

	// Siempre en vivo (muestra estados), pero lo que consulta renueva la caché para
	// que ls y el autocompletado tengan datos recientes.
	cf.cached, cf.fresh = true, true

	if !watch {
		inv, err := a.load(ctx, cf, "")
		if err != nil {
			return err
		}
		view := classify(inv.Targets)
		if cf.json {
			return writeJSON(a.env.Stdout, view)
		}
		printAttention(a.env.Stdout, view, style, vf.fullNames)
		return nil
	}
	return a.watchAttention(ctx, cf, interval, notify, style, vf.fullNames)
}

// watchAttention sondea hasta que se cancele el contexto (Ctrl+C), lo que es
// una salida normal. Un fallo transitorio de una máquina no la interrumpe.
func (a *app) watchAttention(ctx context.Context, cf commonFlags, interval time.Duration, notify bool, style *ui.Style, full bool) error {
	svc := actions.Service{Runner: a.env.Runner}
	sleep := a.env.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	previous := map[string]string{}
	first := true
	for {
		inv, err := inventory.LoadCached(ctx, a.env.Runner, inventory.Options{
			Machines: cf.machines, LocalOnly: cf.local, Timeout: cf.serverTimeout,
		}, a.cacheFor(cf))
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}

		if a.env.Terminal {
			fmt.Fprint(a.env.Stdout, "\x1b[H\x1b[2J")
		} else if !first {
			fmt.Fprintln(a.env.Stdout, "---")
		}
		printAttention(a.env.Stdout, classify(inv.Targets), style, full)
		for _, w := range inv.Warnings {
			fmt.Fprintf(a.env.Stdout, "warning: %v\n", w)
		}

		current := make(map[string]string, len(inv.Targets))
		for _, t := range inv.Targets {
			current[t.Ref()] = t.Status
			if notify && !first && previous[t.Ref()] != t.Status && (t.Status == "blocked" || t.Status == "done") {
				a.notifyEnd(ctx, svc, true, t, "is "+t.Status)
			}
		}
		previous, first = current, false

		if err := sleep(ctx, interval); err != nil {
			return nil
		}
	}
}

// sleepContext espera d o hasta que el contexto termine.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func printAttention(w io.Writer, v attentionView, st *ui.Style, full bool) {
	if len(v.NeedsYou) == 0 {
		fmt.Fprintln(w, "Nothing needs you.")
	} else {
		printSection(w, st.Bold(fmt.Sprintf("Needs you (%d)", len(v.NeedsYou))), v.NeedsYou, st, full)
	}
	if len(v.Working) > 0 {
		fmt.Fprintln(w)
		printSection(w, st.Bold(fmt.Sprintf("Working (%d)", len(v.Working))), v.Working, st, full)
	}
	if v.Idle > 0 {
		fmt.Fprintf(w, "\nIdle: %d more (hum ls)\n", v.Idle)
	}
}

// printSection imprime un grupo con las columnas alineadas. El relleno se hace
// con ui.PadRight porque text/tabwriter cuenta las secuencias ANSI como texto.
func printSection(w io.Writer, title string, rows []inventory.Target, st *ui.Style, full bool) {
	fmt.Fprintln(w, title)
	var refW, agentW, statusW int
	for _, t := range rows {
		refW = max(refW, ui.VisibleLen(t.Ref()))
		agentW = max(agentW, ui.VisibleLen(t.Agent))
		statusW = max(statusW, ui.VisibleLen(t.Status))
	}
	gap := strings.Repeat(" ", ui.Gap)
	for _, t := range rows {
		fmt.Fprintf(w, "  %s%s%s%s%s%s%s\n",
			ui.PadRight(t.Ref(), refW), gap,
			st.Paint(ui.RoleDim, ui.PadRight(t.Agent, agentW)), gap,
			st.Paint(ui.StatusRole(t.Status), ui.PadRight(t.Status, statusW)), gap,
			ui.ShortName(t.Name, full))
	}
}
