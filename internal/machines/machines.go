// Paquete machines gestiona los perfiles de máquinas SSH guardados en herdr:
// listarlos, encontrarlos por nombre, habilitarlos o deshabilitarlos y
// renombrarlos con las mismas garantías que el menú original (relectura por ID, sin reintentos
// ante resultados inciertos y confirmación posterior).
package machines

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/SantosJMM/herdr-human/internal/herdr"
)

var (
	// ErrStateChanged indica que otro cliente cambió el perfil después de que
	// la persona lo viera; hay que revisar el estado actual antes de reintentar.
	ErrStateChanged = errors.New("the profile changed elsewhere; review the current state first")
	// ErrProfileGone indica que el perfil ya no existe.
	ErrProfileGone = errors.New("the profile was removed; refresh and choose again")
	// ErrInvalidLabel indica que la etiqueta nueva no es aceptable.
	ErrInvalidLabel = errors.New("label must be text without control characters and must not start with \"-\"")
	// ErrLabelTaken indica que otra máquina ya usa esa etiqueta.
	ErrLabelTaken = errors.New("another saved machine already uses that label")
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

// ValidLabel devuelve la etiqueta sin espacios en los extremos si es
// aceptable. Se rechaza la vacía, la que empieza por "-" (herdr podría tomarla
// como una opción) y la que lleva caracteres de control.
func ValidLabel(label string) (string, error) {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" || strings.HasPrefix(trimmed, "-") || strings.ContainsFunc(trimmed, unicode.IsControl) {
		return "", fmt.Errorf("%q: %w", label, ErrInvalidLabel)
	}
	return trimmed, nil
}

// Rename cambia la etiqueta del perfil que la persona vio (selected).
//
// Valida la etiqueta antes de tocar herdr. Después relee los perfiles: si el
// perfil desapareció o su etiqueta ya no es la que se vio, se niega
// (ErrProfileGone, ErrStateChanged); si ya tiene esa etiqueta no hace nada; si
// otra máquina usa esa etiqueta (sin distinguir mayúsculas) se niega
// (ErrLabelTaken). Tras mutar vuelve a leerlo para confirmar; si no se
// confirma devuelve ErrUnconfirmed. La mutación nunca se reintenta. Cambiar
// solo las mayúsculas de la propia etiqueta es válido.
func (s Service) Rename(ctx context.Context, selected herdr.Profile, label string) (Result, error) {
	label, err := ValidLabel(label)
	if err != nil {
		return Result{}, err
	}

	profiles, err := herdr.ListProfiles(ctx, s.Runner)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrProfileGone, err)
	}
	fresh, found := herdr.Profile{}, false
	for _, p := range profiles {
		if p.ID == selected.ID {
			fresh, found = p, true
			break
		}
	}
	if !found {
		return Result{}, fmt.Errorf("%w: profile %s not found", ErrProfileGone, selected.ID)
	}
	if fresh.Label != selected.Label {
		return Result{}, fmt.Errorf("%w (now %q)", ErrStateChanged, fresh.Label)
	}
	if fresh.Label == label {
		return Result{Profile: fresh}, nil
	}
	for _, p := range profiles {
		if p.ID != fresh.ID && strings.EqualFold(p.Label, label) {
			return Result{}, fmt.Errorf("%w (%q)", ErrLabelTaken, p.Label)
		}
	}

	if err := herdr.SetLabel(ctx, s.Runner, fresh.ID, label); err != nil {
		return Result{}, fmt.Errorf("rename %q: %w", fresh.Label, err)
	}

	confirmed, err := s.lookup(ctx, fresh.ID)
	if err != nil {
		return Result{}, fmt.Errorf("%w: re-reading %q failed: %v", ErrUnconfirmed, fresh.Label, err)
	}
	if confirmed.Label != label {
		return Result{}, fmt.Errorf("%w: it is still called %q", ErrUnconfirmed, confirmed.Label)
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
