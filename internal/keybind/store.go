package keybind

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Store aplica cambios de atajos al config.toml de herdr con red de seguridad:
// valida antes de tocar el archivo, guarda un respaldo y lo escribe de una vez.
type Store struct {
	// Path es el config.toml.
	Path string
	// Check ejecuta `herdr config check` sobre el archivo dado y devuelve lo que
	// imprime. Sirve para validar un cambio en una copia antes de aplicarlo.
	Check func(ctx context.Context, path string) (string, error)
	// Reload pide al servidor de herdr que relea la configuración.
	Reload func(ctx context.Context) error
	// Now es el reloj (por defecto time.Now); decide el nombre del respaldo.
	Now func() time.Time
}

// Result cuenta lo que pasó al aplicar un cambio.
type Result struct {
	// Changed es false si el archivo ya estaba como se pedía.
	Changed bool
	// Backup es el respaldo del día; vacío si no hizo falta crearlo.
	Backup string
	// Reloaded es true si herdr recargó la configuración. Si no, ReloadErr dice
	// por qué: el archivo ya está guardado, solo falta recargarlo.
	Reloaded  bool
	ReloadErr error
	Conflicts []Conflict
}

// RejectedError es un cambio que `herdr config check` rechaza: no se aplicó.
type RejectedError struct{ Issues []string }

func (e *RejectedError) Error() string {
	return "herdr rejects this shortcut: " + strings.Join(e.Issues, "; ")
}

// ErrNoConfig se devuelve si no se sabe dónde está el config.toml.
var ErrNoConfig = errors.New("cannot find herdr's config.toml (set HERDR_CONFIG_PATH)")

// Read lee el config.toml; si no existe, es un archivo vacío.
func (s Store) Read() (*Config, error) {
	if s.Path == "" {
		return nil, ErrNoConfig
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return Parse(""), nil
		}
		return nil, err
	}
	return Parse(string(data)), nil
}

// Save enlaza key a b (cambia la tecla si ya tenía una) y lo aplica.
func (s Store) Save(ctx context.Context, b Binding, key, description string) (Result, error) {
	norm, err := Normalize(key)
	if err != nil {
		return Result{}, err
	}
	cfg, err := s.Read()
	if err != nil {
		return Result{}, err
	}
	next, err := cfg.Set(b, norm, description)
	if err != nil {
		return Result{}, err
	}
	res, err := s.apply(ctx, cfg.Text(), next)
	res.Conflicts = cfg.Conflicts(norm, b)
	return res, err
}

// Remove quita el atajo de b y lo aplica.
func (s Store) Remove(ctx context.Context, b Binding) (Result, error) {
	cfg, err := s.Read()
	if err != nil {
		return Result{}, err
	}
	next, err := cfg.Remove(b)
	if err != nil {
		return Result{}, err
	}
	return s.apply(ctx, cfg.Text(), next)
}

// apply valida el texto nuevo en una copia, y solo si herdr no encuentra
// problemas nuevos guarda el respaldo, escribe el archivo y pide recargar.
func (s Store) apply(ctx context.Context, oldText, newText string) (Result, error) {
	if newText == oldText {
		return Result{}, nil
	}
	if s.Check == nil {
		return Result{}, errors.New("cannot validate the change: no way to run `herdr config check`")
	}
	before, err := s.checkText(ctx, oldText)
	if err != nil {
		return Result{}, err
	}
	after, err := s.checkText(ctx, newText)
	if err != nil {
		return Result{}, err
	}
	if added := newIssues(before, after); len(added) > 0 {
		return Result{}, &RejectedError{Issues: added}
	}

	res := Result{Changed: true}
	if _, err := os.Stat(s.Path); err == nil {
		backup, err := s.backup(oldText)
		if err != nil {
			return Result{}, fmt.Errorf("saving a backup of config.toml: %w", err)
		}
		res.Backup = backup
	}
	if err := writeAtomic(s.Path, newText); err != nil {
		return Result{}, err
	}
	if s.Reload != nil {
		if res.ReloadErr = s.Reload(ctx); res.ReloadErr == nil {
			res.Reloaded = true
		}
	}
	return res, nil
}

// checkText ejecuta `herdr config check` sobre una copia temporal del texto.
func (s Store) checkText(ctx context.Context, text string) ([]string, error) {
	tmp, err := os.CreateTemp("", "hum-config-*.toml")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	out, err := s.Check(ctx, tmp.Name())
	if err != nil {
		return nil, fmt.Errorf("running `herdr config check`: %w", err)
	}
	return issues(out), nil
}

// backup guarda el config.toml tal como estaba antes del primer cambio del día; los
// cambios siguientes del mismo día no crean más copias.
func (s Store) backup(oldText string) (string, error) {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	name := s.Path + ".bak-hum-" + now().Format("20060102")
	if _, err := os.Stat(name); err == nil {
		return "", nil
	}
	return name, os.WriteFile(name, []byte(oldText), 0o600)
}

// writeAtomic escribe en un archivo temporal de la misma carpeta y lo renombra,
// para que una interrupción no deje el archivo a medias.
func writeAtomic(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".hum-*")
	if err != nil {
		return err
	}
	_, werr := tmp.WriteString(text)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

var indexInIssue = regexp.MustCompile(`\[\d+\]`)

// issues saca de la salida de `herdr config check` las líneas de problemas, sin
// la cabecera y con los índices ([10]) borrados: añadir un bloque desplaza los
// números de los demás y no debe contar como un problema nuevo.
func issues(output string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "config: ok" || line == "config: issues found" {
			continue
		}
		out = append(out, indexInIssue.ReplaceAllString(line, "[]"))
	}
	return out
}

// newIssues devuelve lo que after tiene de más respecto a before. Los problemas
// que el archivo ya tenía no son culpa del cambio y no lo bloquean.
func newIssues(before, after []string) []string {
	remaining := map[string]int{}
	for _, b := range before {
		remaining[b]++
	}
	var added []string
	for _, a := range after {
		if remaining[a] > 0 {
			remaining[a]--
			continue
		}
		added = append(added, a)
	}
	return added
}
