package herdr_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/herdr"
)

// TestMain permite que este mismo binario haga de falso herdr: "parent" lanza un
// hijo que hereda stdout/stderr y sigue vivo, como hace herdr con ssh.
func TestMain(m *testing.M) {
	switch os.Getenv("HERDR_TEST_HELPER") {
	case "parent":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "HERDR_TEST_HELPER=child")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		time.Sleep(time.Minute)
		os.Exit(0)
	case "child":
		// Vive hasta que el test crea el archivo de parada (o 10 s como tope).
		stop := os.Getenv("HERDR_TEST_STOP")
		for range 200 {
			if _, err := os.Stat(stop); err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Si herdr se queda colgado y se vence el plazo, Run debe volver enseguida
// aunque un proceso hijo siga teniendo abiertas las tuberías.
func TestRunReturnsAtTheDeadlineEvenIfAChildKeepsThePipesOpen(t *testing.T) {
	stop := filepath.Join(t.TempDir(), "stop")
	t.Setenv("HERDR_TEST_HELPER", "parent")
	t.Setenv("HERDR_TEST_STOP", stop)
	// Al acabar se deja salir al hijo para que Go pueda borrar el ejecutable de prueba.
	t.Cleanup(func() {
		_ = os.WriteFile(stop, nil, 0o600)
		time.Sleep(300 * time.Millisecond)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := (&herdr.ExecRunner{Bin: os.Args[0]}).Run(ctx, "")
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want the deadline", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("Run took %v: it waited for the child that kept the pipes open", elapsed)
	}
}
