// Comando hum: interfaz amigable para el CLI de herdr.
package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/trace"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

func main() {
	start := processStart()
	tracer, closeTrace := newTracer(os.Args[1:], start)
	tracer.Mark("main")

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
		Trace:         tracer,
		CheckConfig:   checkConfig,
		Version:       versionLine(),
	}
	if terminal {
		ui.EnableVT(os.Stdout)
		env.Width = ui.Width(os.Stdout, os.Getenv)
		env.Height = ui.Height(os.Stdout)
	}

	tracer.Mark("environment ready")
	code := cli.Run(ctx, env, os.Args[1:])
	tracer.Mark("exit")
	closeTrace()

	stop()
	os.Exit(code)
}

// newTracer abre la traza si está encendida (HUM_TRACE o `hum trace on`). Nunca
// falla: si no se puede abrir el archivo, simplemente no hay traza. No traza el
// propio `hum trace` ni el autocompletado, que se llama en cada pulsación.
func newTracer(args []string, start time.Time) (*trace.Tracer, func()) {
	nothing := func() {}
	if len(args) > 0 && (args[0] == "trace" || args[0] == "__complete") {
		return nil, nothing
	}
	dir := cacheDir()
	if !trace.Enabled(os.Getenv, dir) {
		return nil, nothing
	}
	f, err := trace.Open(trace.LogPath(os.Getenv, dir))
	if err != nil {
		return nil, nothing
	}
	return trace.New(f, traceLabel(args, trace.ContentEnabled(dir)), start), func() { _ = f.Close() }
}

// traceLabel solo incluye argumentos libres cuando se ha pedido --content.
// Incluso un comando desconocido puede ser texto privado: no se copia al log.
func traceLabel(args []string, content bool) string {
	if content {
		return strings.Join(args, " ")
	}
	if len(args) == 0 {
		return "hum"
	}
	switch args[0] {
	case "palette":
		if len(args) > 1 && args[1] == "open" {
			return "palette open"
		}
		return "palette"
	case "help", "version", "ls", "id", "say", "ask", "attention", "pick",
		"new", "completion", "look", "name", "rename", "close", "worktree",
		"alias", "go", "wait", "machines", "popup", "keys":
		return args[0]
	default:
		return "hum"
	}
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
