package herdrhuman_test

import (
	"os"
	"regexp"
	"testing"

	herdrhuman "github.com/SantosJMM/herdr-human"
)

func TestVersionIsTheOneInThePluginManifest(t *testing.T) {
	data, err := os.ReadFile("herdr-plugin.toml")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^version = "([^"]+)"`).FindSubmatch(data)
	if m == nil {
		t.Fatal("the manifest has no version")
	}
	if got := herdrhuman.Version(); got != string(m[1]) || got == "unknown" {
		t.Errorf("Version() = %q, manifest says %q", got, m[1])
	}
}
