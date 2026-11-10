// Paquete complete calcula el autocompletado de hum para bash, zsh y PowerShell.
// No toca la red ni el disco: la fuente de datos (agentes y máquinas) se inyecta,
// así que se prueba sin herdr y quien lo use decide de dónde salen los nombres.
package complete

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/inventory"
)

// Source da los datos que hacen falta para sugerir nombres. Se pregunta solo lo
// que la posición actual necesita (Targets para un agente, Machines para una
// máquina), porque cada pregunta puede costar una consulta.
type Source interface {
	Targets() []inventory.Target
	Machines() []string
	// Aliases devuelve los alias guardados (nombre -> destino).
	Aliases() map[string]string
}

// Candidate es una sugerencia con una descripción opcional.
type Candidate struct{ Value, Desc string }

// Result son las sugerencias para una línea. Files pide a la shell que complete
// rutas de archivo (cuando lo que se espera es una carpeta).
type Result struct {
	Candidates []Candidate
	Files      bool
}

// Flag es una opción de un subcomando; Value dice si lleva un valor detrás.
type Flag struct {
	Name  string
	Value bool
}

// Shells son las shells con script de autocompletado.
var Shells = []string{"bash", "zsh", "powershell"}

var (
	statuses       = []string{"idle", "working", "blocked", "done", "unknown"}
	colors         = []string{"auto", "always", "never"}
	machineActions = []string{"enable", "disable", "toggle", "menu"}
)

// order es el orden en que se sugieren los subcomandos.
var order = []string{"ls", "id", "say", "ask", "attention", "look", "name", "rename", "close", "worktree", "alias", "go", "wait", "pick", "new", "palette", "keys", "machines", "completion", "version", "help"}

// whoCommands son los subcomandos cuyo primer argumento es un agente.
var whoCommands = map[string]bool{"id": true, "say": true, "ask": true, "look": true, "name": true, "rename": true, "close": true, "go": true, "wait": true}

func f(name string) Flag  { return Flag{Name: name} }
func fv(name string) Flag { return Flag{Name: name, Value: true} }

// base son las opciones de los comandos que consultan el inventario.
func base() []Flag { return []Flag{f("--local"), fv("--machine"), fv("--server-timeout")} }

// cached añade --fresh, de los comandos que usan la caché.
func cached() []Flag { return append(base(), f("--fresh")) }

var flags = map[string][]Flag{
	"ls":        append(cached(), f("--json"), f("--verbose"), fv("--color"), f("--full-names")),
	"id":        append(cached(), f("--json"), f("--cmd")),
	"say":       append(cached(), f("--wait"), fv("--timeout")),
	"ask":       append(cached(), fv("--timeout"), fv("--lines")),
	"attention": append(base(), f("--json"), fv("--color"), f("--full-names"), f("--watch"), fv("--interval"), f("--notify")),
	"look":      append(cached(), fv("--lines")),
	"name":      append(cached(), f("--clear")),
	"rename":    append(cached(), f("--tab"), f("--workspace")),
	"close":     append(cached(), f("--tab"), f("--workspace"), f("--yes")),
	"worktree":  append(cached(), fv("--base"), fv("--label"), f("--no-focus"), f("--force"), f("--yes")),
	"alias":     {},
	"go":        cached(),
	"wait":      append(cached(), fv("--until"), fv("--timeout"), f("--notify")),
	"pick":      append(base(), fv("--color"), f("--full-names"), f("--embedded")),
	"new":       {fv("--name"), fv("--machine"), f("--no-focus"), fv("--timeout")},
	"palette":   {f("--embedded")},
	"keys":      {f("--all"), f("--force")},
	"machines":  {f("--json")},
}

// Subcommands devuelve los subcomandos que se sugieren, en orden.
func Subcommands() []string { return append([]string(nil), order...) }

// Flags devuelve las opciones que se sugieren para un subcomando.
func Flags(cmd string) []Flag { return append([]Flag(nil), flags[cmd]...) }

// Tokenize parte una línea en palabras: agrupa lo que va entre comillas simples
// o dobles y, si la línea acaba en un espacio, añade una palabra vacía (la que se
// está empezando a escribir). No interpreta barras invertidas, para no romper
// las rutas de Windows.
func Tokenize(line string) []string {
	var (
		tokens []string
		cur    strings.Builder
		quote  rune
		inWord bool
	)
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				tokens = append(tokens, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		return append(tokens, cur.String())
	}
	return append(tokens, "")
}

