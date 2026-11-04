package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/SantosJMM/herdr-human/internal/herdr"
)

// WorktreeScope dice en qué repositorio se opera: el workspace de un agente
// (sirve también en máquinas remotas) o una carpeta de la máquina local.
// Nunca se pasa --trust-repository: confiar en un repositorio es decisión de
// la persona, que lo hace a mano con herdr.
type WorktreeScope struct {
	Machine   string
	Workspace string
	Cwd       string
}

func (sc WorktreeScope) args() []string {
	if sc.Workspace != "" {
		return []string{"--workspace", sc.Workspace}
	}
	return []string{"--cwd", sc.Cwd}
}

// Worktree es un checkout de Git conocido por herdr.
type Worktree struct {
	Branch   string `json:"branch"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Linked   bool   `json:"is_linked_worktree"`
	Detached bool   `json:"is_detached"`
	Bare     bool   `json:"is_bare"`
	Prunable bool   `json:"is_prunable"`
}

// WorktreeList devuelve los worktrees del repositorio del ámbito.
func (s Service) WorktreeList(ctx context.Context, sc WorktreeScope) ([]Worktree, error) {
	var res struct {
		Worktrees []Worktree `json:"worktrees"`
	}
	args := append([]string{"worktree", "list"}, sc.args()...)
	if err := herdr.Call(ctx, s.Runner, sc.Machine, args, &res); err != nil {
		return nil, err
	}
	return res.Worktrees, nil
}

// ErrInvalidBranch indica que el nombre de rama no es aceptable.
var ErrInvalidBranch = errors.New("branch must be text without control characters and must not start with \"-\"")

// ValidBranch comprueba un nombre de rama o de referencia base antes de dárselo
// a herdr: no vacío, sin caracteres de control y sin "-" inicial (se leería
// como una opción).
func ValidBranch(branch string) error {
	if strings.TrimSpace(branch) == "" || strings.HasPrefix(branch, "-") || strings.ContainsFunc(branch, unicode.IsControl) {
		return fmt.Errorf("%q: %w", branch, ErrInvalidBranch)
	}
	return nil
}

// WorktreeNewOptions modifica la creación de un worktree.
type WorktreeNewOptions struct {
	Branch  string
	Base    string
	Label   string
	NoFocus bool
}

// WorktreeNew crea un worktree con una rama nueva y devuelve el resultado
// tal como lo informa herdr (su forma no está fijada por hum).
func (s Service) WorktreeNew(ctx context.Context, sc WorktreeScope, opts WorktreeNewOptions) (json.RawMessage, error) {
	if err := ValidBranch(opts.Branch); err != nil {
		return nil, err
	}
	args := append([]string{"worktree", "create"}, sc.args()...)
	args = append(args, "--branch", opts.Branch)
	if opts.Base != "" {
		if err := ValidBranch(opts.Base); err != nil {
			return nil, err
		}
		args = append(args, "--base", opts.Base)
	}
	if opts.Label != "" {
		if err := ValidLabel(opts.Label); err != nil {
			return nil, err
		}
		args = append(args, "--label", opts.Label)
	}
	if opts.NoFocus {
		args = append(args, "--no-focus")
	}
	var res json.RawMessage
	if err := herdr.Call(ctx, s.Runner, sc.Machine, args, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// WorktreeRemove borra el checkout del worktree de ese workspace. Es
// destructivo: quien llama debe haber confirmado antes.
func (s Service) WorktreeRemove(ctx context.Context, sc WorktreeScope, force bool) error {
	args := append([]string{"worktree", "remove"}, sc.args()...)
	if force {
		args = append(args, "--force")
	}
	return herdr.Call(ctx, s.Runner, sc.Machine, args, nil)
}
