package main

import (
	"os"
	"os/exec"
)

// detach lanza este mismo programa con esos argumentos como un proceso
// independiente: sigue vivo cuando el actual termina y no comparte su consola
// ni sus flujos estándar.
func detach(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = detachedAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	// No se espera al hijo: se suelta el manejador para no dejar recursos abiertos.
	return cmd.Process.Release()
}
