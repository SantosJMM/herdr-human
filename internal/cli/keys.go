package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/keybind"
	"github.com/SantosJMM/herdr-human/internal/palette"
	"github.com/SantosJMM/herdr-human/internal/tui"
)

// keyStore arma el Store que edita el config.toml de herdr de esta máquina.
func (a *app) keyStore() keybind.Store {
	return keybind.Store{
		Path:  palette.ConfigPath(a.getenv),
		Check: a.env.CheckConfig,
		Reload: func(ctx context.Context) error {
			_, err := herdr.Text(ctx, a.env.Runner, "", []string{"server", "reload-config"})
			return err
		},
	}
}

// keyService implementa tui.KeyService sobre el config.toml.
type keyService struct {
	store keybind.Store
	goos  string
	// entries da las entradas conocidas, para nombrar con su título el atajo que
	// choca en vez de mostrar su comando.
	entries func() []palette.Entry
}

func (a *app) keyService(entries func() []palette.Entry) *keyService {
	return &keyService{store: a.keyStore(), goos: a.env.GOOS, entries: entries}
}

func (k *keyService) Bindable(e palette.Entry) (string, bool) {
	b, ok := keybind.SpecFor(e, k.goos)
	if !ok {
		return "", false
	}
	cfg, err := k.store.Read()
	if err != nil {
		return "", true // se puede enlazar; el motivo del fallo saldrá al guardar
	}
	return cfg.KeyOf(b), true
}

func (k *keyService) Check(e palette.Entry, key string) tui.KeyCheck {
	b, ok := keybind.SpecFor(e, k.goos)
	if !ok {
		return tui.KeyCheck{Problem: "this entry cannot be bound to a key"}
	}
	norm, err := keybind.Normalize(key)
	if err != nil {
		return tui.KeyCheck{Problem: err.Error()}
	}
	if !keybind.IsSafeDirect(norm) {
		return tui.KeyCheck{Key: norm, Problem: norm + " is a plain key: it would swallow what you type; use ctrl, alt or the prefix"}
	}
	cfg, err := k.store.Read()
	if err != nil {
		return tui.KeyCheck{Key: norm, Problem: err.Error()}
	}
	if cfg.KeyOf(b) != "" && keybind.SameKey(cfg.KeyOf(b), norm) {
		return tui.KeyCheck{Key: norm, Problem: norm + " is already its shortcut"}
	}
	check := tui.KeyCheck{Key: norm}
	for _, c := range cfg.Conflicts(norm, b) {
		if c.Blocking() {
			check.Problem = k.describe(c) + "; free it first"
			return check
		}
		check.Warning = k.describe(c) + "; herdr may keep either"
	}
	return check
}

// describe cuenta un choque, con el título de la entrada si se la conoce.
func (k *keyService) describe(c keybind.Conflict) string {
	if c.Source == keybind.SourceCustom && k.entries != nil {
		for _, e := range k.entries() {
			if b, ok := keybind.SpecFor(e, k.goos); ok && b.Command == c.Name {
				c.Name = e.Title
				break
			}
		}
	}
	return c.Describe()
}

func (k *keyService) Save(ctx context.Context, e palette.Entry, key string) (tui.KeySaved, error) {
	b, ok := keybind.SpecFor(e, k.goos)
	if !ok {
		return tui.KeySaved{}, errors.New("this entry cannot be bound to a key")
	}
	res, err := k.store.Save(ctx, b, key, e.Title)
	return saved(res, keyOrEmpty(key), err)
}

func (k *keyService) Remove(ctx context.Context, e palette.Entry) (tui.KeySaved, error) {
	b, ok := keybind.SpecFor(e, k.goos)
	if !ok {
		return tui.KeySaved{}, errors.New("this entry cannot be bound to a key")
	}
	res, err := k.store.Remove(ctx, b)
	return saved(res, "", err)
}

func keyOrEmpty(key string) string {
	if n, err := keybind.Normalize(key); err == nil {
		return n
	}
	return ""
}

func saved(res keybind.Result, key string, err error) (tui.KeySaved, error) {
	if err != nil {
		var rejected *keybind.RejectedError
		if errors.As(err, &rejected) {
			return tui.KeySaved{}, fmt.Errorf("herdr would not accept it, so nothing was changed: %s", strings.Join(rejected.Issues, "; "))
		}
		return tui.KeySaved{}, err
	}
	out := tui.KeySaved{Key: key}
	if res.ReloadErr != nil {
		out.Note = "saved, but herdr did not reload its configuration (" + res.ReloadErr.Error() + "): run `herdr server reload-config`"
	}
	return out, nil
}

// --- hum keys ---

const keysUsage = "usage: hum keys ls [--all] | set <entry> <key> [--force] | rm <entry>"

// keysCmd despacha `hum keys`: ver, asignar y quitar atajos de las entradas de la
// paleta desde la línea de comandos. Es lo mismo que hace la paleta con ctrl+k.
func (a *app) keysCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usagef("%s", keysUsage)
	}
	switch args[0] {
	case "ls":
		return a.keysList(ctx, args[1:])
	case "set":
		return a.keysSet(ctx, args[1:])
	case "rm":
		return a.keysRemove(ctx, args[1:])
	}
	return usagef("%s", keysUsage)
}