// Complete devuelve las sugerencias para la línea (que incluye el nombre del
// programa como primera palabra) hasta donde está el cursor. src puede ser nulo.
func Complete(line string, src Source) Result {
	tokens := Tokenize(line)
	words := tokens[1:]
	if len(words) == 0 {
		words = []string{""}
	}
	cur := words[len(words)-1]
	before := words[:len(words)-1]

	if len(words) == 1 {
		return filter(plain(order), cur)
	}
	cmd := words[0]
	spec, known := flags[cmd]
	if !known && cmd != "completion" && cmd != "help" {
		return Result{}
	}

	// Una opción con su valor pegado: --color=al
	if name, val, found := strings.Cut(cur, "="); found && strings.HasPrefix(name, "--") {
		if values, ok := flagValues(spec, name, src); ok {
			return withPrefix(filter(values, val), name+"=")
		}
		return Result{}
	}
	if strings.HasPrefix(cur, "-") {
		var names []Candidate
		for _, fl := range spec {
			names = append(names, Candidate{Value: fl.Name})
		}
		return filter(names, cur)
	}
	// El valor de una opción que lo lleva: --color [aquí]
	if len(before) > 1 {
		if prev := before[len(before)-1]; strings.HasPrefix(prev, "-") {
			if fl, ok := lookup(spec, prev); ok && fl.Value {
				values, _ := flagValues(spec, prev, src)
				return filter(values, cur)
			}
		}
	}

	return positional(cmd, spec, before[1:], cur, src)
}

// positional completa un argumento sin opción; args son las palabras anteriores
// del subcomando (opciones incluidas).
func positional(cmd string, spec []Flag, args []string, cur string, src Source) Result {
	var pos []string // argumentos posicionales anteriores, sin opciones ni sus valores
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if fl, ok := lookup(spec, strings.SplitN(a, "=", 2)[0]); ok && fl.Value && !strings.Contains(a, "=") {
				i++ // salta el valor
			}
			continue
		}
		pos = append(pos, a)
	}
	idx := len(pos)

	switch {
	case whoCommands[cmd] && idx == 0:
		if src == nil {
			return Result{}
		}
		return filter(whoWithAliases(src), cur)
	case cmd == "worktree" && idx == 0:
		return filter(plain([]string{"ls", "new", "rm"}), cur)
	case cmd == "worktree" && src != nil && ((idx == 1 && (pos[0] == "ls" || pos[0] == "rm")) || (idx == 2 && pos[0] == "new")):
		return filter(whoWithAliases(src), cur)
	case cmd == "alias" && idx == 0:
		return filter(plain([]string{"set", "rm", "ls"}), cur)
	case cmd == "alias" && idx == 1 && pos[0] == "rm":
		if src == nil {
			return Result{}
		}
		return filter(aliasCandidates(src.Aliases(), false), cur)
	case cmd == "alias" && idx == 2 && pos[0] == "set":
		if src == nil {
			return Result{}
		}
		// El destino de un alias es un agente de verdad: no se sugieren otros alias.
		return filter(whoCandidates(src.Targets()), cur)
	case cmd == "new" && idx == 0:
		return filter(kindCandidates(cur), cur)
	case cmd == "new" && idx == 1:
		return Result{Files: true}
	case cmd == "completion" && idx == 0:
		return filter(plain(Shells), cur)
	case cmd == "machines" && idx == 0:
		return filter(plain(machineActions), cur)
	case cmd == "machines" && idx == 1 && (pos[0] == "enable" || pos[0] == "disable" || pos[0] == "toggle"):
		if src == nil {
			return Result{}
		}
		return filter(safe(src.Machines()), cur)
	}
	return Result{}
}

func lookup(spec []Flag, name string) (Flag, bool) {
	for _, fl := range spec {
		if fl.Name == name {
			return fl, true
		}
	}
	return Flag{}, false
}

// flagValues devuelve los valores que se pueden sugerir para una opción.
func flagValues(spec []Flag, name string, src Source) ([]Candidate, bool) {
	if fl, ok := lookup(spec, name); !ok || !fl.Value {
		return nil, false
	}
	switch name {
	case "--color":
		return plain(colors), true
	case "--until":
		return plain(statuses), true
	case "--machine":
		out := plain([]string{"local"})
		if src != nil {
			out = append(out, safe(src.Machines())...)
		}
		return out, true
	}
	return nil, true // numérico o texto libre: no hay nada que sugerir
}

