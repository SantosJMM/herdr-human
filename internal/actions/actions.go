// Paquete actions implementa las operaciones de alto nivel sobre un agente
// (hablarle, leerlo, renombrarlo, enfocarlo) traduciéndolas a comandos de herdr.
package actions

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/inventory"
)

// ErrInvalidName indica que el nombre no cumple el formato que acepta herdr-human.
var ErrInvalidName = errors.New("name must use lowercase letters, digits, - or _, up to 32 characters")

var namePattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// Service agrupa las acciones; solo depende de un Runner.
type Service struct {
	Runner herdr.Runner
}

// SayOptions modifica el envío de un mensaje.
type SayOptions struct {
	// Wait espera a que el agente termine de procesar el mensaje.
	Wait bool
	// TimeoutMillis acota la espera; cero deja el valor por defecto de herdr.
	TimeoutMillis int
}

// Say envía un mensaje al agente como si lo escribiera una persona.
func (s Service) Say(ctx context.Context, t inventory.Target, text string, opts SayOptions) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("message is empty")
	}

	// herdr toma como texto el argumento que sigue al pane id aunque empiece
	// por "-" (y no admite "--"), así que el texto va primero y las opciones
	// después. Verificado contra herdr 0.9.4.
	args := []string{"agent", "prompt", t.PaneID, text}
	if opts.Wait {
		args = append(args, "--wait")
		if opts.TimeoutMillis > 0 {
			args = append(args, "--timeout", strconv.Itoa(opts.TimeoutMillis))
		}
	}
	return herdr.Call(ctx, s.Runner, t.Machine, args, nil)
}

// Look devuelve la salida reciente del terminal del agente.
// lines <= 0 deja el valor por defecto de herdr.
func (s Service) Look(ctx context.Context, t inventory.Target, lines int) (string, error) {
	args := []string{"agent", "read", t.PaneID}
	if lines > 0 {
		args = append(args, "--lines", strconv.Itoa(lines))
	}
	return herdr.Text(ctx, s.Runner, t.Machine, args)
}

// ValidName comprueba el formato de un nombre sin tocar herdr; un nombre vacío
// no es válido aquí (Name lo interpreta como "borrar", quien llama lo decide).
func ValidName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("%q: %w", name, ErrInvalidName)
	}
	return nil
}

// Name asigna el nombre al agente y al pane a la vez, para que ambos coincidan.
// Un nombre vacío borra el nombre asignado.
func (s Service) Name(ctx context.Context, t inventory.Target, name string) error {
	rename := func(kind string) []string {
		if name == "" {
			return []string{kind, "rename", t.PaneID, "--clear"}
		}
		return []string{kind, "rename", t.PaneID, name}
	}

	if name != "" && !namePattern.MatchString(name) {
		return fmt.Errorf("%q: %w", name, ErrInvalidName)
	}
	if err := herdr.Call(ctx, s.Runner, t.Machine, rename("agent"), nil); err != nil {
		return err
	}
	if err := herdr.Call(ctx, s.Runner, t.Machine, rename("pane"), nil); err != nil {
		return fmt.Errorf("agent was renamed but its pane was not: %w", err)
	}
	return nil
}

// ErrInvalidLabel indica que la etiqueta de un tab o workspace no es aceptable.
var ErrInvalidLabel = errors.New("label must be text without control characters and must not start with \"-\"")

// ValidLabel comprueba la etiqueta de un tab o workspace. Se rechaza lo que
// empieza por "-" porque herdr podría tomarlo como una opción (no se pudo
// verificar sin renombrar algo real), y los caracteres de control.
func ValidLabel(label string) error {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" || strings.HasPrefix(trimmed, "-") || strings.ContainsFunc(label, unicode.IsControl) {
		return fmt.Errorf("%q: %w", label, ErrInvalidLabel)
	}
	return nil
}

// RenameTab cambia la etiqueta del tab que contiene al agente.
func (s Service) RenameTab(ctx context.Context, t inventory.Target, label string) error {
	if err := ValidLabel(label); err != nil {
		return err
	}
	return herdr.Call(ctx, s.Runner, t.Machine, []string{"tab", "rename", t.TabID, label}, nil)
}

