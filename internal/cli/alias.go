package cli

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/SantosJMM/herdr-human/internal/alias"
)

// aliasFile es el nombre del archivo de alias dentro de Env.ConfigDir.
const aliasFile = "aliases.json"

const aliasUsage = `usage: hum alias set <name> <who>
       hum alias rm <name>
       hum alias ls`

// aliases devuelve el almacén de alias; sin carpeta de configuración no
// guarda nada.
func (a *app) aliases() alias.Store {
	if a.env.ConfigDir == "" {
		return alias.Store{}
	}
	return alias.Store{Path: filepath.Join(a.env.ConfigDir, aliasFile)}
}

// aliasCmd gestiona los alias. No consulta herdr: guardar un alias no comprueba
// que el agente exista, porque los agentes van y vienen; se resuelve al usarlo.
func (a *app) aliasCmd(args []string) error {
	if len(args) == 0 {
		return usagef("%s", aliasUsage)
	}
	verb, rest := args[0], args[1:]
	store := a.aliases()

	switch {
	case verb == "ls" && len(rest) == 0:
		all, err := store.List()
		if err != nil {
			return err
		}
		names := make([]string, 0, len(all))
		for name := range all {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(a.env.Stdout, "%s -> %s\n", name, all[name])
		}
		return nil

	case verb == "set":
		fs := newFlagSet("alias set")
		rest, err := parse(fs, rest)
		if err != nil {
			return err
		}
		if len(rest) != 2 {
			return usagef("%s", aliasUsage)
		}
		if err := store.Set(rest[0], rest[1]); err != nil {
			return err
		}
		fmt.Fprintf(a.env.Stdout, "alias %s -> %s\n", rest[0], rest[1])
		return nil

	case verb == "rm":
		fs := newFlagSet("alias rm")
		rest, err := parse(fs, rest)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return usagef("%s", aliasUsage)
		}
		if err := store.Remove(rest[0]); err != nil {
			return err
		}
		fmt.Fprintf(a.env.Stdout, "removed alias %s\n", rest[0])
		return nil
	}
	return usagef("%s", aliasUsage)
}
