package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Estas pruebas ejecutan los scripts REALES de instalación (scripts/install.sh y
// scripts/install.ps1) contra un servidor HTTP local que hace de GitHub Releases.
// No tocan la red ni el herdr de nadie. Si falta el intérprete, la prueba se
// omite y lo dice (una omisión no es un resultado correcto).

const (
	fakeVersion = "9.9.9"
	goodBinary  = "downloaded hum"
	builtBinary = "built by go"
)

// shellKind describe cómo se ejecuta cada script y qué activo debe pedir.
type shellKind struct {
	name   string
	script string
	asset  string // nombre del archivo del release para el entorno simulado
	binary string // dónde debe quedar el binario dentro de la raíz del plugin
	// command devuelve el comando que ejecuta el script, o "" si no hay intérprete.
	command func(root string) *exec.Cmd
	// stubs crea los falsos que simulan la plataforma y devuelve variables de entorno.
	stubs func(t *testing.T, dir string, unsupported bool) []string
	// goStub escribe un go falso y devuelve su ruta.
	goStub func(t *testing.T, dir string) string
}

func shellKinds() []shellKind {
	return []shellKind{
		{
			name: "sh", script: "install.sh", asset: "hum-linux-amd64", binary: filepath.Join("bin", "hum"),
			command: func(root string) *exec.Cmd {
				sh, err := exec.LookPath("sh")
				if err != nil {
					return nil
				}
				c := exec.Command(sh, "scripts/install.sh")
				c.Dir = root
				return c
			},
			stubs: func(t *testing.T, dir string, unsupported bool) []string {
				// uname falso: el script debe creer que es Linux x86_64 (o Plan9).
				system := "Linux"
				if unsupported {
					system = "Plan9"
				}
				writeFile(t, filepath.Join(dir, "uname"), "#!/bin/sh\ncase \"$1\" in -m) echo x86_64;; *) echo "+system+";; esac\n", 0o755)
				return []string{"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")}
			},
			goStub: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "gostub")
				writeFile(t, p, `#!/bin/sh
echo "$@" > "$(dirname "$0")/go-called"
[ -n "$GO_STUB_FAIL" ] && { echo "boom from go" >&2; exit 1; }
while [ $# -gt 0 ]; do
  if [ "$1" = "-o" ]; then shift; mkdir -p "$(dirname "$1")"; printf '`+builtBinary+`' > "$1"; fi
  shift
done
`, 0o755)
				return p
			},
		},
		{
			name: "powershell", script: "install.ps1", asset: "hum-windows-amd64.exe", binary: filepath.Join("bin", "hum.exe"),
			command: func(root string) *exec.Cmd {
				// El manifest usa "powershell" (5.1 en Windows): se prueba ese mismo.
				exe, err := exec.LookPath("powershell")
				if err != nil {
					exe, err = exec.LookPath("pwsh")
				}
				if err != nil {
					return nil
				}
				c := exec.Command(exe, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/install.ps1")
				c.Dir = root
				return c
			},
			stubs: func(t *testing.T, dir string, unsupported bool) []string {
				arch := "AMD64"
				if unsupported {
					arch = "x86"
				}
				return []string{"PROCESSOR_ARCHITECTURE=" + arch}
			},
			goStub: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "gostub.ps1")
				writeFile(t, p, `$dir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Content -LiteralPath (Join-Path $dir 'go-called') -Value ($args -join ' ')
if ($env:GO_STUB_FAIL) { [Console]::Error.WriteLine('boom from go'); exit 1 }
for ($i = 0; $i -lt $args.Count; $i++) {
  if ($args[$i] -eq '-o') {
    $o = $args[$i + 1]
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $o) | Out-Null
    [IO.File]::WriteAllText($o, '`+builtBinary+`')
  }
}
exit 0
`, 0o644)
				return p
			},
		},
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

type result struct {
	stdout, stderr string
	code           int
	root, stubDir  string
}

func (r result) has(rel string) bool {
	_, err := os.Stat(filepath.Join(r.root, rel))
	return err == nil
}

