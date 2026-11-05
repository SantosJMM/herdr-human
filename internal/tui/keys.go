package tui

import (
	"context"

	"github.com/SantosJMM/herdr-human/internal/palette"
)

// KeyCheck es el veredicto sobre una tecla candidata para una entrada.
type KeyCheck struct {
	// Key es la tecla en su forma canónica (la que se escribiría en el archivo).
	Key string
	// Problem impide guardarla (tecla inválida, ya usada...): se muestra y no se avanza.
	Problem string
	// Warning no la impide, pero hay que confirmarla a conciencia: por ejemplo, es
	// un atajo por defecto de herdr y no se sabe cuál de los dos gana.
	Warning string
}

// KeySaved cuenta cómo salió un cambio de atajo.
type KeySaved struct {
	// Key es la tecla que quedó; vacía si se quitó.
	Key string
	// Note es un aviso a mostrar junto al resultado (por ejemplo, que herdr no
	// recargó la configuración).
	Note string
}

// KeyService deja que la paleta asigne, cambie y quite atajos de teclado en el
// config.toml de herdr. Con nulo, la paleta no ofrece la función.
type KeyService interface {
	// Bindable dice si la entrada puede enlazarse a una tecla y cuál tiene ahora
	// (vacía si ninguna).
	Bindable(e palette.Entry) (current string, ok bool)
	// HowItRuns cuenta, en una frase, qué pasará al pulsar el atajo de la entrada
	// (se ejecuta directo, o abre la paleta y pregunta). Vacía si no hay nada que decir.
	HowItRuns(e palette.Entry) string
	// Check valida una tecla candidata sin escribir nada.
	Check(e palette.Entry, key string) KeyCheck
	// Save enlaza la tecla a la entrada y recarga la configuración de herdr.
	Save(ctx context.Context, e palette.Entry, key string) (KeySaved, error)
	// Remove quita el atajo de la entrada.
	Remove(ctx context.Context, e palette.Entry) (KeySaved, error)
}
