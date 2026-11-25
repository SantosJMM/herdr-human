package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/SantosJMM/herdr-human/internal/trace"
)

// traceTailLines es cuántas líneas muestra `hum trace show`.
const traceTailLines = 40

const traceUsage = "usage: hum trace on [--content] | off | show | clear"

// traceCmd despacha `hum trace on|off|show|clear`. La traza vive en la carpeta de
// caché de hum, que es la misma desde una terminal y desde un popup (a diferencia
// de las variables de entorno, que el popup hereda del servidor de herdr).
func (a *app) traceCmd(args []string) error {
	content := len(args) == 2 && args[0] == "on" && args[1] == "--content"
	if len(args) != 1 && !content {
		return usagef(traceUsage)
	}
	dir := a.env.CacheDir
	if dir == "" {
		return errors.New("this system has no cache folder for the trace")
	}
	log := trace.LogPath(a.getenv, dir)

	switch args[0] {
	case "on":
		var err error
		if content {
			err = trace.SetContentEnabled(dir)
		} else {
			err = trace.SetEnabled(dir, true)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(a.env.Stdout, "trace on; popups and commands now append their timings to %s\n", log)
		if content {
			fmt.Fprintln(a.env.Stdout, "WARNING: --content records full CLI arguments, including messages and secrets. Use synthetic data; run hum trace on to stop recording content. Existing log entries are kept.")
		}
	case "off":
		if err := trace.SetEnabled(dir, false); err != nil {
			return err
		}
		fmt.Fprintln(a.env.Stdout, "trace off (the log is kept; \"hum trace clear\" deletes it)")
	case "clear":
		if err := os.Remove(log); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Fprintln(a.env.Stdout, "trace log deleted")
	case "show":
		state := "off"
		if trace.Enabled(a.getenv, dir) {
			state = "on"
			if trace.ContentEnabled(dir) {
				state = "on (with content; may contain secrets)"
			}
		}
		text, err := trace.Tail(log, traceTailLines)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(a.env.Stdout, "trace %s; no log yet (%s). Run \"hum trace on\", open a popup and try again.\n", state, log)
				return nil
			}
			return err
		}
		fmt.Fprintf(a.env.Stdout, "trace %s; last lines of %s:\n%s", state, log, text)
	default:
		return usagef(traceUsage)
	}
	return nil
}
