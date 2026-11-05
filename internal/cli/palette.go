package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/palette"
	"github.com/SantosJMM/herdr-human/internal/tui"
)

const (
	// defaultPluginID es el id del plugin cuando herdr no lo informa.
	defaultPluginID = "herdr-human"
	// paletteID es el id de la acción y del pane de la paleta en el manifest.
	paletteID = "palette"
	// Espera a que el popup termine de cerrarse antes de lanzar lo elegido: mientras
	// está abierto, cualquier comando que abra un pane o popup recibe `ui_busy`.
	// Se espera un poco y se reintenta mientras herdr siga diciendo que está ocupado.
	runFirstDelay   = 30 * time.Millisecond
	runRetryDelay   = 50 * time.Millisecond
	runMaxAttempts  = 40
	uiBusyCode      = "ui_busy"
	paletteRunUsage = "usage: hum palette run --action PLUGIN ACTION | --command HERDR_ARGS..."
)

// palette despacha `hum palette [open|run]`.
func (a *app) palette(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return a.paletteUI(ctx)
	}
	switch args[0] {
	case "open":
		if len(args) != 1 {
			return usagef("usage: hum palette open")
		}
		return a.paletteOpen(ctx)
	case "run":
		return a.paletteRun(ctx, args[1:])
	}
	return usagef("usage: hum palette [open | run ...]")
}

func (a *app) pluginID() string {
	if id := a.getenv("HERDR_PLUGIN_ID"); id != "" {
		return id
	}
	return defaultPluginID
}

// paletteOpen es lo que ejecuta el atajo: abre el popup de la paleta. Es lo
// único que hace; herdr le pasa al popup el workspace, la pestaña y el pane de
// origen, así que no hace falta preguntarlos ni reenviarlos.
func (a *app) paletteOpen(ctx context.Context) error {
	_, err := herdr.Text(ctx, a.env.Runner, "", []string{
		"plugin", "pane", "open", "--plugin", a.pluginID(), "--entrypoint", paletteID, "--focus",
	})
	return err
}

// paletteUI dibuja la paleta. Se pinta al instante con los comandos embebidos y
// las acciones de plugins de la última vez; las acciones frescas se piden a
// herdr en segundo plano y se incorporan cuando llegan.
func (a *app) paletteUI(ctx context.Context) error {
	if !a.interactive() {
		return withHint(errors.New("the palette needs an interactive terminal"), `open it with the herdr key bound to the "palette" action`)
	}
	style, err := a.newStyle("")
	if err != nil {
		return err
	}
	native, err := palette.Native()
	if err != nil {
		return err
	}

	var (
		dir     = palette.StateDir(a.getenv)
		origin  = palette.OriginFromEnv(a.getenv)
		chords  = palette.LoadChords(a.getenv)
		recents = palette.LoadRecents(dir)
		self    = a.pluginID() + "." + paletteID
	)
	merge := func(plugins []palette.Entry) []palette.Entry {
		return palette.Available(palette.Merge(chords, recents, native, plugins), origin)
	}

	// El primer cuadro no espera a herdr: lo embebido y la caché están en disco.
	cached := palette.LoadCache(dir)
	ictx, cancel := context.WithCancel(ctx)
	defer cancel()
	updates := make(chan tui.PluginUpdate, 1)
	go func() {
		plugins, err := palette.ListPlugins(ictx, a.env.Runner)
		if err != nil {
			updates <- tui.PluginUpdate{Err: err}
			return
		}
		entries := palette.PluginEntries(plugins, palette.Platform(a.env.GOOS), self)
		palette.SaveCache(dir, entries)
		updates <- tui.PluginUpdate{Entries: entries}
	}()

	vals := &palette.Context{Origin: origin, Lists: palette.HerdrLister{Runner: a.env.Runner}}
	res, err := tui.RunPalette(ictx, a.env.Stdin, a.env.Stdout, tui.PaletteOptions{
		Style:   style,
		Entries: merge(cached),
		Updates: updates,
		Merge:   merge,
		Resolve: func(ctx context.Context, e palette.Entry, p palette.Prompter) ([]string, bool, error) {
			return palette.Resolve(ctx, e, vals, p)
		},
	})
	if err != nil {
		return fmt.Errorf("palette: %w", err)
	}
	if res.Err != nil {
		return res.Err
	}
	if !res.Chosen {
		return nil
	}
	recents.Record(dir, res.Entry.ID)
	return a.dispatchPalette(res)
}

// dispatchPalette entrega lo elegido a un proceso aparte y termina. El popup de
// la paleta es modal: mientras viva, herdr rechaza todo lo que abra un pane o
// popup. El proceso aparte espera a que el popup se cierre y entonces lo ejecuta.
func (a *app) dispatchPalette(res tui.PaletteResult) error {
	if a.env.Detach == nil {
		return errors.New("this build cannot start a background process")
	}
	args := []string{"palette", "run"}
	if res.Entry.Kind == palette.KindNative {
		args = append(append(args, "--command"), res.Argv...)
	} else {
		args = append(args, "--action", res.Entry.PluginID, res.Entry.ActionID)
	}
	return a.env.Detach(args)
}

// paletteRun ejecuta lo que eligió la paleta, ya sin el popup. Si falla, avisa
// con una notificación de herdr, porque nadie mira la terminal de este proceso.
func (a *app) paletteRun(ctx context.Context, args []string) error {
	var argv []string
	var label string
	switch {
	case len(args) == 3 && args[0] == "--action":
		argv = []string{"plugin", "action", "invoke", args[2], "--plugin", args[1]}
		label = args[1] + "." + args[2]
	case len(args) >= 2 && args[0] == "--command":
		argv = args[1:]
		label = "herdr " + strings.Join(argv, " ")
	default:
		return usagef("%s", paletteRunUsage)
	}

	err := a.runWhenFree(ctx, argv)
	if err != nil {
		_, _ = herdr.Text(ctx, a.env.Runner, "", []string{"notification", "show", "Command palette", "--body", label + " failed: " + err.Error()})
	}
	return err
}

// runWhenFree ejecuta el comando de herdr en cuanto el popup deja de estar abierto.
func (a *app) runWhenFree(ctx context.Context, argv []string) error {
	delay := runFirstDelay
	var err error
	for range runMaxAttempts {
		if err = a.sleepFor(ctx, delay); err != nil {
			return err
		}
		_, err = herdr.Text(ctx, a.env.Runner, "", argv)
		var api *herdr.APIError
		if err == nil || !errors.As(err, &api) || api.Code != uiBusyCode {
			return err
		}
		delay = runRetryDelay
	}
	return err
}

func (a *app) sleepFor(ctx context.Context, d time.Duration) error {
	if a.env.Sleep != nil {
		return a.env.Sleep(ctx, d)
	}
	return sleepContext(ctx, d)
}
