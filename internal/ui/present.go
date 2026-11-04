package ui

import (
	"fmt"
	"strings"
	"time"
)

// ShortName reduce los títulos con ruido ("Tarea | proyecto | Tarea | 5h 8%
// left") a su primer segmento, salvo si full. Los nombres que pone la persona
// no contienen " | ", así que no se alteran.
func ShortName(name string, full bool) string {
	if full {
		return name
	}
	first, _, _ := strings.Cut(name, " | ")
	if first = strings.TrimSpace(first); first != "" {
		return first
	}
	return name
}

// StatusRole elige el color de un estado de agente.
func StatusRole(status string) Role {
	switch status {
	case "blocked":
		return RoleBlocked
	case "working":
		return RoleWorking
	case "done":
		return RoleDone
	case "idle":
		return RoleIdle
	}
	return RoleDim
}

// FormatElapsed muestra una duración corta: "180ms" o "1.3s".
func FormatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// Color indica si el estilo emite secuencias de color.
func (s *Style) Color() bool { return s.cap.Color }

// ASCII indica si se usan los glifos de respaldo (HUM_ASCII=1).
func (s *Style) ASCII() bool { return s.cap.ASCII }

// spinnerStep es lo que dura cada cuadro del indicador de espera.
const spinnerStep = 100 * time.Millisecond

// Spinner devuelve el cuadro del indicador de espera para un tiempo transcurrido
// (uno nuevo cada 100 ms); con ascii usa | / - \ en vez de los puntos Braille.
func Spinner(ascii bool, elapsed time.Duration) string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	if ascii {
		frames = []string{"|", "/", "-", `\`}
	}
	i := int(elapsed / spinnerStep)
	return frames[(i%len(frames)+len(frames))%len(frames)]
}
