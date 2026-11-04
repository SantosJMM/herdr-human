// Paquete gitinfo averigua la rama de git de una carpeta leyendo los archivos de
// .git, sin ejecutar git: es de solo lectura y no depende de que git esté
// instalado.
package gitinfo

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// maxFile acota lo que se lee de HEAD o de un puntero .git: son archivos de
	// una línea, así que algo más grande no es un repositorio de fiar.
	maxFile = 4096
	// maxDepth acota cuántas carpetas se suben buscando el repositorio.
	maxDepth = 32
)

// Branch devuelve la rama activa del repositorio que contiene dir ("main"), o
// los siete primeros caracteres del commit si HEAD está separado. Devuelve ""
// si dir no está en un repositorio o no se puede leer.
func Branch(dir string) string {
	if dir == "" {
		return ""
	}
	d := filepath.Clean(dir)
	for range maxDepth {
		gitPath := filepath.Join(d, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			gitDir := gitPath
			if !info.IsDir() {
				// Worktree o submódulo: .git es un archivo "gitdir: <ruta>".
				if gitDir = pointer(gitPath, d); gitDir == "" {
					return ""
				}
			}
			return headName(gitDir)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
	return ""
}

// pointer resuelve el "gitdir: ruta" de un archivo .git; las rutas relativas lo
// son respecto a la carpeta que lo contiene.
func pointer(file, base string) string {
	line, ok := readSmall(file)
	if !ok {
		return ""
	}
	path, found := strings.CutPrefix(line, "gitdir:")
	if !found {
		return ""
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return path
}

func headName(gitDir string) string {
	head, ok := readSmall(filepath.Join(gitDir, "HEAD"))
	if !ok {
		return ""
	}
	if ref, found := strings.CutPrefix(head, "ref:"); found {
		ref = strings.TrimSpace(ref)
		if name, isBranch := strings.CutPrefix(ref, "refs/heads/"); isBranch && name != "" {
			return name
		}
		return ""
	}
	if isHash(head) {
		return head[:7]
	}
	return ""
}

func isHash(s string) bool {
	if len(s) < 7 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// readSmall lee la primera línea de un archivo pequeño; un archivo mayor que
// maxFile se rechaza en vez de truncarse.
func readSmall(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil || len(data) > maxFile {
		return "", false
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(line), true
}
