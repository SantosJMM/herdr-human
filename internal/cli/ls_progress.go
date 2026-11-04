package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// errPending marca, en el dibujo, una máquina que todavía no ha respondido.
var errPending = errors.New("no answer yet")

const (
	// redrawEvery es cada cuánto se refresca el indicador de espera.
	redrawEvery = 100 * time.Millisecond
	// graceDelay es lo que se espera antes del primer dibujo: lo que responde en
	// ese tiempo se percibe como instantáneo y no merece un dibujo intermedio.
	graceDelay = 150 * time.Millisecond
)

// canRedraw dice si ls puede dibujar el árbol en su sitio mientras llegan las
// máquinas: hace falta una terminal de la que se conozca la altura, y que no se
// pida JSON. Con una tubería la salida es la de siempre, sin secuencias.
func (a *app) canRedraw(cf commonFlags) bool {
	return a.env.Terminal && a.env.Height > 0 && !cf.json
}

// screen redibuja un bloque de líneas en el mismo sitio de la terminal. Solo es
// seguro si el bloque cabe en la pantalla: si deja de caber, deja de dibujar (y
// borra lo dibujado) y el resultado final se imprime una vez al terminar.
type screen struct {
	w      io.Writer
	height int
	drawn  int  // líneas del último dibujo, que siguen en pantalla
	off    bool // dejó de redibujar porque no cabía
}

// erase devuelve la secuencia que sube al principio del último dibujo y borra
// hasta el final de la pantalla; vacía si no hay nada dibujado.
func (s *screen) erase() string {
	if s.drawn == 0 {
		return ""
	}
	seq := fmt.Sprintf("\x1b[%dA\r\x1b[J", s.drawn)
	s.drawn = 0
	return seq
}

// draw sustituye el dibujo anterior por lines, de una sola escritura para que no parpadee.
func (s *screen) draw(lines []string) {
	if s.off {
		return
	}
	if len(lines) >= s.height {
		fmt.Fprint(s.w, s.erase())
		s.off = true
		return
	}
	s.write(lines)
}

// finish deja en pantalla el resultado definitivo, sustituyendo el último dibujo.
func (s *screen) finish(lines []string) {
	s.write(lines)
	s.drawn = 0
}

func (s *screen) write(lines []string) {
	var sb strings.Builder
	sb.WriteString(s.erase())
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	fmt.Fprint(s.w, sb.String())
	s.drawn = len(lines)
}

// lsProgressive consulta las máquinas y dibuja el árbol en su sitio a medida que
// responden (lo local al instante y lo cacheado también), con las pendientes
// como "connecting". Al terminar deja el mismo árbol que el camino normal y lo
// devuelve; un fallo local lo borra y se devuelve como error.
func (a *app) lsProgressive(ctx context.Context, cf commonFlags, st *ui.Style, full bool) (inventory.Inventory, error) {
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := inventory.StartCached(sctx, a.env.Runner, a.options(cf, ""), a.cacheFor(cf))
	if err != nil {
		return inventory.Inventory{}, err
	}

	scr := &screen{w: a.env.Stdout, height: a.env.Height}
	got := make(map[string]inventory.Update, len(stream.Machines))
	start := time.Now()
	ticker := time.NewTicker(redrawEvery)
	defer ticker.Stop()

	for len(got) < len(stream.Machines) {
		select {
		case u, ok := <-stream.Updates:
			if !ok {
				return inventory.Inventory{}, errors.New("the inventory stream closed early")
			}
			got[u.Machine] = u
			// Se recoge todo lo que ya esté disponible antes de dibujar.
			for more := true; more; {
				select {
				case u, ok = <-stream.Updates:
					if ok {
						got[u.Machine] = u
					}
					more = ok
				default:
					more = false
				}
			}
		case <-ticker.C:
		case <-ctx.Done():
			fmt.Fprint(a.env.Stdout, scr.erase())
			return inventory.Inventory{}, ctx.Err()
		}

		if local, ok := got[""]; ok && local.Err != nil {
			fmt.Fprint(a.env.Stdout, scr.erase())
			return inventory.Inventory{}, local.Err
		}
		// Hasta pasado el margen no se dibuja nada: si todo responde enseguida, se
		// imprime una sola vez y sin parpadeo.
		if len(got) < len(stream.Machines) && time.Since(start) >= graceDelay {
			scr.draw(a.treeLines(partialInventory(stream.Machines, got, time.Since(start)), st, full))
		}
	}

	inv, err := inventory.Assemble(stream.Machines, got)
	if err != nil {
		fmt.Fprint(a.env.Stdout, scr.erase())
		return inv, err
	}
	scr.finish(a.treeLines(inv, st, full))
	return inv, nil
}

// partialInventory arma lo que ya se sabe: las máquinas que respondieron con sus
// agentes y el resto como pendientes, para dibujarlas con su estado.
func partialInventory(machines []string, got map[string]inventory.Update, elapsed time.Duration) inventory.Inventory {
	var inv inventory.Inventory
	for _, machine := range machines {
		u, ok := got[machine]
		if !ok {
			inv.Timings = append(inv.Timings, inventory.Timing{Machine: machine, Elapsed: elapsed, Err: errPending})
			continue
		}
		inv.Timings = append(inv.Timings, inventory.Timing{
			Machine: machine, Elapsed: u.Elapsed, Err: u.Err, Cached: u.Cached, Age: u.Age,
		})
		if u.Err == nil {
			inv.Targets = append(inv.Targets, u.Targets...)
		}
	}
	return inv
}

// warn avisa por stderr de las máquinas que no respondieron.
func (a *app) warn(inv inventory.Inventory) {
	for _, w := range inv.Warnings {
		fmt.Fprintf(a.env.Stderr, "warning: %v\n", w)
	}
}