// RenameWorkspace cambia la etiqueta del workspace que contiene al agente.
func (s Service) RenameWorkspace(ctx context.Context, t inventory.Target, label string) error {
	if err := ValidLabel(label); err != nil {
		return err
	}
	return herdr.Call(ctx, s.Runner, t.Machine, []string{"workspace", "rename", t.WorkspaceID, label}, nil)
}

// CloseTab cierra el tab que contiene al agente, y con él a todo lo que corre
// en ese tab. Quien llama debe haber confirmado antes.
func (s Service) CloseTab(ctx context.Context, t inventory.Target) error {
	return herdr.Call(ctx, s.Runner, t.Machine, []string{"tab", "close", t.TabID}, nil)
}

// CloseWorkspace cierra el workspace que contiene al agente, y con él todos sus
// tabs. Quien llama debe haber confirmado antes.
func (s Service) CloseWorkspace(ctx context.Context, t inventory.Target) error {
	return herdr.Call(ctx, s.Runner, t.Machine, []string{"workspace", "close", t.WorkspaceID}, nil)
}

// Focus lleva el foco de herdr al agente.
func (s Service) Focus(ctx context.Context, t inventory.Target) error {
	return herdr.Call(ctx, s.Runner, t.Machine, []string{"agent", "focus", t.PaneID}, nil)
}

// ErrTimeout indica que venció el plazo de una espera sin que el agente
// llegara al estado pedido.
var ErrTimeout = errors.New("timed out waiting for the agent")

// WaitOptions modifica la espera de un agente.
type WaitOptions struct {
	// Until lista los estados que cuentan como fin de la espera; vacío deja el
	// valor por defecto de herdr (idle, done o blocked).
	Until []string
	// TimeoutMillis acota la espera; cero espera indefinidamente.
	TimeoutMillis int
}

// Wait espera a que el agente llegue a uno de los estados pedidos y devuelve
// el estado alcanzado. Un plazo vencido se devuelve como ErrTimeout.
func (s Service) Wait(ctx context.Context, t inventory.Target, opts WaitOptions) (string, error) {
	args := []string{"agent", "wait", t.PaneID}
	for _, state := range opts.Until {
		args = append(args, "--until", state)
	}
	if opts.TimeoutMillis > 0 {
		args = append(args, "--timeout", strconv.Itoa(opts.TimeoutMillis))
	}

	var result struct {
		Agent struct {
			Status string `json:"agent_status"`
		} `json:"agent"`
	}
	if err := herdr.Call(ctx, s.Runner, t.Machine, args, &result); err != nil {
		return "", mapWaitError(err)
	}
	return result.Agent.Status, nil
}

// Notify muestra un aviso de herdr en esta máquina, que es donde está la persona.
func (s Service) Notify(ctx context.Context, title, body string) error {
	return herdr.Call(ctx, s.Runner, "", []string{"notification", "show", title, "--body", body}, nil)
}

// ErrBlocked indica que el agente espera a la persona (aprobación o pregunta)
// y herdr rechazó enviarle el mensaje.
var ErrBlocked = errors.New("the agent is waiting for you")

// readWindow es cuántas líneas se leen para localizar el mensaje enviado; es
// mayor que las líneas de respaldo porque la respuesta puede ser larga.
const readWindow = 200

// AskOptions modifica una pregunta.
type AskOptions struct {
	// TimeoutMillis acota la espera; cero espera indefinidamente.
	TimeoutMillis int
	// Lines es cuántas líneas finales se devuelven si no se localiza el mensaje.
	Lines int
}

// Answer es el resultado de preguntar a un agente.
type Answer struct {
	// Status es el estado en que quedó el agente (idle, done o blocked).
	Status string
	// Reply es el texto de la respuesta, o la última pantalla si no se pudo aislar.
	Reply string
	// AfterMessage es true si Reply es lo que aparece tras el mensaje enviado, y
	// false si es el respaldo (las últimas líneas).
	AfterMessage bool
}

