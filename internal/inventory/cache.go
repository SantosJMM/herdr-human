package inventory

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	// DefaultCacheTTL es cuánto vive la respuesta de una máquina remota.
	DefaultCacheTTL = 10 * time.Second

	cacheVersion = 1
	// maxCacheFile acota lo que se lee del archivo: más grande no es una caché de fiar.
	maxCacheFile = 8 << 20
	// cacheMaxAge es la edad a partir de la cual una entrada se poda al guardar.
	cacheMaxAge = time.Hour
)

// Cache guarda en disco la última respuesta de cada máquina remota para no
// repetir la consulta (~1,3 s por SSH) si se pide otra vez enseguida. El
// servidor local nunca se cachea: responde en milisegundos y sus estados
// cambian a cada rato. Todo es de mejor esfuerzo: un archivo ilegible, de otra
// versión o demasiado grande equivale a no tener caché, y un fallo al guardar
// no es un error. Es seguro para varias goroutines.
type Cache struct {
	// Path es el archivo; vacío desactiva la caché.
	Path string
	// TTL es la vida de una entrada; cero usa DefaultCacheTTL.
	TTL time.Duration
	// Now es el reloj; nulo usa time.Now.
	Now func() time.Time
	// Bypass ignora lo guardado pero sigue escribiendo lo que se consulte
	// (para --fresh: no se confía en la caché, pero se renueva).
	Bypass bool

	mu      sync.Mutex
	loaded  bool
	dirty   bool
	entries map[string]cacheEntry
}

type cacheEntry struct {
	At      time.Time `json:"at"`
	Targets []Target  `json:"targets"`
}

type cacheFile struct {
	Version  int                   `json:"version"`
	Machines map[string]cacheEntry `json:"machines"`
}

func (c *Cache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Cache) ttl() time.Duration {
	if c.TTL > 0 {
		return c.TTL
	}
	return DefaultCacheTTL
}

// load lee el archivo una sola vez; cualquier problema deja la caché vacía.
func (c *Cache) load() {
	if c.loaded {
		return
	}
	c.loaded = true
	c.entries = map[string]cacheEntry{}
	if c.Path == "" {
		return
	}
	f, err := os.Open(c.Path)
	if err != nil {
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCacheFile+1))
	if err != nil || len(data) > maxCacheFile {
		return
	}
	var file cacheFile
	if json.Unmarshal(data, &file) != nil || file.Version != cacheVersion || file.Machines == nil {
		return
	}
	c.entries = file.Machines
}

// get devuelve los agentes guardados de una máquina remota si siguen vigentes.
func (c *Cache) get(machine string) ([]Target, time.Duration, bool) {
	if c == nil || c.Path == "" || machine == "" {
		return nil, 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	if c.Bypass {
		return nil, 0, false
	}
	e, ok := c.entries[machine]
	age := c.now().Sub(e.At)
	if !ok || age < 0 || age >= c.ttl() {
		return nil, 0, false
	}
	return e.Targets, age, true
}

// Peek devuelve los agentes remotos guardados sin mirar el TTL, con tal de que
// no tengan más de una hora: sirve al autocompletado, que prefiere un dato algo
// viejo a esperar la red. No consulta nada y no depende de Bypass.
func (c *Cache) Peek() []Target {
	if c == nil || c.Path == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()

	now := c.now()
	machines := make([]string, 0, len(c.entries))
	for machine := range c.entries {
		machines = append(machines, machine)
	}
	sort.Strings(machines) // orden estable entre ejecuciones

	var out []Target
	for _, machine := range machines {
		e := c.entries[machine]
		if age := now.Sub(e.At); machine != "" && age >= 0 && age <= cacheMaxAge {
			out = append(out, e.Targets...)
		}
	}
	return out
}

// put guarda la respuesta de una máquina remota; se escribe en disco con save.
func (c *Cache) put(machine string, targets []Target) {
	if c == nil || c.Path == "" || machine == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	c.entries[machine] = cacheEntry{At: c.now(), Targets: targets}
	c.dirty = true
}

// save escribe el archivo si algo cambió: poda lo muy viejo y lo hace de forma
// atómica (archivo temporal y renombrado) con permisos solo para el usuario.
func (c *Cache) save() error {
	if c == nil || c.Path == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty {
		return nil
	}
	now := c.now()
	for machine, e := range c.entries {
		if age := now.Sub(e.At); age > cacheMaxAge || age < -cacheMaxAge {
			delete(c.entries, machine)
		}
	}
	data, err := json.Marshal(cacheFile{Version: cacheVersion, Machines: c.entries})
	if err != nil {
		return err
	}
	dir := filepath.Dir(c.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "inventory-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), c.Path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	c.dirty = false
	return nil
}

// Invalidate borra la caché: se usa tras cambiar algo (crear o renombrar un
// agente) para que la siguiente consulta no sirva datos anteriores al cambio.
func (c *Cache) Invalidate() error {
	if c == nil || c.Path == "" {
		return nil
	}
	c.mu.Lock()
	c.entries, c.loaded, c.dirty = map[string]cacheEntry{}, true, false
	c.mu.Unlock()
	if err := os.Remove(c.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
