package palette

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
)

// Origin es el lugar de herdr desde donde se abrió la paleta. La paleta vive en
// un popup, que no es un pane y no recibe HERDR_PANE_ID, pero herdr le pasa en
// HERDR_PLUGIN_CONTEXT_JSON el workspace, la pestaña y el pane enfocados.
type Origin struct {
	PaneID      string
	TabID       string
	WorkspaceID string
	Cwd         string
}

type contextJSON struct {
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	PaneID      string `json:"focused_pane_id"`
	Cwd         string `json:"focused_pane_cwd"`
}

// OriginFromEnv lee el origen de HERDR_PLUGIN_CONTEXT_JSON. Si falta o no se
// entiende, usa los HERDR_*_ID sueltos, que bastan para un subconjunto de comandos.
func OriginFromEnv(getenv func(string) string) Origin {
	var c contextJSON
	if raw := getenv("HERDR_PLUGIN_CONTEXT_JSON"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &c)
	}
	pick := func(fromJSON, envName string) string {
		if fromJSON != "" {
			return fromJSON
		}
		return getenv(envName)
	}
	return Origin{
		PaneID:      pick(c.PaneID, "HERDR_PANE_ID"),
		TabID:       pick(c.TabID, "HERDR_TAB_ID"),
		WorkspaceID: pick(c.WorkspaceID, "HERDR_WORKSPACE_ID"),
		Cwd:         c.Cwd,
	}
}

// Static devuelve el valor de una clave de contexto estática.
func (o Origin) Static(key string) string {
	switch key {
	case "pane_id":
		return o.PaneID
	case "tab_id":
		return o.TabID
	case "workspace_id":
		return o.WorkspaceID
	case "cwd":
		return o.Cwd
	}
	return ""
}

// Has dice si la clave puede resolverse con este origen. Los vecinos (siguiente
// y anterior) se calculan al ejecutar, pero solo tienen sentido si se conoce el
// workspace o la pestaña de partida.
func (o Origin) Has(key string) bool {
	switch key {
	case "next_workspace_id", "previous_workspace_id":
		return o.WorkspaceID != ""
	case "next_tab_id", "previous_tab_id":
		return o.TabID != ""
	}
	return o.Static(key) != ""
}

// Available deja las entradas cuyo contexto está completo: "Pane: Close" sin
// pane que cerrar sería una fila que falla al elegirla. Esto no consulta herdr.
func Available(entries []Entry, o Origin) []Entry {
	return slices.DeleteFunc(slices.Clone(entries), func(e Entry) bool {
		return slices.ContainsFunc(e.RequiredContext(), func(k string) bool { return !o.Has(k) })
	})
}

// Workspace y Tab son lo que la paleta necesita saber de cada uno.
type Workspace struct {
	ID     string `json:"workspace_id"`
	Label  string `json:"label"`
	Number int    `json:"number"`
}

// Tab es una pestaña de un workspace.
type Tab struct {
	ID          string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	Number      int    `json:"number"`
}

// Lister lee de herdr las listas que la paleta necesita, y solo cuando las necesita.
type Lister interface {
	Workspaces(ctx context.Context) ([]Workspace, error)
	Tabs(ctx context.Context, workspaceID string) ([]Tab, error)
	Agents(ctx context.Context) ([]Agent, error)
}

// Context entrega los valores de contexto de un comando. Los vecinos se
// calculan al pedirlos y se recuerdan: abrir la paleta no cuesta ninguna
// consulta de más, y el coste solo lo paga quien ejecuta "Next/Previous".
type Context struct {
	Origin Origin
	Lists  Lister

	workspaces []Workspace
	tabs       []Tab
	loadedWS   bool
	loadedTabs bool
}

// Value resuelve una clave de contexto, estática o calculada.
func (c *Context) Value(ctx context.Context, key string) (string, error) {
	if v := c.Origin.Static(key); v != "" {
		return v, nil
	}
	var id string
	switch key {
	case "next_workspace_id":
		id = c.neighbour(ctx, true, 1)
	case "previous_workspace_id":
		id = c.neighbour(ctx, true, -1)
	case "next_tab_id":
		id = c.neighbour(ctx, false, 1)
	case "previous_tab_id":
		id = c.neighbour(ctx, false, -1)
	}
	if id == "" {
		return "", fmt.Errorf("missing %q for this command", key)
	}
	return id, nil
}

func (c *Context) neighbour(ctx context.Context, workspace bool, offset int) string {
	if c.Lists == nil {
		return ""
	}
	if workspace {
		if !c.loadedWS {
			c.workspaces, _ = c.Lists.Workspaces(ctx)
			c.loadedWS = true
		}
		items := slices.Clone(c.workspaces)
		slices.SortStableFunc(items, func(a, b Workspace) int { return a.Number - b.Number })
		return neighbourID(items, func(w Workspace) string { return w.ID }, c.Origin.WorkspaceID, offset)
	}
	if !c.loadedTabs {
		c.tabs, _ = c.Lists.Tabs(ctx, c.Origin.WorkspaceID)
		c.loadedTabs = true
	}
	items := slices.DeleteFunc(slices.Clone(c.tabs), func(t Tab) bool {
		return c.Origin.WorkspaceID != "" && t.WorkspaceID != c.Origin.WorkspaceID
	})
	slices.SortStableFunc(items, func(a, b Tab) int { return a.Number - b.Number })
	return neighbourID(items, func(t Tab) string { return t.ID }, c.Origin.TabID, offset)
}

// neighbourID da el vecino de current dando la vuelta por los extremos, como los
// atajos de herdr. Con un solo elemento él mismo es su vecino: el comando queda
// inofensivo en vez de desaparecer cuando solo hay una pestaña.
func neighbourID[T any](items []T, id func(T) string, current string, offset int) string {
	i := slices.IndexFunc(items, func(t T) bool { return id(t) == current })
	if i < 0 {
		return ""
	}
	return id(items[(i+offset+len(items))%len(items)])
}
