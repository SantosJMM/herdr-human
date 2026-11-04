// Paquete herdr envuelve el ejecutable de herdr: ejecuta comandos locales o
// en una máquina guardada y decodifica sus respuestas JSON.
package herdr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Runner ejecuta un comando de herdr y devuelve su salida estándar.
// Una machine vacía significa el servidor local. Ante un error, la
// implementación debe devolver igualmente la salida capturada, porque herdr
// reporta los errores de la API como JSON en stdout.
type Runner interface {
	Run(ctx context.Context, machine string, args ...string) ([]byte, error)
}

// waitDelay es cuánto se espera, tras cancelar, a que se cierren las tuberías de
// herdr antes de cortarlas a la fuerza.
const waitDelay = time.Second

// ExecRunner es el Runner real, basado en os/exec.
type ExecRunner struct {
	// Bin es la ruta o nombre del ejecutable de herdr.
	Bin string
}

// NewExecRunner usa HERDR_BIN_PATH si existe; si no, busca "herdr" en el PATH.
func NewExecRunner() *ExecRunner {
	bin := os.Getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = "herdr"
	}
	return &ExecRunner{Bin: bin}
}

// ExitError describe un fallo del proceso de herdr.
type ExitError struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *ExitError) Error() string {
	msg := fmt.Sprintf("herdr %s: %v", strings.Join(e.Args, " "), e.Err)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

func (e *ExitError) Unwrap() error { return e.Err }

// Run implementa Runner.
func (r *ExecRunner) Run(ctx context.Context, machine string, args ...string) ([]byte, error) {
	full := make([]string, 0, len(args)+2)
	if machine != "" {
		full = append(full, "--machine", machine)
	}
	full = append(full, args...)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, r.Bin, full...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Al vencer el plazo se mata herdr, pero un hijo suyo (ssh) puede seguir con
	// las tuberías abiertas; sin esto Run esperaría a que termine por su cuenta y
	// el plazo por máquina no se respetaría.
	cmd.WaitDelay = waitDelay

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return stdout.Bytes(), &ExitError{
			Args:   full,
			Stderr: strings.TrimSpace(stderr.String()),
			Err:    err,
		}
	}
	return stdout.Bytes(), nil
}

// errNoResult indica que se esperaba un resultado y la respuesta no lo traía.
var errNoResult = errors.New("herdr response has no result")
