package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/palette"
	"github.com/SantosJMM/herdr-human/internal/trace"
	"github.com/SantosJMM/herdr-human/internal/tui"
)

const (
	// defaultPluginID es el id del plugin cuando herdr no lo informa.
	defaultPluginID = "herdr-human"
	// paletteID es el id de la acción y del pane de la paleta en el manifest.
	paletteID = "palette"
	// Mientras un popup está abierto (o terminando de cerrarse), cualquier comando
	// que abra un pane o popup recibe `ui_busy`. Se reintenta con una espera corta
	// mientras herdr siga diciendo que está ocupado.
	runFirstDelay  = 30 * time.Millisecond
	busyRetryDelay = 50 * time.Millisecond
	// Tras elegir, el popup anterior puede tardar en cerrarse: hasta ~2 s.
	runMaxAttempts = 40
	// Al abrir, solo cubre el cierre del popup anterior: hasta ~0,5 s. Si de
	// verdad hay otro popup abierto, no tiene sentido insistir más.
	openMaxAttempts = 10
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
func (a *app) paletteOpen(ctx context.Context) error { return a.openPane(ctx, paletteID) }

// openPane abre y enfoca el pane del plugin con ese id. Reintenta unos instantes
// si el popup anterior aún se está cerrando.
func (a *app) openPane(ctx context.Context, entrypoint string) error {
	argv := []string{"plugin", "pane", "open", "--plugin", a.pluginID(), "--entrypoint", entrypoint, "--focus"}
	if a.env.Trace != nil {
		// Con la traza encendida, el popup sabe cuándo empezó todo (el arranque de
		// este proceso, que es lo más cerca que hay de la pulsación de la tecla).
		argv = append(argv, "--env", trace.TriggerEnv+"="+trace.FormatTrigger(a.env.Trace.Start()))
	}
	a.env.Trace.Mark("open: asking herdr to open the pane")
	err := a.herdrWhenFree(ctx, argv, 0, openMaxAttempts)
	a.env.Trace.Mark("open: herdr answered")
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
	initial := merge(cached)
	a.env.Trace.Mark("palette: catalog, cache and settings ready")
	ictx, cancel := context.WithCancel(ctx)
	defer cancel()
	updates := make(chan tui.PluginUpdate, 1)
	go func() {
		plugins, err := palette.ListPlugins(ictx, a.env.Runner)
		a.env.Trace.Mark("palette: herdr answered with the plugin list")
		if err != nil {
			updates <- tui.PluginUpdate{Err: err}
			return
		}
		entries := palette.PluginEntries(plugins, palette.Platform(a.env.GOOS), self)
		palette.SaveCache(dir, entries)
		updates <- tui.PluginUpdate{Entries: entries}
	}()

	vals := &palette.Context{Origin: origin, Lists: palette.HerdrLister{Runner: a.env.Runner}}
	res, err := tui.RunPalette(ictx, a.env.Stdin, a.tracedStdout(), tui.PaletteOptions{
		Style:   style,
		Entries: initial,
		Updates: updates,
		Merge:   merge,
		Trace:   a.env.Trace.Mark,
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

	err := a.herdrWhenFree(ctx, argv, runFirstDelay, runMaxAttempts)
	if err != nil {
		_, _ = herdr.Text(ctx, a.env.Runner, "", []string{"notification", "show", "Command palette", "--body", label + " failed: " + err.Error()})
	}
	return err
}

// herdrWhenFree ejecuta un comando de herdr y, mientras herdr conteste que está
// ocupado con un popup, lo reintenta hasta attempts veces. first es la espera
// antes del primer intento (0 = de inmediato).
func (a *app) herdrWhenFree(ctx context.Context, argv []string, first time.Duration, attempts int) error {
	delay := first
	var err error
	for range attempts {
		if delay > 0 {
			if err = a.sleepFor(ctx, delay); err != nil {
				return err
			}
		}
		_, err = herdr.Text(ctx, a.env.Runner, "", argv)
		if !isUIBusy(err) {
			return err
		}
		delay = busyRetryDelay
	}
	return err
}

// isUIBusy dice si herdr rechazó el comando porque hay un popup abierto. El CLI
// de herdr imprime ese error como JSON por stderr y sale con código 1, así que
// además del sobre de stdout hay que mirar el stderr del proceso.
func isUIBusy(err error) bool {
	if err == nil {
		return false
	}
	var api *herdr.APIError
	if errors.As(err, &api) {
		return api.Code == uiBusyCode
	}
	var exit *herdr.ExitError
	return errors.As(err, &exit) && strings.Contains(exit.Stderr, `"`+uiBusyCode+`"`)
}

func (a *app) sleepFor(ctx context.Context, d time.Duration) error {
	if a.env.Sleep != nil {
		return a.env.Sleep(ctx, d)
	}
	return sleepContext(ctx, d)
}

// frameMarker es un texto que solo aparece en el primer cuadro dibujado de la paleta.
var frameMarker = []byte("Command palette")

// tracedStdout devuelve la salida de la paleta. Con la traza apagada es la
// salida tal cual. Encendida, la envuelve para anotar cuándo se escribe el primer
// cuadro en la terminal, que es lo que de verdad ve la persona: cuando la salida
// es un archivo se conserva como tal (Bubble Tea lo necesita para conocer el
// tamaño de la ventana).
func (a *app) tracedStdout() io.Writer {
	if a.env.Trace == nil {
		return a.env.Stdout
	}
	const name = "palette: first frame written to the terminal"
	var once sync.Once
	onFrame := func() {
		once.Do(func() {
			if t0, ok := trace.ParseTrigger(a.getenv(trace.TriggerEnv)); ok {
				a.env.Trace.MarkSince(name, t0)
				return
			}
			a.env.Trace.Mark(name)
		})
	}
	if f, ok := a.env.Stdout.(*os.File); ok {
		return &frameFile{File: f, onFrame: onFrame}
	}
	return &frameWriter{Writer: a.env.Stdout, onFrame: onFrame}
}

// frameFile es un *os.File que avisa al escribir el primer cuadro. Al incrustar
// el archivo conserva Fd, Read y Close, que Bubble Tea usa para detectar la terminal.
type frameFile struct {
	*os.File
	onFrame func()
}

func (f *frameFile) Write(p []byte) (int, error) {
	n, err := f.File.Write(p)
	if err == nil && bytes.Contains(p, frameMarker) {
		f.onFrame()
	}
	return n, err
}

// frameWriter es lo mismo para una salida que no es un archivo.
type frameWriter struct {
	io.Writer
	onFrame func()
}

func (f *frameWriter) Write(p []byte) (int, error) {
	n, err := f.Writer.Write(p)
	if err == nil && bytes.Contains(p, frameMarker) {
		f.onFrame()
	}
	return n, err
}
