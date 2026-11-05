package cli

import (
	"context"
	"slices"

	"github.com/SantosJMM/herdr-human/internal/tui"
)

// popupPanes son los panes del manifest que se abren con `hum popup <nombre>`.
var popupPanes = []string{"overview", "machines", "pick", "attention"}

// popupShown son los comandos cuyo resultado se muestra en un popup y se deja
// leer hasta que la persona pulsa una tecla.
var popupShown = []string{"ls", "attention"}

// popup despacha `hum popup`. Existe para que el manifest arranque hum
// directamente, sin intérprete de por medio:
//
//	hum popup <overview|machines|pick|attention>   abre ese pane (lo usan las acciones)
//	hum popup show <ls|attention>                  lo usan los panes que solo muestran texto
func (a *app) popup(ctx context.Context, args []string) error {
	switch {
	case len(args) == 1 && slices.Contains(popupPanes, args[0]):
		return a.openPane(ctx, args[0])
	case len(args) == 2 && args[0] == "show" && slices.Contains(popupShown, args[1]):
		return a.popupShow(ctx, args[1])
	}
	return usagef("usage: hum popup <overview|machines|pick|attention> | hum popup show <ls|attention>")
}

// popupShow ejecuta un comando y, en una terminal, espera una tecla antes de
// terminar: el popup se cierra cuando el proceso termina, y sin la espera el
// resultado desaparecería antes de poder leerlo.
func (a *app) popupShow(ctx context.Context, command string) error {
	err := a.dispatch(ctx, []string{command})
	if a.interactive() {
		if _, werr := a.env.Stdout.Write([]byte("\n")); werr == nil {
			_ = tui.WaitKey(ctx, a.env.Stdin, a.env.Stdout, "Press any key to close")
		}
	}
	return err
}
