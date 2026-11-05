package trace

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestMarkWritesTheTimeTheProcessAndTheElapsedTime(t *testing.T) {
	var buf bytes.Buffer
	start := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	tr := New(&buf, "palette", start)
	tr.now = func() time.Time { return start.Add(83*time.Millisecond + 200*time.Microsecond) }
	tr.Mark("first frame written")

	line := strings.TrimSpace(buf.String())
	want := regexp.MustCompile(`^2026-11-05T10:00:00\.083Z pid=\d+ palette \+83\.2ms first frame written$`)
	if !want.MatchString(line) {
		t.Errorf("unexpected line %q", line)
	}
}

func TestMarkSinceAddsTheTimeFromAnotherProcessTrigger(t *testing.T) {
	var buf bytes.Buffer
	start := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	tr := New(&buf, "palette", start)
	tr.now = func() time.Time { return start.Add(40 * time.Millisecond) }
	tr.MarkSince("first frame written", start.Add(-60*time.Millisecond))
	if !strings.Contains(buf.String(), "first frame written (100.0ms since the trigger)") {
		t.Errorf("got %q", buf.String())
	}
}

func TestANilTracerDoesNothing(t *testing.T) {
	var tr *Tracer
	tr.Mark("x")
	tr.MarkSince("x", time.Now())
}

func TestEnabledByEnvironmentOrByTheMarkerFile(t *testing.T) {
	dir := t.TempDir()
	if Enabled(env(nil), dir) {
		t.Error("off by default")
	}
	for _, v := range []string{"1", "true", `C:\x\trace.log`} {
		if !Enabled(env(map[string]string{"HUM_TRACE": v}), dir) {
			t.Errorf("HUM_TRACE=%q should enable the trace", v)
		}
	}
	if Enabled(env(map[string]string{"HUM_TRACE": "0"}), dir) {
		t.Error("HUM_TRACE=0 is off")
	}
	if err := SetEnabled(dir, true); err != nil {
		t.Fatal(err)
	}
	if !Enabled(env(nil), dir) {
		t.Error("the marker file turns it on for processes that cannot see the shell environment (popups)")
	}
	if err := SetEnabled(dir, false); err != nil || Enabled(env(nil), dir) {
		t.Errorf("turning it off removes the marker: %v", err)
	}
	if err := SetEnabled(dir, false); err != nil {
		t.Errorf("turning it off twice is fine: %v", err)
	}
	if Enabled(env(nil), "") {
		t.Error("without a directory there is no marker to find")
	}
}

func TestLogPathFollowsHUMTRACEWhenItLooksLikeAPath(t *testing.T) {
	if got := LogPath(env(map[string]string{"HUM_TRACE": "1"}), "/cache"); got != filepath.Join("/cache", "trace.log") {
		t.Errorf("got %q", got)
	}
	if got := LogPath(env(map[string]string{"HUM_TRACE": "/tmp/mine.log"}), "/cache"); got != "/tmp/mine.log" {
		t.Errorf("got %q", got)
	}
}

func TestOpenAppendsAndStartsOverWhenTheFileGrowsTooMuch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "trace.log")
	for _, text := range []string{"one\n", "two\n"} {
		f, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.WriteString(text)
		_ = f.Close()
	}
	if data, _ := os.ReadFile(path); string(data) != "one\ntwo\n" {
		t.Errorf("lines are appended, got %q", data)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxLogBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("fresh\n")
	_ = f.Close()
	if data, _ := os.ReadFile(path); string(data) != "fresh\n" {
		t.Errorf("an oversized log starts over, got %d bytes", len(data))
	}
}

func TestTailKeepsTheLastLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.log")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := Tail(path, 2); err != nil || got != "c\nd\n" {
		t.Errorf("got %q %v", got, err)
	}
	if got, _ := Tail(path, 10); got != "a\nb\nc\nd\n" {
		t.Errorf("got %q", got)
	}
	if _, err := Tail(filepath.Join(t.TempDir(), "none"), 3); err == nil {
		t.Error("a missing log is an error the caller can explain")
	}
}

func TestTriggerRoundTrip(t *testing.T) {
	at := time.UnixMilli(1793873711707)
	got, ok := ParseTrigger(FormatTrigger(at))
	if !ok || !got.Equal(at) {
		t.Errorf("got %v %v", got, ok)
	}
	for _, bad := range []string{"", "abc", "-5", "0"} {
		if _, ok := ParseTrigger(bad); ok {
			t.Errorf("%q must not parse", bad)
		}
	}
}
