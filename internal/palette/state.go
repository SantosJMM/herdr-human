package palette

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Nombres de los archivos de estado y límites.
const (
	recentsFile = "recents.json"
	cacheFile   = "plugins-cache.json"
	maxRecents  = 50
	// maxStateBytes limita lo que se lee de un archivo de estado: un archivo
	// ajeno o corrupto y enorme no debe frenar la apertura de la paleta.
	maxStateBytes = 1 << 20
)

// StateDir es la carpeta de estado del plugin; sin herdr usa una carpeta temporal.
func StateDir(getenv func(string) string) string {
	if dir := getenv("HERDR_PLUGIN_STATE_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), "herdr-human-palette")
}

func readState(dir, name string, into any) bool {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxStateBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxStateBytes {
		return false
	}
	return json.Unmarshal(data, into) == nil
}

// writeState escribe en un archivo temporal y lo renombra, para que una
// interrupción no deje un archivo a medias. Un fallo solo cuesta el orden o la
// caché, así que no se propaga.
func writeState(dir, name string, v any) {
	data, err := json.Marshal(v)
	if err != nil || os.MkdirAll(dir, 0o700) != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, name+".*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), filepath.Join(dir, name)) != nil {
		_ = os.Remove(tmp.Name())
	}
}

// Recents lleva el orden de uso de las entradas, la más reciente primero.
type Recents struct{ ids []string }

// LoadRecents lee los recientes; cualquier fallo se traduce en "sin recientes".
func LoadRecents(dir string) Recents {
	var ids []string
	if !readState(dir, recentsFile, &ids) {
		return Recents{}
	}
	return Recents{ids: slices.DeleteFunc(ids, func(s string) bool { return s == "" })}
}

// Rank devuelve la posición de id; lo que nunca se usó va después de todo lo reciente.
func (r Recents) Rank(id string) int {
	if i := slices.Index(r.ids, id); i >= 0 {
		return i
	}
	return maxRecents + 1
}

// Record pone id al frente y guarda la lista.
func (r Recents) Record(dir, id string) Recents {
	if id == "" {
		return r
	}
	ids := append([]string{id}, slices.DeleteFunc(slices.Clone(r.ids), func(s string) bool { return s == id })...)
	if len(ids) > maxRecents {
		ids = ids[:maxRecents]
	}
	writeState(dir, recentsFile, ids)
	return Recents{ids: ids}
}

// Merge junta las fuentes, pone el atajo a cada entrada y ordena: primero los
// recientes y luego por origen y título, para que el orden sea predecible.
func Merge(chords map[string]string, recents Recents, sources ...[]Entry) []Entry {
	var all []Entry
	for _, s := range sources {
		for _, e := range s {
			e.Chord = chords[e.ID]
			all = append(all, e)
		}
	}
	slices.SortStableFunc(all, func(a, b Entry) int {
		if d := recents.Rank(a.ID) - recents.Rank(b.ID); d != 0 {
			return d
		}
		if c := strings.Compare(strings.ToLower(a.Source), strings.ToLower(b.Source)); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	})
	return all
}

// LoadCache lee las entradas de plugins guardadas en la apertura anterior. Sirven
// para pintar la paleta completa al instante mientras herdr responde.
func LoadCache(dir string) []Entry {
	var entries []Entry
	if !readState(dir, cacheFile, &entries) {
		return nil
	}
	return slices.DeleteFunc(entries, func(e Entry) bool {
		return e.Kind != KindPlugin || e.PluginID == "" || e.ActionID == "" || e.ID == ""
	})
}

// SaveCache guarda las entradas de plugins para la próxima apertura.
func SaveCache(dir string, entries []Entry) { writeState(dir, cacheFile, entries) }
