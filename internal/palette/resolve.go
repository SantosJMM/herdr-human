package palette

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/herdr"
)

// InputSpec pide un texto a la persona.
type InputSpec struct {
	Prompt   string
	Initial  string
	Required bool
}

// Choice es una opción de una lista de selección.
type Choice struct {
	Value string
	Label string
}

// Prompter es la parte de la paleta que habla con la persona. ok es false si
// canceló (Esc): eso aborta todo el comando, no lo ejecuta con un hueco.
type Prompter interface {
	Input(ctx context.Context, spec InputSpec) (text string, ok bool, err error)
	Choose(ctx context.Context, prompt string, choices []Choice) (value string, ok bool, err error)
}

// Resolve arma la línea de comandos de herdr de un comando nativo, recorriendo
// sus argumentos en orden y preguntando cuando hace falta. ok es false si la
// persona canceló en alguna pregunta.
func Resolve(ctx context.Context, e Entry, vals *Context, p Prompter) (argv []string, ok bool, err error) {
	argv = slices.Clone(e.Command)
	for _, a := range e.Arguments {
		switch a.Source {
		case SourceLiteral:
			argv = append(argv, a.Value)

		case SourceContext:
			v, err := vals.Value(ctx, a.Key)
			if err != nil {
				return nil, false, err
			}
			argv = append(argv, v)

		case SourceInput:
			initial := ""
			if a.DefaultContext != "" {
				initial = vals.Origin.Static(a.DefaultContext)
			}
			text, ok, err := p.Input(ctx, InputSpec{Prompt: a.Prompt, Initial: initial, Required: a.Required})
			if err != nil || !ok {
				return nil, false, err
			}
			switch {
			case a.Flag == "":
				argv = append(argv, text)
			case strings.TrimSpace(text) != "":
				argv = append(argv, a.Flag, text)
			}
			// Un valor opcional vacío no añade nada: ni la opción ni el valor.

		case SourceSelect:
			choices, err := SelectorChoices(ctx, vals.Lists, a.Selector, vals.Origin, a.ExcludeContext)
			if err != nil {
				return nil, false, err
			}
			if len(choices) == 0 {
				return nil, false, fmt.Errorf("nothing to choose for %q", a.Prompt)
			}
			value, ok, err := p.Choose(ctx, a.Prompt, choices)
			if err != nil || !ok {
				return nil, false, err
			}
			argv = append(argv, value)

		default:
			return nil, false, fmt.Errorf("unsupported argument source %q", a.Source)
		}
	}
	return argv, true, nil
}

// Agent es un agente de herdr tal como lo muestra el selector de agentes.
type Agent struct {
	PaneID string `json:"pane_id"`
	Agent  string `json:"agent"`
	Status string `json:"agent_status"`
	Title  string `json:"terminal_title_stripped"`
}

var statusGlyph = map[string]string{"working": "●", "idle": "○", "blocked": "!", "done": "✓"}

func agentLabel(a Agent) string {
	glyph := statusGlyph[a.Status]
	if glyph == "" {
		glyph = "·"
	}
	name := a.Agent
	if name == "" {
		name = "agent"
	}
	title := a.Title
	if title == "" {
		title = a.PaneID
	}
	return fmt.Sprintf("%s %s — %s", glyph, name, title)
}

func labelOrID(label, id string) string {
	if label != "" {
		return label
	}
	return id
}

// SelectorChoices lista las opciones de un selector, sin la excluida (por
// ejemplo, no ofrecer el workspace en el que ya estás para "cambiar de workspace").
// Las listas se piden a herdr en este momento: ninguna se carga al abrir.
func SelectorChoices(ctx context.Context, l Lister, selector string, o Origin, excludeKey string) ([]Choice, error) {
	// Los tipos de agente son una lista fija que mantiene hum, la misma que usa
	// `hum new`: no hace falta herdr para ofrecerla ni se copia aquí.
	if selector == "agent_kinds" {
		choices := make([]Choice, len(actions.Kinds))
		for i, k := range actions.Kinds {
			choices[i] = Choice{Value: k, Label: k}
		}
		return choices, nil
	}
	if l == nil {
		return nil, errors.New("no herdr connection to list " + selector)
	}
	var choices []Choice
	switch selector {
	case "workspaces":
		items, err := l.Workspaces(ctx)
		if err != nil {
			return nil, err
		}
		for _, w := range items {
			choices = append(choices, Choice{Value: w.ID, Label: labelOrID(w.Label, w.ID)})
		}
	case "tabs":
		items, err := l.Tabs(ctx, o.WorkspaceID)
		if err != nil {
			return nil, err
		}
		for _, t := range items {
			choices = append(choices, Choice{Value: t.ID, Label: labelOrID(t.Label, t.ID)})
		}
	case "agents":
		items, err := l.Agents(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range items {
			choices = append(choices, Choice{Value: a.PaneID, Label: agentLabel(a)})
		}
	case "machines":
		items, err := l.Machines(ctx)
		if err != nil {
			return nil, err
		}
		for _, m := range items {
			state := "disabled"
			if m.Enabled {
				state = "enabled"
			}
			choices = append(choices, Choice{Value: m.ID, Label: fmt.Sprintf("%s (%s) — %s", labelOrID(m.Label, m.ID), m.Target, state)})
		}
	case "plugins":
		items, err := l.Plugins(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range items {
			state := "disabled"
			if p.Enabled {
				state = "enabled"
			}
			choices = append(choices, Choice{Value: p.PluginID, Label: fmt.Sprintf("%s (%s) — %s", labelOrID(p.Name, p.PluginID), p.PluginID, state)})
		}
	default:
		return nil, fmt.Errorf("unknown selector %q", selector)
	}

	excluded := ""
	if excludeKey != "" {
		excluded = o.Static(excludeKey)
	}
	return slices.DeleteFunc(choices, func(c Choice) bool { return c.Value == "" || (excluded != "" && c.Value == excluded) }), nil
}

// HerdrLister implementa Lister llamando al CLI de herdr.
type HerdrLister struct{ Runner herdr.Runner }

// Workspaces lista los workspaces.
func (h HerdrLister) Workspaces(ctx context.Context) ([]Workspace, error) {
	var res struct {
		Workspaces []Workspace `json:"workspaces"`
	}
	err := herdr.Call(ctx, h.Runner, "", []string{"workspace", "list"}, &res)
	return res.Workspaces, err
}

// Tabs lista las pestañas del workspace (todas si workspaceID está vacío).
func (h HerdrLister) Tabs(ctx context.Context, workspaceID string) ([]Tab, error) {
	args := []string{"tab", "list"}
	if workspaceID != "" {
		args = append(args, "--workspace", workspaceID)
	}
	var res struct {
		Tabs []Tab `json:"tabs"`
	}
	err := herdr.Call(ctx, h.Runner, "", args, &res)
	return res.Tabs, err
}

// Agents lista los agentes.
func (h HerdrLister) Agents(ctx context.Context) ([]Agent, error) {
	var res struct {
		Agents []Agent `json:"agents"`
	}
	err := herdr.Call(ctx, h.Runner, "", []string{"agent", "list"}, &res)
	return res.Agents, err
}

// Machines lista los perfiles de máquinas SSH guardados.
func (h HerdrLister) Machines(ctx context.Context) ([]herdr.Profile, error) {
	return herdr.ListProfiles(ctx, h.Runner)
}

// Plugins lista los plugins instalados, habilitados o no.
func (h HerdrLister) Plugins(ctx context.Context) ([]PluginInfo, error) {
	return ListPlugins(ctx, h.Runner)
}
