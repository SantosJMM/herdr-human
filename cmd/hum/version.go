package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	herdrhuman "github.com/SantosJMM/herdr-human"
)

// versionLine arma «0.1.2 (commit abc1234, windows/amd64)». El commit solo está
// si Go lo registró al compilar dentro de un repositorio; «+dirty» marca cambios
// sin confirmar.
func versionLine() string {
	parts := []string{}
	if info, ok := debug.ReadBuildInfo(); ok {
		var rev string
		dirty := false
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if len(rev) > 7 {
			rev = rev[:7]
		}
		if rev != "" {
			if dirty {
				rev += "+dirty"
			}
			parts = append(parts, "commit "+rev)
		}
	}
	parts = append(parts, runtime.GOOS+"/"+runtime.GOARCH)
	return fmt.Sprintf("%s (%s)", herdrhuman.Version(), strings.Join(parts, ", "))
}
