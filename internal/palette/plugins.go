package palette

import (
	"context"
	"slices"

	"github.com/SantosJMM/herdr-human/internal/herdr"
)

// PluginInfo es lo que `herdr plugin list --json` cuenta de cada plugin. Esa
// única llamada trae el nombre y las acciones, así que no hace falta pedir
// además `plugin action list`.
type PluginInfo struct {
	PluginID string         `json:"plugin_id"`
	Name     string         `json:"name"`
	Enabled  bool           `json:"enabled"`
	Actions  []PluginAction `json:"actions"`
}

// PluginAction es una acción tal como la declara el manifest de un plugin.
type PluginAction struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Platforms   []string `json:"platforms"`
}

// ListPlugins pide a herdr los plugins instalados con sus acciones.
func ListPlugins(ctx context.Context, r herdr.Runner) ([]PluginInfo, error) {
	var res struct {
		Plugins []PluginInfo `json:"plugins"`
	}
	if err := herdr.Call(ctx, r, "", []string{"plugin", "list", "--json"}, &res); err != nil {
		return nil, err
	}
	return res.Plugins, nil
}

// Platform traduce runtime.GOOS al nombre de plataforma que usan los manifests.
func Platform(goos string) string {
	if goos == "darwin" {
		return "macos"
	}
	return goos
}

// PluginEntries convierte los plugins en filas de la paleta. Se omiten los
// plugins deshabilitados, las acciones que no corren en esta plataforma y todo
// lo del plugin excluido (el propio, para que la paleta no se liste a sí misma).
// Una lista vacía de plataformas significa que herdr la ejecuta en cualquiera.
func PluginEntries(plugins []PluginInfo, platform, excludePluginID string) []Entry {
	var entries []Entry
	for _, p := range plugins {
		if !p.Enabled || p.PluginID == "" || p.PluginID == excludePluginID {
			continue
		}
		source := p.Name
		if source == "" {
			source = p.PluginID
		}
		for _, a := range p.Actions {
			if a.ID == "" || (len(a.Platforms) > 0 && !slices.Contains(a.Platforms, platform)) {
				continue
			}
			title := a.Title
			if title == "" {
				title = a.ID
			}
			entries = append(entries, Entry{
				Kind: KindPlugin, ID: p.PluginID + "." + a.ID, Title: title, Description: a.Description,
				Source: source, PluginID: p.PluginID, ActionID: a.ID,
			})
		}
	}
	return entries
}