func (r result) read(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(r.root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// scenario es lo que publica el servidor y qué go hay disponible.
type scenario struct {
	files  map[string]string // ruta bajo /v<versión>/ -> contenido; nil = sin release (404)
	goMode string            // "stub", "fail" o "none"
	env    []string
	// unsupported simula una plataforma para la que no se publican binarios.
	unsupported bool
}

func install(t *testing.T, k shellKind, sc scenario) result {
	t.Helper()
	cmd := k.command(t.TempDir())
	if cmd == nil {
		t.Skipf("no interpreter for %s on this machine: NOT checked", k.name)
	}
	root := cmd.Dir

	// La raíz del plugin: scripts reales + un manifest con una versión de mentira.
	src, err := os.ReadFile(filepath.Join("..", "..", "scripts", k.script))
	if err != nil {
		t.Fatalf("the install script is missing: %v", err)
	}
	writeFile(t, filepath.Join(root, "scripts", k.script), string(src), 0o755)
	writeFile(t, filepath.Join(root, "herdr-plugin.toml"), "id = \"herdr-human\"\nversion = \""+fakeVersion+"\"\n", 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := "/v" + fakeVersion + "/"
		if body, ok := sc.files[strings.TrimPrefix(r.URL.Path, prefix)]; ok && strings.HasPrefix(r.URL.Path, prefix) {
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	stubDir := t.TempDir()
	// Sin PROCESSOR_ARCHITEW6432 del anfitrión (un valor vacío rompería el
	// entorno de PowerShell), para que la arquitectura la decida la prueba.
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(kv), "PROCESSOR_ARCHITEW6432=") {
			env = append(env, kv)
		}
	}
	env = append(env, "HUM_BASE_URL="+srv.URL, "HUM_RETRY_SLEEP=0")
	env = append(env, k.stubs(t, stubDir, sc.unsupported)...)
	switch sc.goMode {
	case "stub":
		env = append(env, "HUM_GO="+k.goStub(t, stubDir))
	case "fail":
		env = append(env, "HUM_GO="+k.goStub(t, stubDir), "GO_STUB_FAIL=1")
	default:
		env = append(env, "HUM_GO="+filepath.Join(stubDir, "no-such-go"))
	}
	cmd.Env = append(env, sc.env...)

	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	code := 0
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("could not run the script: %v", err)
		}
		code = ee.ExitCode()
	}
	return result{stdout: out.String(), stderr: errb.String(), code: code, root: root, stubDir: stubDir}
}

func release(k shellKind, binary string) map[string]string {
	return map[string]string{
		k.asset:      binary,
		"SHA256SUMS": sha(binary) + "  " + k.asset + "\n",
	}
}

func goWasCalled(r result) bool {
	_, err := os.Stat(filepath.Join(r.stubDir, "go-called"))
	return err == nil
}

