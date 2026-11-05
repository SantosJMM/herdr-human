// Comando hum: interfaz amigable para el CLI de herdr.
package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

func main() {
	// Ctrl+C cancela las consultas en curso en vez de dejarlas colgadas.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	terminal := isTerminal(os.Stdout)
	env := cli.Env{
		Runner: herdr.NewExecRunner(),
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		// Una terminal es un dispositivo de caracteres; una tubería o un archivo no.
		Terminal:      terminal,
		StdinTerminal: isTerminal(os.Stdin),
		Getenv:        os.Getenv,
		GOOS:          runtime.GOOS,
		Theme:         ui.LoadThemeFile(runtime.GOOS, os.Getenv),
		Copy:          copyToClipboard,
		Getwd:         os.Getwd,
		CacheDir:      cacheDir(),
		ConfigDir:     configDir(os.Getenv),
		Detach:        detach,
	}
	if terminal {
		ui.EnableVT(os.Stdout)
		env.Width = ui.Width(os.Stdout, os.Getenv)
		env.Height = ui.Height(os.Stdout)
	}

	code := cli.Run(ctx, env, os.Args[1:])

	stop()
	os.Exit(code)
}

// cacheDir es la carpeta de caché del usuario para hum; vacía (sin caché) si el
// sistema no tiene una.
func cacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, "herdr-human")
}

// configDir es la carpeta de configuración de hum: la que herdr da al plugin
// (HERDR_PLUGIN_CONFIG_DIR) o, si no está, la del usuario. Vacía si no hay.
func configDir(getenv func(string) string) string {
	if dir := getenv("HERDR_PLUGIN_CONFIG_DIR"); dir != "" {
		return dir
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, "herdr-human")
}

// isTerminal indica si f es una terminal interactiva y no una tubería o un archivo.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
