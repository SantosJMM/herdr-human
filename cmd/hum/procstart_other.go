//go:build !windows

package main

import "time"

// processStart devuelve el instante de arranque conocido. Fuera de Windows no se
// consulta al sistema: la traza empieza en main, así que no incluye la carga del
// ejecutable.
func processStart() time.Time { return time.Now() }
