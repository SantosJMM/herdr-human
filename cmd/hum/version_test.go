package main

import (
	"runtime"
	"strings"
	"testing"

	herdrhuman "github.com/SantosJMM/herdr-human"
)

func TestVersionLineHasManifestVersionAndPlatform(t *testing.T) {
	got := versionLine()
	if !strings.HasPrefix(got, herdrhuman.Version()+" (") {
		t.Errorf("must start with the manifest version: %q", got)
	}
	if !strings.Contains(got, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Errorf("must name the platform: %q", got)
	}
}
