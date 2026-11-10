package cli

import (
	"context"
	"errors"

	"github.com/SantosJMM/herdr-human/internal/tui"
)

// homeID es el id de la acción y del pane de la pantalla de inicio en el manifest.
const homeID = "home"

// home despacha `hum home [--embedded | open]`.
func (a *app) home(ctx context.Context, args []string) error {
	switch {
	case len(args) == 0:
		return a.homeUI(ctx, false)
	case len(args) == 1 && args[0] == "--embedded":
		// Dentro de un popup de herdr, que ya dibuja el marco y el título.
		return a.homeUI(ctx, true)
	case len(args) == 1 && args[0] == "open":
		// Es lo que ejecuta el atajo: abre el popup y nada más.
		return a.openPane(ctx, homeID)
	}
	return usagef("usage: hum home [--embedded | open]")
}

// homeUI dibuja la pantalla de inicio.
func (a *app) homeUI(ctx context.Context, embedded bool) error {
	if !a.interactive() {
		return withHint(errors.New("the home screen needs an interactive terminal"), `open it with the herdr key bound to the "home" action`)
	}
	style, err := a.newStyle("")
	if err != nil {
		return err
	}
	a.env.Trace.Mark("home: settings ready")
	err = tui.RunHome(ctx, a.env.Stdin, a.env.Stdout, tui.HomeOptions{
		Style:    style,
		Tabs:     a.homeTabs(),
		Embedded: embedded,
		Trace:    a.env.Trace.Mark,
	})
	if err != nil {
		return err
	}
	return nil
}

// homeTabs son las pestañas de la pantalla de inicio, en su orden. Las secciones
// que aún no están hechas muestran un aviso.
func (a *app) homeTabs() []tui.Tab {
	soon := "Not available yet."
	return []tui.Tab{
		tui.NewTextTab("Agents", soon),
		tui.NewTextTab("Attention", soon),
		tui.NewTextTab("Machines", soon),
		tui.NewTextTab("Commands", soon),
	}
}