// whoCandidates son los nombres que hum acepta para un agente: su pane id con la
// máquina ("odoous/w1:p2"), y su nombre y "maquina/nombre" si el nombre es de los
// que hum entiende (los títulos con espacios y barras no se sugieren).
func whoCandidates(targets []inventory.Target) []Candidate {
	seen := map[string]bool{}
	var out []Candidate
	add := func(value, desc string) {
		if value == "" || seen[value] || clean(value) != value {
			return
		}
		seen[value] = true
		out = append(out, Candidate{Value: value, Desc: desc})
	}
	for _, t := range targets {
		desc := clean(strings.TrimSpace(t.Agent + " " + t.Status + "  " + shortTitle(t.Name)))
		add(t.Ref(), desc)
		if actions.ValidName(t.Name) == nil {
			add(t.Name, desc)
			if t.Machine != "" {
				add(t.MachineLabel()+"/"+t.Name, desc)
			}
		}
	}
	return out
}

func shortTitle(name string) string {
	first, _, _ := strings.Cut(name, " | ")
	return strings.TrimSpace(first)
}

// kindCandidates sugiere los tipos de agente, también como "maquina/tipo".
func kindCandidates(cur string) []Candidate {
	prefix := ""
	if m, _, found := strings.Cut(cur, "/"); found {
		prefix = m + "/"
	}
	out := make([]Candidate, 0, len(actions.Kinds))
	for _, k := range actions.Kinds {
		out = append(out, Candidate{Value: prefix + k})
	}
	return out
}

func plain(values []string) []Candidate {
	out := make([]Candidate, len(values))
	for i, v := range values {
		out[i] = Candidate{Value: v}
	}
	return out
}

// safe descarta los valores que llevarían caracteres de control a la shell.
// whoWithAliases sugiere los alias y, después, los agentes.
func whoWithAliases(src Source) []Candidate {
	out := aliasCandidates(src.Aliases(), true)
	seen := map[string]bool{}
	for _, c := range out {
		seen[c.Value] = true
	}
	for _, c := range whoCandidates(src.Targets()) {
		if !seen[c.Value] { // un alias gana: aparece una sola vez
			out = append(out, c)
		}
	}
	return out
}

// aliasCandidates sugiere los nombres de alias ordenados; con describe, cada
// uno lleva a dónde apunta. Lo guardado en el archivo no es de fiar: se limpia.
func aliasCandidates(aliases map[string]string, describe bool) []Candidate {
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		if name != "" && clean(name) == name {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]Candidate, 0, len(names))
	for _, name := range names {
		c := Candidate{Value: name}
		if describe {
			c.Desc = clean("alias -> " + aliases[name])
		}
		out = append(out, c)
	}
	return out
}

func safe(values []string) []Candidate {
	var out []Candidate
	for _, v := range values {
		if v != "" && clean(v) == v {
			out = append(out, Candidate{Value: v})
		}
	}
	return out
}

// clean quita los caracteres de control (escapes, saltos de línea, tabuladores)
// y recorta lo demasiado largo: lo que viene de un agente o de la configuración
// no es de fiar y va a parar a la línea de comandos.
func clean(s string) string {
	var sb strings.Builder
	n := 0
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		if n++; n > 100 {
			break
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// filter conserva los candidatos que empiezan por cur, sin distinguir mayúsculas.
func filter(cs []Candidate, cur string) Result {
	var out []Candidate
	for _, c := range cs {
		if len(c.Value) >= len(cur) && strings.EqualFold(c.Value[:prefixLen(c.Value, cur)], cur) {
			out = append(out, c)
		}
	}
	return Result{Candidates: out}
}

// prefixLen devuelve cuántos bytes de v ocupan tantas letras como tiene cur.
func prefixLen(v, cur string) int {
	n, i := utf8.RuneCountInString(cur), 0
	for ; n > 0 && i < len(v); n-- {
		_, size := utf8.DecodeRuneInString(v[i:])
		i += size
	}
	return i
}

func withPrefix(r Result, prefix string) Result {
	for i := range r.Candidates {
		r.Candidates[i].Value = prefix + r.Candidates[i].Value
	}
	return r
}
