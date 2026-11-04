// Paquete herdrtest ofrece un Runner falso para probar sin ejecutar herdr.
package herdrtest

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Call registra una invocación recibida por el Fake.
type Call struct {
	Machine string
	Args    []string
}

type reply struct {
	out  string
	err  error
	hang bool
	// seq, si no es nil, entrega una salida distinta en cada llamada; la
	// última se repite. Sirve para simular un estado que cambia entre lecturas.
	seq  []string
	next int
}

// Fake implementa herdr.Runner con respuestas programadas.
// Es seguro para uso concurrente.
type Fake struct {
	mu      sync.Mutex
	replies map[string]reply
	calls   []Call
}

// New crea un Fake vacío; toda llamada sin respuesta programada falla.
func New() *Fake {
	return &Fake{replies: make(map[string]reply)}
}

func key(machine string, args []string) string {
	return machine + "\x00" + strings.Join(args, "\x1f")
}

// Reply programa la salida estándar para una invocación exacta.
func (f *Fake) Reply(machine string, args []string, out string) {
	f.set(machine, args, reply{out: out})
}

// Sequence programa salidas sucesivas: la primera llamada recibe outs[0], la
// segunda outs[1], y así hasta repetir la última.
func (f *Fake) Sequence(machine string, args []string, outs ...string) {
	f.set(machine, args, reply{seq: outs})
}

// Fail programa un error del runner para una invocación exacta.
func (f *Fake) Fail(machine string, args []string, err error) {
	f.set(machine, args, reply{err: err})
}

// Hang hace que la invocación espere hasta que su contexto termine.
func (f *Fake) Hang(machine string, args []string) {
	f.set(machine, args, reply{hang: true})
}

func (f *Fake) set(machine string, args []string, r reply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies[key(machine, args)] = r
}

// Calls devuelve una copia de las invocaciones recibidas, en orden de llegada.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Run implementa herdr.Runner.
func (f *Fake) Run(ctx context.Context, machine string, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, Call{Machine: machine, Args: append([]string(nil), args...)})
	k := key(machine, args)
	r, ok := f.replies[k]
	if ok && len(r.seq) > 0 {
		r.out = r.seq[min(r.next, len(r.seq)-1)]
		stored := f.replies[k]
		stored.next++
		f.replies[k] = stored
	}
	f.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("herdrtest: unexpected call machine=%q args=%q", machine, args)
	}
	if r.hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return []byte(r.out), r.err
}
