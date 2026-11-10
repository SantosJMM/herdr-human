// Package herdrhuman lleva el manifiesto del plugin dentro del binario, para que
// `hum version` diga la versión del manifiesto sin depender de cómo se compiló
// (go build, go install o el binario descargado del release).
package herdrhuman

import (
	_ "embed"
	"regexp"
)

//go:embed herdr-plugin.toml
var manifest string

var versionLine = regexp.MustCompile(`(?m)^version\s*=\s*"([^"]+)"\s*$`)

// Version es la versión que declara herdr-plugin.toml; "unknown" si no la tiene.
func Version() string {
	if m := versionLine.FindStringSubmatch(manifest); m != nil {
		return m[1]
	}
	return "unknown"
}
