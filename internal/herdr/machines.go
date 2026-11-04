package herdr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Machine es una máquina SSH guardada en herdr.
type Machine struct {
	ID      string
	Label   string
	Target  string
	Enabled bool
}

// ListMachines lee `herdr machine list`, que imprime filas separadas por
// tabuladores: id, etiqueta, destino, perfil y estado. Las filas con menos de
// cinco campos se ignoran.
func ListMachines(ctx context.Context, r Runner) ([]Machine, error) {
	text, err := Text(ctx, r, "", []string{"machine", "list"})
	if err != nil {
		return nil, err
	}

	var machines []Machine
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(fields) < 5 {
			continue
		}
		machines = append(machines, Machine{
			ID:      strings.TrimSpace(fields[0]),
			Label:   strings.TrimSpace(fields[1]),
			Target:  strings.TrimSpace(fields[2]),
			Enabled: strings.TrimSpace(fields[4]) == "enabled",
		})
	}
	return machines, nil
}

// Profile es un perfil de máquina SSH tal como lo describe `herdr machine list --json`.
// Enabled es estado de configuración (conexión automática), no una comprobación
// de que la máquina esté alcanzable.
type Profile struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Target   string `json:"target"`
	Session  string `json:"session"`
	Enabled  bool   `json:"enabled"`
	Selected bool   `json:"selected"`
}

// ListProfiles devuelve los perfiles guardados con su ID inmutable.
func ListProfiles(ctx context.Context, r Runner) ([]Profile, error) {
	text, err := Text(ctx, r, "", []string{"machine", "list", "--json"})
	if err != nil {
		return nil, err
	}
	var profiles []Profile
	if err := json.Unmarshal([]byte(text), &profiles); err != nil {
		return nil, fmt.Errorf("herdr machine list --json: decode response: %w", err)
	}
	return profiles, nil
}

// SetEnabled habilita o deshabilita un perfil por su ID. No reintenta ni
// verifica: quien la llama decide cómo confirmar el resultado.
func SetEnabled(ctx context.Context, r Runner, id string, enabled bool) error {
	verb := "disable"
	if enabled {
		verb = "enable"
	}
	_, err := Text(ctx, r, "", []string{"machine", verb, id})
	return err
}
