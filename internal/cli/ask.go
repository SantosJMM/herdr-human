package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SantosJMM/herdr-human/internal/actions"
)

// ask envía una pregunta, espera la respuesta y la imprime. Solo la respuesta
// va a stdout; el progreso y el origen de la respuesta van a stderr para que
// el resultado se pueda canalizar. Sale con 3 si el agente espera a la persona
// y con 4 si venció el plazo.
func (a *app) ask(ctx context.Context, args []string) error {
	var cf commonFlags
	var timeoutMillis, lines int
	fs := newFlagSet("ask")
	cf.register(fs, false)
	cf.registerFresh(fs)
	fs.IntVar(&timeoutMillis, "timeout", 0, "milliseconds to wait for the answer")
	fs.IntVar(&lines, "lines", actions.DefaultReplyLines, "lines to show if your message is not found on screen")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) < 2 {
		return usagef("usage: hum ask <who> <message...> [--timeout MS] [--lines N]")
	}
	if lines <= 0 {
		return usagef("--lines must be positive")
	}

	t, err := a.resolveTarget(ctx, cf, rest[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(a.env.Stderr, "asking %s (%s)...\n", t.Ref(), t.Name)

	start := time.Now()
	answer, err := actions.Service{Runner: a.env.Runner}.Ask(ctx, t, strings.Join(rest[1:], " "),
		actions.AskOptions{TimeoutMillis: timeoutMillis, Lines: lines})
	switch {
	case errors.Is(err, actions.ErrBlocked):
		return &exitError{code: exitBlocked, err: fmt.Errorf("%s is waiting for you; run: hum go %s", t.Ref(), t.Ref())}
	case errors.Is(err, actions.ErrTimeout):
		return &exitError{code: exitTimeout, err: fmt.Errorf("timed out waiting for %s (%s)", t.Ref(), t.Name)}
	case err != nil:
		return targetHint(err, t)
	}
	fmt.Fprintf(a.env.Stderr, "%s in %s\n", answer.Status, time.Since(start).Round(100*time.Millisecond))

	if answer.Status == "blocked" {
		return &exitError{code: exitBlocked, err: fmt.Errorf("%s is waiting for you: %s\nhint: hum go %s",
			t.Ref(), lastLine(answer.Reply), t.Ref())}
	}
	if answer.AfterMessage {
		fmt.Fprintln(a.env.Stderr, "reply: after your message")
	} else {
		fmt.Fprintf(a.env.Stderr, "reply: last %d lines (your message was not found)\n", lines)
	}
	fmt.Fprintln(a.env.Stdout, answer.Reply)
	return nil
}

// lastLine devuelve la última línea no vacía de s.
func lastLine(s string) string {
	parts := strings.Split(strings.TrimRight(s, "\r\n \t"), "\n")
	return strings.TrimSpace(parts[len(parts)-1])
}
