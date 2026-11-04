package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// APIError es un error reportado por herdr dentro de su sobre JSON.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type envelope struct {
	Result json.RawMessage `json:"result"`
	Error  *APIError       `json:"error"`
}

// apiErrorFrom busca un error de la API en una salida que quizá no sea JSON.
func apiErrorFrom(out []byte) *APIError {
	var env envelope
	if err := json.Unmarshal(bytes.TrimSpace(out), &env); err != nil {
		return nil
	}
	return env.Error
}

func describe(args []string) string {
	return "herdr " + strings.Join(args, " ")
}

// Text ejecuta un comando cuya salida es texto plano (por ejemplo, agent read).
// Si herdr falla con un error de la API, devuelve ese error en vez del genérico.
func Text(ctx context.Context, r Runner, machine string, args []string) (string, error) {
	out, err := r.Run(ctx, machine, args...)
	if err != nil {
		if apiErr := apiErrorFrom(out); apiErr != nil {
			return "", apiErr
		}
		return "", fmt.Errorf("%s: %w", describe(args), err)
	}
	if apiErr := apiErrorFrom(out); apiErr != nil {
		return "", apiErr
	}
	return string(out), nil
}

// Call ejecuta un comando de herdr y decodifica su campo "result" en result.
// Si result es nil, solo comprueba que el comando no haya fallado.
func Call(ctx context.Context, r Runner, machine string, args []string, result any) error {
	out, err := r.Run(ctx, machine, args...)
	if err != nil {
		if apiErr := apiErrorFrom(out); apiErr != nil {
			return apiErr
		}
		return fmt.Errorf("%s: %w", describe(args), err)
	}

	var env envelope
	if err := json.Unmarshal(bytes.TrimSpace(out), &env); err != nil {
		return fmt.Errorf("%s: decode response: %w", describe(args), err)
	}
	if env.Error != nil {
		return env.Error
	}
	if result == nil {
		return nil
	}
	if len(env.Result) == 0 {
		return fmt.Errorf("%s: %w", describe(args), errNoResult)
	}
	if err := json.Unmarshal(env.Result, result); err != nil {
		return fmt.Errorf("%s: decode result: %w", describe(args), err)
	}
	return nil
}
