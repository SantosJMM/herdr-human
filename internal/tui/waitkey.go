package tui

import (
	"context"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

// waitKey es un programa mínimo: muestra un texto y termina con la primera tecla.
type waitKey struct{ text string }

func (waitKey) Init() tea.Cmd { return nil }

func (w waitKey) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		return w, tea.Quit
	}
	return w, nil
}

func (w waitKey) View() string { return w.text }

// WaitKey muestra text en la línea actual y espera a que se pulse cualquier
// tecla. No usa pantalla alterna, así que lo que había impreso antes sigue a la
// vista. Termina también si se cancela el contexto.
func WaitKey(ctx context.Context, in io.Reader, out io.Writer, text string) error {
	_, err := tea.NewProgram(waitKey{text: text},
		tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx)).Run()
	return err
}
