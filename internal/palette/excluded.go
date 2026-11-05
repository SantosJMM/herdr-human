package palette

import "strings"

// Motivos por los que un comando de herdr no está en la paleta. Cada comando de
// herdr debe estar en commands.json o aquí, con su motivo: así, cuando una
// versión nueva de herdr añade comandos, el test de cobertura obliga a decidir
// qué hacer con ellos en vez de olvidarlos sin que nadie se entere.
const (
	reasonOutput      = "prints information; the palette runs the command detached, so nobody would see the output"
	reasonInternal    = "meant for agent integrations and scripts, not for people"
	reasonDestructive = "destructive or hard to undo: run it deliberately from a terminal, where it can ask first"
	reasonTerminal    = "needs this terminal (interactive or attaches the client), which a detached run does not have"
	reasonFreeForm    = "takes free-form arguments (text, keys, paths) that the palette cannot ask for properly"
	reasonThirdParty  = "installs, links or changes third-party code: do it deliberately from a terminal"
	reasonSetup       = "one-time setup or a rare, global change: not worth a palette row"
	reasonPlumbing    = "plugin plumbing that the palette itself already drives"
	reasonHum         = "has a proper interactive flow in a hum command (pick, ask, name, new, machines)"
)

// excluded son los comandos de herdr que se dejan fuera de la paleta a propósito.
// La clave es la ruta del comando sin "herdr", por ejemplo "pane read".
var excluded = map[string]string{
	// Solo muestran información.
	"workspace list": reasonOutput, "workspace get": reasonOutput, "tab list": reasonOutput, "tab get": reasonOutput,
	"worktree list": reasonOutput, "pane list": reasonOutput, "pane current": reasonOutput, "pane get": reasonOutput,
	"pane layout": reasonOutput, "pane process-info": reasonOutput, "pane neighbor": reasonOutput, "pane edges": reasonOutput,
	"pane read": reasonOutput, "pane wait-output": reasonOutput, "agent list": reasonOutput, "agent get": reasonOutput,
	"agent read": reasonOutput, "agent wait": reasonOutput, "agent explain": reasonOutput, "machine list": reasonOutput,
	"machine status": reasonOutput, "config check": reasonOutput, "channel show": reasonOutput, "session list": reasonOutput,
	"integration status": reasonOutput, "server agent-manifests": reasonOutput, "api snapshot": reasonOutput,
	"api schema": reasonOutput, "plugin list": reasonOutput, "plugin config-dir": reasonOutput,
	"plugin action list": reasonOutput, "plugin log list": reasonOutput,

	// Los usan las integraciones de agentes y los scripts.
	"pane report-agent": reasonInternal, "pane report-agent-session": reasonInternal, "pane release-agent": reasonInternal,
	"pane report-metadata": reasonInternal, "workspace report-metadata": reasonInternal, "notification show": reasonInternal,

	// Destructivos o difíciles de deshacer.
	"worktree remove": reasonDestructive, "machine remove": reasonDestructive,
	"config reset-keys": reasonDestructive, "session stop": reasonDestructive, "session delete": reasonDestructive,
	"server stop": reasonDestructive, "plugin uninstall": reasonDestructive, "integration uninstall": reasonDestructive,

	// Necesitan esta terminal.
	"agent attach": reasonTerminal, "session attach": reasonTerminal, "machine reconnect": reasonTerminal,

	// Argumentos libres que la paleta no sabe pedir bien.
	"pane input": reasonFreeForm, "pane send-text": reasonFreeForm, "pane send-keys": reasonFreeForm, "pane run": reasonFreeForm,
	"agent send-keys": reasonFreeForm, "machine add": reasonFreeForm, "machine rename": reasonFreeForm,

	// Código de terceros.
	"plugin install": reasonThirdParty, "plugin update": reasonThirdParty, "plugin link": reasonThirdParty,
	"plugin unlink": reasonThirdParty, "server update-agent-manifests": reasonThirdParty,

	// Se hace una vez o casi nunca.
	"integration install": reasonSetup, "channel set": reasonSetup,

	// La propia paleta ya lo maneja.
	"plugin action invoke": reasonPlumbing, "plugin pane open": reasonPlumbing, "plugin pane focus": reasonPlumbing,
	"plugin pane close": reasonPlumbing,

	// hum tiene un flujo propio, con preguntas.
	"agent prompt": reasonHum, "agent rename": reasonHum, "agent start": reasonHum,
}

// Excluded devuelve, por comando de herdr, el motivo de que no esté en la paleta.
func Excluded() map[string]string {
	out := make(map[string]string, len(excluded))
	for k, v := range excluded {
		out[k] = v
	}
	return out
}

// CatalogCommands devuelve los comandos de herdr distintos que usa el catálogo,
// como rutas sin "herdr" (por ejemplo "pane move"), con las entradas que los usan.
func CatalogCommands() (map[string][]string, error) {
	entries, err := Native()
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, e := range entries {
		if e.Kind != KindNative {
			continue // los comandos de hum no son comandos de herdr
		}
		key := strings.Join(e.Command, " ")
		out[key] = append(out[key], e.ID)
	}
	return out, nil
}
