// Paquete trace escribe una traza de tiempos opcional para medir lo que tardan
// los popups en dibujar su primer cuadro. Está apagada salvo que se pida, no
// cuesta nada apagada y nunca puede romper el comando que mide.
//
// Se activa con la variable HUM_TRACE o con el archivo marcador que crea
// `hum trace on`. La variable sola no basta para los popups: heredan el entorno
// del servidor de herdr, no el de la terminal desde donde se lanzó hum.
package trace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// logName y markerName son los archivos dentro de la carpeta de caché de hum.
	logName    = "trace.log"
	markerName = "trace.on"
	// maxLogBytes es el tamaño a partir del cual se empieza un archivo nuevo.
	maxLogBytes = 256 << 10
)

// Tracer anota marcas con su hora y el tiempo desde el arranque del proceso.
// El valor nil es válido y no hace nada.
type Tracer struct {
	mu    sync.Mutex
	w     io.Writer
	label string
	pid   int
	start time.Time
	now   func() time.Time
}

// New crea un Tracer que escribe en w. start es el instante de arranque del
// proceso (o el más temprano que se conozca); label identifica el comando.
func New(w io.Writer, label string, start time.Time) *Tracer {
	return &Tracer{w: w, label: label, pid: os.Getpid(), start: start, now: time.Now}
}

// Mark anota que se llegó a name. Cada línea lleva la hora absoluta, para poder
// cruzar las trazas de procesos distintos (la acción y su popup).
func (t *Tracer) Mark(name string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	fmt.Fprintf(t.w, "%s pid=%d %s +%.1fms %s\n",
		now.UTC().Format("2006-01-02T15:04:05.000Z"), t.pid, t.label, ms(now.Sub(t.start)), name)
}

// MarkSince es como Mark pero añade el tiempo desde un instante anterior, que
// puede venir de otro proceso (el disparo de la acción).
func (t *Tracer) MarkSince(name string, from time.Time) {
	if t == nil {
		return
	}
	t.Mark(fmt.Sprintf("%s (%.1fms since the trigger)", name, ms(t.now().Sub(from))))
}

// Start devuelve el instante de arranque del proceso; con un Tracer nil, el cero.
func (t *Tracer) Start() time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.start
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// Enabled dice si la traza está encendida: HUM_TRACE con un valor que no sea
// vacío ni "0", o el archivo marcador en dir.
func Enabled(getenv func(string) string, dir string) bool {
	if v := getenv("HUM_TRACE"); v != "" && v != "0" {
		return true
	}
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, markerName))
	return err == nil
}

// LogPath es el archivo de la traza. Si HUM_TRACE parece una ruta (lleva un
// separador), esa ruta; si no, trace.log dentro de dir.
func LogPath(getenv func(string) string, dir string) string {
	if v := getenv("HUM_TRACE"); strings.ContainsAny(v, `/\`) {
		return v
	}
	return filepath.Join(dir, logName)
}

// SetEnabled enciende o apaga la traza persistente con el archivo marcador.
func SetEnabled(dir string, on bool) error {
	marker := filepath.Join(dir, markerName)
	if !on {
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(marker, []byte("1\n"), 0o600)
}

// Open abre el archivo de la traza para añadir líneas, empezando de cero si se
// hizo muy grande. Quien lo llama debe cerrar el resultado.
func Open(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_APPEND
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogBytes {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	return os.OpenFile(path, flags, 0o600)
}

// Tail devuelve las últimas n líneas del archivo de la traza.
func Tail(path string, n int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// TriggerEnv es la variable por la que `hum palette open` le pasa al popup el
// instante del disparo, en milisegundos Unix, solo cuando la traza está activa.
const TriggerEnv = "HUM_TRACE_T0"

// FormatTrigger escribe un instante para TriggerEnv.
func FormatTrigger(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }

// ParseTrigger lee un TriggerEnv; ok es false si falta o no es un número.
func ParseTrigger(s string) (t time.Time, ok bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(n), true
}
