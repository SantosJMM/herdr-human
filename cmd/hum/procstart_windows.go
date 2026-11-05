package main

import (
	"syscall"
	"time"
)

// processStart devuelve el instante en que Windows creó este proceso. Así la
// traza incluye lo que pasa antes de main: cargar el .exe, el antivirus y el
// arranque del runtime de Go.
func processStart() time.Time {
	var created, exited, kernel, user syscall.Filetime
	handle, err := syscall.GetCurrentProcess()
	if err != nil || syscall.GetProcessTimes(handle, &created, &exited, &kernel, &user) != nil {
		return time.Now()
	}
	return time.Unix(0, created.Nanoseconds())
}