func TestInstallScripts(t *testing.T) {
	for _, k := range shellKinds() {
		t.Run(k.name, func(t *testing.T) {
			t.Run("downloads and verifies the release binary", func(t *testing.T) {
				r := install(t, k, scenario{files: release(k, goodBinary), goMode: "stub"})
				if r.code != 0 {
					t.Fatalf("exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
				}
				if got := r.read(t, k.binary); got != goodBinary {
					t.Errorf("binary = %q, want the downloaded one\nstderr: %s", got, r.stderr)
				}
				if goWasCalled(r) {
					t.Error("go was used although the download worked")
				}
			})

			t.Run("a checksum mismatch is never installed and falls back to go", func(t *testing.T) {
				files := release(k, goodBinary)
				files["SHA256SUMS"] = sha("something else") + "  " + k.asset + "\n"
				r := install(t, k, scenario{files: files, goMode: "stub"})
				if r.code != 0 {
					t.Fatalf("exit %d\nstderr: %s", r.code, r.stderr)
				}
				if got := r.read(t, k.binary); got != builtBinary {
					t.Errorf("binary = %q, want the one built by go (the bad download must not stay)", got)
				}
				if !strings.Contains(strings.ToLower(r.stderr), "checksum") {
					t.Errorf("stderr should say the checksum did not match:\n%s", r.stderr)
				}
			})

			t.Run("a release that does not exist falls back to go", func(t *testing.T) {
				r := install(t, k, scenario{goMode: "stub"})
				if r.code != 0 {
					t.Fatalf("exit %d\nstderr: %s", r.code, r.stderr)
				}
				if got := r.read(t, k.binary); got != builtBinary {
					t.Errorf("binary = %q, want the one built by go", got)
				}
				if !strings.Contains(r.stderr, "building from source") {
					t.Errorf("stderr should say it builds from source:\n%s", r.stderr)
				}
			})

			t.Run("no release and no go gives a clear message", func(t *testing.T) {
				r := install(t, k, scenario{goMode: "none"})
				if r.code == 0 {
					t.Fatalf("exit 0, want a failure\nstderr: %s", r.stderr)
				}
				for _, want := range []string{"Go", "https://go.dev/dl/", "herdr plugin install SantosJMM/herdr-human"} {
					if !strings.Contains(r.stderr, want) {
						t.Errorf("stderr missing %q:\n%s", want, r.stderr)
					}
				}
				if r.has(k.binary) {
					t.Error("a binary was left behind after a failed install")
				}
			})

			t.Run("a platform without a published binary falls back to go", func(t *testing.T) {
				// Aunque el release exista, esta plataforma no tiene binario.
				r := install(t, k, scenario{files: release(k, goodBinary), goMode: "stub", unsupported: true})
				if r.code != 0 {
					t.Fatalf("exit %d\nstderr: %s", r.code, r.stderr)
				}
				if got := r.read(t, k.binary); got != builtBinary {
					t.Errorf("binary = %q, want the one built by go", got)
				}
			})

			t.Run("go failing is reported as a failure", func(t *testing.T) {
				r := install(t, k, scenario{goMode: "fail"})
				if r.code == 0 {
					t.Fatalf("exit 0, want a failure\nstderr: %s", r.stderr)
				}
				if r.has(k.binary) {
					t.Error("a binary was left behind after a failed build")
				}
			})

			t.Run("go is asked for the plugin command", func(t *testing.T) {
				r := install(t, k, scenario{goMode: "stub"})
				called, err := os.ReadFile(filepath.Join(r.stubDir, "go-called"))
				if err != nil {
					t.Fatalf("go was not called: %v\nstderr: %s", err, r.stderr)
				}
				if !strings.Contains(string(called), "build") || !strings.Contains(string(called), "./cmd/hum") {
					t.Errorf("go was called as %q, want a build of ./cmd/hum", called)
				}
			})
		})
	}
}

func TestReleaseWorkflowPublishesWhatTheScriptsAsk(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("the release workflow is missing: %v", err)
	}
	wf := string(data)
	for _, asset := range []string{
		"hum-linux-amd64", "hum-linux-arm64", "hum-darwin-amd64", "hum-darwin-arm64",
		"hum-windows-amd64.exe", "hum-windows-arm64.exe", "SHA256SUMS",
	} {
		if !strings.Contains(wf, asset) {
			t.Errorf("release.yml does not publish %s", asset)
		}
	}
	if !strings.Contains(wf, "CGO_ENABLED") {
		t.Error("release.yml should build with CGO_ENABLED=0 so binaries run anywhere")
	}
}

func TestManifestBuildsWithTheInstallScripts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "herdr-plugin.toml"))
	if err != nil {
		t.Fatal(err)
	}
	m := string(data)
	if n := strings.Count(m, "[[build]]"); n != 2 {
		t.Fatalf("manifest has %d [[build]] entries, want 2 (unix and windows)", n)
	}
	for _, want := range []string{`"scripts/install.sh"`, `"scripts/install.ps1"`} {
		if !strings.Contains(m, want) {
			t.Errorf("manifest does not run %s", want)
		}
	}
}
