// Paquete machines gestiona los perfiles de máquinas SSH guardados en herdr:
// listarlos, encontrarlos por nombre y habilitarlos o deshabilitarlos con las
// mismas garantías que el menú original (relectura por ID, sin reintentos
// ante resultados inciertos y confirmación posterior).
package machines

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/herdr"
)

var (
	// ErrStateChanged indica que otro cliente cambió el perfil después de que
	// la persona lo viera; hay que revisar el estado actual antes de reintentar.
	ErrStateChanged = errors.New("the profile changed elsewhere; review the current state first")
	// ErrProfileGone indica que el perfil ya no existe.
	ErrProfileGone = errors.New("the profile was removed; refresh and choose again")
	// ErrUnconfirmed indica que herdr aceptó el cambio pero no se pudo
	// comprobar que surtiera efecto. No se reintenta automáticamente.
	ErrUnconfirmed = errors.New("the change could not be confirmed")
)

// Service opera sobre los perfiles a través de un Runner.
type Service struct {
	Runner herdr.Runner
}

// Result describe el resultado de SetEnabled.
type Result struct {
	// Profile es el estado confirmado del perfil.
	Profile herdr.Profile
	// Changed es false cuando el perfil ya estaba en el estado pedido.
	Changed bool
}

// List devuelve los perfiles ordenados por etiqueta, sin distinguir mayúsculas.
func (s Service) List(ctx context.Context) ([]herdr.Profile, error) {
	profiles, err := herdr.ListProfiles(ctx, s.Runner)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(profiles, func(a, b herdr.Profile) int {
		return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label))
	})
	return profiles, nil
}

// Find elige un perfil por etiqueta exacta, por ID o por prefijo único de la
// etiqueta (sin distinguir mayúsculas). Gana la etiqueta exacta.
func Find(profiles []herdr.Profile, query string) (herdr.Profile, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return herdr.Profile{}, errors.New("empty machine name")
	}

	for _, p := range profiles {
		if strings.EqualFold(p.Label, query) || p.ID == query {
			return p, nil
		}
	}

	var hits []herdr.Profile
	for _, p := range profiles {
		if strings.HasPrefix(strings.ToLower(p.Label), strings.ToLower(query)) {
			hits = append(hits, p)
		}
	}
	switch len(hits) {
	case 0:
		return herdr.Profile{}, fmt.Errorf("no saved machine matches %q", query)
	case 1:
		return hits[0], nil
	default:
		labels := make([]string, len(hits))
		for i, h := range hits {
			labels[i] = h.Label
		}
		return herdr.Profile{}, fmt.Errorf("%q matches %d saved machines (%s); be more specific",
			query, len(hits), strings.Join(labels, ", "))
	}
}

// SetEnabled habilita o deshabilita el perfil que la persona vio (selected).
//
// Antes de mutar relee el perfil por su ID inmutable: si desapareció o su
// estado ya no coincide con lo que se vio, se niega (ErrProfileGone,
// ErrStateChanged). Si ya está en el estado pedido no hace nada. Tras mutar
// vuelve a leerlo para confirmar; si no se confirma devuelve ErrUnconfirmed.
// La mutación nunca se reintenta.
func (s Service) SetEnabled(ctx context.Context, selected herdr.Profile, enabled bool) (Result, error) {
	fresh, err := s.lookup(ctx, selected.ID)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrProfileGone, err)
	}
	if fresh.Enabled != selected.Enabled {
		return Result{}, fmt.Errorf("%w (%s)", ErrStateChanged, fresh.Label)
	}
	if fresh.Enabled == enabled {
		return Result{Profile: fresh}, nil
	}

	if err := herdr.SetEnabled(ctx, s.Runner, fresh.ID, enabled); err != nil {
		return Result{}, fmt.Errorf("change %q: %w", fresh.Label, err)
	}

	confirmed, err := s.lookup(ctx, fresh.ID)
	if err != nil {
		return Result{}, fmt.Errorf("%w: re-reading %q failed: %v", ErrUnconfirmed, fresh.Label, err)
	}
	if confirmed.Enabled != enabled {
		return Result{}, fmt.Errorf("%w: %q is still %s", ErrUnconfirmed, fresh.Label, state(confirmed.Enabled))
	}
	return Result{Profile: confirmed, Changed: true}, nil
}

func (s Service) lookup(ctx context.Context, id string) (herdr.Profile, error) {
	profiles, err := herdr.ListProfiles(ctx, s.Runner)
	if err != nil {
		return herdr.Profile{}, err
	}
	for _, p := range profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return herdr.Profile{}, fmt.Errorf("profile %s not found", id)
}

func state(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
