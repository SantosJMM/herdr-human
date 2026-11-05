package palette

import (
	_ "embed" // commands.json viaja dentro del binario
	"encoding/json"
	"fmt"
	"slices"
)

//go:embed commands.json
var catalogJSON []byte

const supportedSchemaVersion = 1

// Claves de contexto válidas. Las estáticas salen del pane de origen; las
// calculadas son el vecino siguiente o anterior del workspace o de la pestaña.
var (
	staticContextKeys   = []string{"pane_id", "tab_id", "workspace_id", "cwd"}
	computedContextKeys = []string{"next_workspace_id", "previous_workspace_id", "next_tab_id", "previous_tab_id"}
	selectorNames       = []string{"workspaces", "tabs", "agents", "machines", "plugins"}
)

type catalogFile struct {
	SchemaVersion int             `json:"schema_version"`
	Commands      []catalogRecord `json:"commands"`
}

type catalogRecord struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Command     []string   `json:"command"`
	Arguments   []Argument `json:"arguments"`
}

// Native devuelve los comandos nativos que trae el binario. Una entrada mal
// formada o repetida se descarta en vez de tumbar toda la paleta.
func Native() ([]Entry, error) { return parseCatalog(catalogJSON) }

func parseCatalog(data []byte) ([]Entry, error) {
	var file catalogFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("commands.json is not valid JSON: %w", err)
	}
	if file.SchemaVersion != supportedSchemaVersion {
		return nil, fmt.Errorf("commands.json declares schema_version %d, expected %d", file.SchemaVersion, supportedSchemaVersion)
	}

	seen := make(map[string]bool, len(file.Commands))
	entries := make([]Entry, 0, len(file.Commands))
	for _, c := range file.Commands {
		if !validRecord(c) || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		entries = append(entries, Entry{
			Kind: KindNative, ID: c.ID, Title: c.Title, Description: c.Description,
			Source: nativeSource, Command: c.Command, Arguments: c.Arguments,
		})
	}
	return entries, nil
}

func validRecord(c catalogRecord) bool {
	if c.ID == "" || c.Title == "" || len(c.Command) == 0 || slices.Contains(c.Command, "") {
		return false
	}
	for _, a := range c.Arguments {
		if !validArgument(a) {
			return false
		}
	}
	return true
}

func validArgument(a Argument) bool {
	switch a.Source {
	case SourceLiteral:
		return a.Value != ""
	case SourceContext:
		return slices.Contains(staticContextKeys, a.Key) || slices.Contains(computedContextKeys, a.Key)
	case SourceInput:
		return a.Prompt != "" && (a.DefaultContext == "" || slices.Contains(staticContextKeys, a.DefaultContext))
	case SourceSelect:
		return slices.Contains(selectorNames, a.Selector) && a.Prompt != "" &&
			(a.ExcludeContext == "" || slices.Contains(staticContextKeys, a.ExcludeContext))
	default:
		return false
	}
}