// bindableEntries son las entradas que se pueden enlazar a una tecla: los comandos
// de herdr que no piden nada y las acciones de todos los plugins.
func (a *app) bindableEntries(ctx context.Context) ([]palette.Entry, error) {
	native, err := palette.Native()
	if err != nil {
		return nil, err
	}
	plugins, err := palette.ListPlugins(ctx, a.env.Runner)
	if err != nil {
		return nil, fmt.Errorf("listing plugin actions: %w", err)
	}
	all := append(native, palette.PluginEntries(plugins, palette.Platform(a.env.GOOS))...)
	var out []palette.Entry
	for _, e := range all {
		if _, ok := keybind.SpecFor(e, a.env.GOOS); ok {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (a *app) keysList(ctx context.Context, args []string) error {
	fs := newFlagSet("keys ls")
	var all bool
	fs.BoolVar(&all, "all", false, "list every entry that can be bound, with or without a key")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usagef("%s", keysUsage)
	}
	entries, err := a.bindableEntries(ctx)
	if err != nil {
		return err
	}
	cfg, err := a.keyStore().Read()
	if err != nil {
		return err
	}
	shown := 0
	for _, e := range entries {
		b, _ := keybind.SpecFor(e, a.env.GOOS)
		key := cfg.KeyOf(b)
		if key == "" && !all {
			continue
		}
		if key == "" {
			key = "-"
		}
		fmt.Fprintf(a.env.Stdout, "%-22s %-34s %s (%s)\n", key, e.ID, e.Title, e.Source)
		shown++
	}
	if shown == 0 {
		fmt.Fprintln(a.env.Stdout, `no shortcuts on palette entries yet; "hum keys ls --all" lists what can be bound`)
	}
	return nil
}

// findEntry busca una entrada por su id (el que muestra `hum keys ls --all`).
func (a *app) findEntry(ctx context.Context, id string) (palette.Entry, error) {
	e, _, err := a.findEntryIn(ctx, id)
	return e, err
}

// findEntryIn es findEntry y además devuelve todas las entradas enlazables, para
// poder nombrar con su título lo que choca.
func (a *app) findEntryIn(ctx context.Context, id string) (palette.Entry, []palette.Entry, error) {
	entries, err := a.bindableEntries(ctx)
	if err != nil {
		return palette.Entry{}, nil, err
	}
	for _, e := range entries {
		if e.ID == id {
			return e, entries, nil
		}
	}
	var near []string
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.ID), strings.ToLower(id)) {
			near = append(near, e.ID)
		}
	}
	err = fmt.Errorf("%q is not an entry that can be bound to a key", id)
	if len(near) > 0 {
		return palette.Entry{}, nil, withHint(err, "did you mean: %s", strings.Join(near[:min(len(near), 5)], ", "))
	}
	return palette.Entry{}, nil, withHint(err, `"hum keys ls --all" lists them`)
}

func (a *app) keysSet(ctx context.Context, args []string) error {
	fs := newFlagSet("keys set")
	var force bool
	fs.BoolVar(&force, "force", false, "bind it even if it is one of herdr's default shortcuts")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 2 {
		return usagef("%s", keysUsage)
	}
	e, entries, err := a.findEntryIn(ctx, rest[0])
	if err != nil {
		return err
	}
	svc := a.keyService(func() []palette.Entry { return entries })
	check := svc.Check(e, rest[1])
	// Los mensajes de Check ya nombran la tecla de la que hablan.
	if check.Problem != "" {
		return withHint(errors.New(check.Problem), "nothing was changed")
	}
	if check.Warning != "" && !force {
		return withHint(errors.New(check.Warning), "use --force to bind it anyway")
	}
	res, err := svc.Save(ctx, e, check.Key)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.env.Stdout, "%s -> %s (%s)\n", res.Key, e.Title, e.ID)
	if res.Note != "" {
		fmt.Fprintln(a.env.Stderr, res.Note)
	}
	return nil
}

func (a *app) keysRemove(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return usagef("%s", keysUsage)
	}
	e, err := a.findEntry(ctx, args[0])
	if err != nil {
		return err
	}
	res, err := a.keyService(nil).Remove(ctx, e)
	if errors.Is(err, keybind.ErrNotBound) {
		return fmt.Errorf("%s has no shortcut in config.toml", e.Title)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(a.env.Stdout, "removed the shortcut of %s (%s)\n", e.Title, e.ID)
	if res.Note != "" {
		fmt.Fprintln(a.env.Stderr, res.Note)
	}
	return nil
}

// chords devuelve, por id de entrada, el atajo que el config.toml le da: las
// acciones de plugins (que se enlazan por su id) y los comandos de herdr de la
// paleta (que se enlazan como un comando de shell).
func (a *app) chords(native []palette.Entry) map[string]string {
	chords := palette.LoadChords(a.getenv)
	if chords == nil {
		chords = map[string]string{}
	}
	if cfg, err := a.keyStore().Read(); err == nil {
		for id, key := range keybind.Chords(native, cfg, a.env.GOOS) {
			chords[id] = key
		}
	}
	return chords
}

// HowItRuns cuenta qué pasará al pulsar el atajo de la entrada.
func (k *keyService) HowItRuns(e palette.Entry) string {
	_, how, ok := keybind.SpecHow(e, k.goos)
	if !ok {
		return ""
	}
	switch how {
	case keybind.HowPluginAction:
		return "herdr runs the plugin action"
	case keybind.HowDirect:
		return "herdr runs the command right away"
	default:
		return "opens the palette on this command, which asks you what it needs"
	}
}
