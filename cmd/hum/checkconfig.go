package main

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/SantosJMM/herdr-human/internal/herdr"
)

// checkConfig ejecuta `herdr config check` sobre el archivo dado (HERDR_CONFIG_PATH
// hace que herdr lea ese archivo en vez del suyo) y devuelve lo que imprime.
// herdr sale con código 1 cuando encuentra problemas y los cuenta por la salida
// estándar; eso no es un fallo al ejecutarlo, es el resultado.
func checkConfig(ctx context.Context, path string) (string, error) {
	bin := os.Getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = "herdr"
	}
	cmd := exec.CommandContext(ctx, bin, "config", "check")
	cmd.Env = append(os.Environ(), "HERDR_CONFIG_PATH="+path)
	herdr.HideConsole(cmd)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if err != nil && errors.As(err, &exit) && len(out) > 0 {
		return string(out), nil
	}
	return string(out), err
}
