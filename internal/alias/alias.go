// Paquete alias guarda atajos de nombre a agente ("docs" -> "odoous/odoo-dev")
// en un archivo JSON de la carpeta de configuración del usuario.
package alias

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/SantosJMM/herdr-human/internal/actions"
)

const (
	// maxFile acota lo que se lee del archivo: es de la persona, pero un archivo
	// enorme o equivocado no debe agotar la memoria.
	maxFile = 64 << 10
	// maxAliases acota el número de alias guardados.
	maxAliases = 500
	// maxTarget acota la longitud de lo que apunta un alias.
	maxTarget = 200
	version   = 1
)

var (
	// ErrNotFound indica que el alias no existe.
	ErrNotFound = errors.New("alias not found")
	// ErrInvalidName indica un nombre de alias con formato inválido.
	ErrInvalidName = errors.New("alias name must use lowercase letters, digits, - or _, up to 32 characters")
	// ErrInvalidTarget indica un destino de alias inválido.
	ErrInvalidTarget = errors.New("alias target must be text without control characters, up to 200 characters, and must not start with \"-\"")
	// ErrNoStore indica que no hay carpeta de configuración donde guardar.
	ErrNoStore = errors.New("there is no configuration folder to keep aliases in")
	// ErrTooMany indica que se llegó al máximo de alias.
	ErrTooMany = errors.New("too many aliases")
)

// Store es el archivo de alias. El valor cero (sin Path) no guarda nada.
type Store struct {
	Path string
}

type file struct {
	Version int               `json:"version"`
	Aliases map[string]string `json:"aliases"`
}

// List devuelve todos los alias; un archivo que no existe es una lista vacía.
// Un archivo ilegible o corrupto es un error: nunca se interpreta a medias.
func (s Store) List() (map[string]string, error) {
	out := map[string]string{}
	if s.Path == "" {
		return out, nil
	}
	f, err := os.Open(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFile {
		return nil, fmt.Errorf("%s: file is larger than %d bytes", s.Path, maxFile)
	}
	var parsed file
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("%s: %w", s.Path, err)
	}
	for name, target := range parsed.Aliases {
		// Lo que no cumple las reglas se descarta: el archivo lo puede haber
		// editado una persona o venir de otra versión.
		if validName(name) == nil && validTarget(target) == nil {
			out[name] = target
		}
	}
	return out, nil
}

// Lookup busca un alias. Cualquier error (archivo ausente o corrupto) cuenta
// como "no hay alias": resolver un nombre nunca debe fallar por esto.
func (s Store) Lookup(name string) (string, bool) {
	all, err := s.List()
	if err != nil {
		return "", false
	}
	target, ok := all[name]
	return target, ok
}

func validName(name string) error {
	// Un nombre que empieza por "-" se leería como una opción de la línea de comandos.
	if actions.ValidName(name) != nil || strings.HasPrefix(name, "-") {
		return fmt.Errorf("%q: %w", name, ErrInvalidName)
	}
	return nil
}

func validTarget(target string) error {
	if strings.TrimSpace(target) == "" || len(target) > maxTarget ||
		strings.HasPrefix(strings.TrimSpace(target), "-") || strings.ContainsFunc(target, unicode.IsControl) {
		return fmt.Errorf("%q: %w", target, ErrInvalidTarget)
	}
	return nil
}

// Set crea o reemplaza un alias.
func (s Store) Set(name, target string) error {
	if err := validName(name); err != nil {
		return err
	}
	if err := validTarget(target); err != nil {
		return err
	}
	if s.Path == "" {
		return ErrNoStore
	}
	all, err := s.List()
	if err != nil {
		return err
	}
	if _, exists := all[name]; !exists && len(all) >= maxAliases {
		return ErrTooMany
	}
	all[name] = strings.TrimSpace(target)
	return s.save(all)
}

// Remove borra un alias.
func (s Store) Remove(name string) error {
	if s.Path == "" {
		return ErrNoStore
	}
	all, err := s.List()
	if err != nil {
		return err
	}
	if _, ok := all[name]; !ok {
		return fmt.Errorf("%q: %w", name, ErrNotFound)
	}
	delete(all, name)
	return s.save(all)
}

// save escribe el archivo de forma atómica y privada (0600 en una carpeta 0700).
func (s Store) save(all map[string]string) error {
	data, err := json.MarshalIndent(file{Version: version, Aliases: all}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "aliases-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil && !errors.Is(err, os.ErrPermission) {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), s.Path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}