// Ask envía el texto, espera a que el agente se asiente y lee su respuesta.
// Un agente ya bloqueado devuelve ErrBlocked sin enviar nada; un plazo vencido,
// ErrTimeout.
func (s Service) Ask(ctx context.Context, t inventory.Target, text string, opts AskOptions) (Answer, error) {
	var answer Answer
	if strings.TrimSpace(text) == "" {
		return answer, errors.New("message is empty")
	}

	args := []string{"agent", "prompt", t.PaneID, text, "--wait"}
	if opts.TimeoutMillis > 0 {
		args = append(args, "--timeout", strconv.Itoa(opts.TimeoutMillis))
	}
	var sent agentResult
	if err := herdr.Call(ctx, s.Runner, t.Machine, args, &sent); err != nil {
		return answer, mapWaitError(err)
	}
	answer.Status = sent.Agent.Status
	if answer.Status == "" {
		// Algunas versiones de herdr no devuelven el agente al esperar.
		var got agentResult
		if err := herdr.Call(ctx, s.Runner, t.Machine, []string{"agent", "get", t.PaneID}, &got); err != nil {
			return answer, err
		}
		answer.Status = got.Agent.Status
	}

	lines := opts.Lines
	if lines <= 0 {
		lines = DefaultReplyLines
	}
	screen, err := s.Look(ctx, t, max(readWindow, lines))
	if err != nil {
		return answer, err
	}
	answer.Reply, answer.AfterMessage = ExtractReply(screen, text, lines)
	return answer, nil
}

// DefaultReplyLines son las líneas finales que se devuelven cuando no se
// localiza el mensaje enviado.
const DefaultReplyLines = 40

// agentResult es la parte de una respuesta de herdr que interesa aquí.
type agentResult struct {
	Agent struct {
		Status string `json:"agent_status"`
	} `json:"agent"`
}

// mapWaitError traduce los errores de la API que tienen significado propio.
func mapWaitError(err error) error {
	var apiErr *herdr.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "timeout":
			return ErrTimeout
		case "agent_blocked":
			return ErrBlocked
		}
	}
	return err
}

// ExtractReply aísla la respuesta de un agente en lo que muestra su terminal.
// Busca la última línea que contiene el comienzo del mensaje enviado (sin
// distinguir mayúsculas ni espacios) y devuelve lo que sigue; afterMessage es
// true en ese caso. Si no lo encuentra, o no hay nada después, devuelve las
// últimas lines líneas y afterMessage es false. Es una heurística: la
// interfaz de cada agente pinta el mensaje a su manera.
func ExtractReply(screen, text string, lines int) (reply string, afterMessage bool) {
	all := strings.Split(strings.ReplaceAll(strings.TrimRight(screen, "\r\n \t"), "\r\n", "\n"), "\n")
	needle, truncated := needleOf(text)

	if needle != "" {
		for i := len(all) - 1; i >= 0; i-- {
			if containsWord(collapse(all[i]), needle, truncated) {
				if after := strings.Trim(strings.Join(all[i+1:], "\n"), "\n"); strings.TrimSpace(after) != "" {
					return strings.TrimRight(after, " \t\n"), true
				}
				break
			}
		}
	}
	if lines > 0 && len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n"), false
}

// needleOf es lo que se busca en pantalla: la primera línea del mensaje,
// normalizada y acotada para tolerar que la interfaz recorte o ajuste líneas.
func needleOf(text string) (needle string, truncated bool) {
	for _, line := range strings.Split(text, "\n") {
		if c := collapse(line); c != "" {
			if r := []rune(c); len(r) > 40 {
				return string(r[:40]), true
			}
			return c, false
		}
	}
	return "", false
}

// containsWord indica si line contiene needle sin partir una palabra: "hi" no
// está en "this". Si needle es el mensaje completo, también exige que termine en
// un límite; si está recortado, solo exige el límite izquierdo.
func containsWord(line, needle string, truncated bool) bool {
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	for from := 0; ; {
		i := strings.Index(line[from:], needle)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(needle)
		leftOK := start == 0
		if !leftOK {
			r, _ := utf8.DecodeLastRuneInString(line[:start])
			leftOK = !isWord(r)
		}
		rightOK := truncated || end == len(line)
		if !rightOK {
			r, _ := utf8.DecodeRuneInString(line[end:])
			rightOK = !isWord(r)
		}
		if leftOK && rightOK {
			return true
		}
		from = start + 1
	}
}

// collapse pasa a minúsculas y reduce cualquier espacio a uno solo.
func collapse(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
