package actions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/herdr"
)

// Kinds son los agentes que herdr sabe arrancar (`herdr agent start --help`).
// Se comprueban antes de crear nada para que una errata no deje un tab vacío.
var Kinds = []string{
	"pi", "claude", "codex", "gemini", "cursor", "devin", "agy", "cline", "omp",
	"mastracode", "opencode", "copilot", "kimi", "kiro", "droid", "amp", "grok",
	"hermes", "kilo", "qodercli", "qwen", "letta", "maki", "muse",
}

// ErrUnknownKind indica un tipo de agente que herdr no arranca.
var ErrUnknownKind = errors.New("unknown agent kind")

// CreateOptions describe un agente nuevo.
type CreateOptions struct {
	// Kind es el tipo de agente (claude, codex…).
	Kind string
	// Dir es la carpeta de trabajo; vacío deja la de herdr.
	Dir string
	// Name es el nombre del agente y del pane; vacío usa el tipo.
	Name string
	// Focus lleva el foco al tab nuevo.
	Focus bool
	// TimeoutMillis acota la espera a que el agente esté listo; cero deja el valor de herdr.
	TimeoutMillis int
}

// Created recoge lo que existe tras Create, también cuando un paso falla.
type Created struct {
	TabID       string
	PaneID      string
	WorkspaceID string
	// Started dice si el agente arrancó y está listo.
	Started bool
	// Named dice si el pane se renombró (solo se intenta con un nombre pedido).
	Named bool
}

// CreateError dice qué paso falló y qué quedó creado. No se deshace nada.
type CreateError struct {
	// Step es "tab", "agent" o "name".
	Step    string
	Created Created
	Err     error
}

func (e *CreateError) Error() string {
	switch e.Step {
	case "tab":
		return fmt.Sprintf("creating the tab failed: %v", e.Err)
	case "agent":
		return fmt.Sprintf("the tab %s was created but starting the agent failed: %v", e.Created.TabID, e.Err)
	}
	return fmt.Sprintf("the agent started in %s but naming its pane failed: %v", e.Created.PaneID, e.Err)
}

func (e *CreateError) Unwrap() error { return e.Err }

// Create abre un tab, arranca el agente en su pane y lo nombra: la secuencia
// `tab create` + `agent start` + `pane rename`. Si un paso falla devuelve un
// *CreateError con lo que ya existe, y no deshace nada.
func (s Service) Create(ctx context.Context, machine string, opts CreateOptions) (Created, error) {
	if !slices.Contains(Kinds, opts.Kind) {
		return Created{}, fmt.Errorf("%q: %w (want one of %s)", opts.Kind, ErrUnknownKind, strings.Join(Kinds, ", "))
	}
	if opts.Name != "" {
		if err := ValidName(opts.Name); err != nil {
			return Created{}, err
		}
	}
	name := opts.Name
	if name == "" {
		name = opts.Kind
	}

	tabArgs := []string{"tab", "create"}
	if opts.Dir != "" {
		tabArgs = append(tabArgs, "--cwd", opts.Dir)
	}
	tabArgs = append(tabArgs, "--label", name)
	if opts.Focus {
		tabArgs = append(tabArgs, "--focus")
	} else {
		tabArgs = append(tabArgs, "--no-focus")
	}

	var tab struct {
		RootPane struct {
			PaneID string `json:"pane_id"`
		} `json:"root_pane"`
		Tab struct {
			TabID       string `json:"tab_id"`
			WorkspaceID string `json:"workspace_id"`
		} `json:"tab"`
	}
	var created Created
	if err := herdr.Call(ctx, s.Runner, machine, tabArgs, &tab); err != nil {
		return created, &CreateError{Step: "tab", Created: created, Err: err}
	}
	created.TabID, created.WorkspaceID, created.PaneID = tab.Tab.TabID, tab.Tab.WorkspaceID, tab.RootPane.PaneID
	if created.PaneID == "" {
		return created, &CreateError{Step: "tab", Created: created, Err: errors.New("herdr created the tab but returned no pane")}
	}

	startArgs := []string{"agent", "start", name, "--kind", opts.Kind, "--pane", created.PaneID}
	if opts.TimeoutMillis > 0 {
		startArgs = append(startArgs, "--timeout", strconv.Itoa(opts.TimeoutMillis))
	}
	if err := herdr.Call(ctx, s.Runner, machine, startArgs, nil); err != nil {
		return created, &CreateError{Step: "agent", Created: created, Err: err}
	}
	created.Started = true

	if opts.Name != "" {
		if err := herdr.Call(ctx, s.Runner, machine, []string{"pane", "rename", created.PaneID, opts.Name}, nil); err != nil {
			return created, &CreateError{Step: "name", Created: created, Err: err}
		}
		created.Named = true
	}
	return created, nil
}
