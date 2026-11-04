package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/machines"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

// clearScreen borra la pantalla y lleva el cursor al inicio (ANSI).
const clearScreen = "\x1b[2J\x1b[H"

func (a *app) machines(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return a.machineList(ctx, nil)
	}
	switch sub, rest := args[0], args[1:]; sub {
	case "ls", "list":
		return a.machineList(ctx, rest)
	case "enable", "disable", "toggle":
		return a.machineChange(ctx, sub, rest)
	case "menu":
		return a.machineMenu(ctx, rest)
	default:
		if strings.HasPrefix(sub, "-") {
			return a.machineList(ctx, args)
		}
		return usagef("unknown machines command %q", sub)
	}
}

func stateLabel(enabled bool) string {
	if enabled {
		return "ENABLED"
	}
	return "DISABLED"
}

func (a *app) machineList(ctx context.Context, args []string) error {
	var asJSON bool
	fs := newFlagSet("machines")
	fs.BoolVar(&asJSON, "json", false, "print JSON")
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usagef("usage: hum machines [--json]")
	}

	profiles, err := machines.Service{Runner: a.env.Runner}.List(ctx)
	if err != nil {
		return err
	}
	if asJSON {
		if profiles == nil {
			profiles = []herdr.Profile{}
		}
		return writeJSON(a.env.Stdout, profiles)
	}
	if len(profiles) == 0 {
		fmt.Fprintln(a.env.Stdout, "No saved machine profiles.")
		return nil
	}

	tw := tabwriter.NewWriter(a.env.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "STATE\tLABEL\tTARGET\tSESSION")
	for _, p := range profiles {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", stateLabel(p.Enabled), p.Label, p.Target, p.Session)
	}
	return tw.Flush()
}

func (a *app) machineChange(ctx context.Context, verb string, args []string) error {
	fs := newFlagSet("machines " + verb)
	rest, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usagef("usage: hum machines %s <name>", verb)
	}

	svc := machines.Service{Runner: a.env.Runner}
	profiles, err := svc.List(ctx)
	if err != nil {
		return err
	}
	selected, err := machines.Find(profiles, rest[0])
	if err != nil {
		return err
	}

	wanted := verb == "enable" || (verb == "toggle" && !selected.Enabled)
	res, err := svc.SetEnabled(ctx, selected, wanted)
	if err != nil {
		return err
	}

	switch {
	case !res.Changed:
		fmt.Fprintf(a.env.Stdout, "%s is already %s\n", res.Profile.Label, strings.ToLower(stateLabel(wanted)))
	case wanted:
		fmt.Fprintf(a.env.Stdout, "enabled %s\n", res.Profile.Label)
	default:
		fmt.Fprintf(a.env.Stdout, "disabled %s (remote sessions and processes keep running)\n", res.Profile.Label)
	}
	return nil
}

// machineMenu es el menú interactivo: un número alterna una máquina, R
// refresca y Q sale. Termina también si se acaba la entrada.
func (a *app) machineMenu(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return usagef("usage: hum machines menu")
	}

	var in io.Reader = a.env.Stdin
	if in == nil {
		in = strings.NewReader("")
	}
	reader := bufio.NewReader(in)
	svc := machines.Service{Runner: a.env.Runner}

	notice := ""
	for {
		profiles := a.drawMenu(ctx, svc, notice)
		notice = ""

		fmt.Fprint(a.env.Stdout, "Number: toggle | R: refresh | Q: quit > ")
		line, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(a.env.Stdout)
				return nil
			}
			return err
		}

		choice := strings.TrimSpace(line)
		switch {
		case strings.EqualFold(choice, "q"):
			return nil
		case strings.EqualFold(choice, "r"):
			continue
		}

		n, convErr := strconv.Atoi(choice)
		if convErr != nil || n < 1 || n > len(profiles) {
			notice = "Choose a number from the current list, R, or Q."
			continue
		}
		notice = a.toggleFromMenu(ctx, svc, profiles[n-1])
	}
}

// drawMenu pinta el encabezado y la lista y devuelve los perfiles mostrados,
// en el mismo orden que sus números.
func (a *app) drawMenu(ctx context.Context, svc machines.Service, notice string) []herdr.Profile {
	out := a.env.Stdout
	st, _ := a.newStyle("") // "" es auto y nunca falla
	fmt.Fprint(out, clearScreen)
	fmt.Fprintln(out, st.Bold("Herdr machines"))
	fmt.Fprintln(out, "Enabled = automatic SSH connection; not a live connection check.")
	fmt.Fprintln(out, "Disabling a profile leaves remote sessions and processes running.")
	fmt.Fprintln(out)

	profiles, err := svc.List(ctx)
	switch {
	case err != nil:
		fmt.Fprintf(out, "Cannot load profiles: %v\n", err)
		profiles = nil
	case len(profiles) == 0:
		fmt.Fprintln(out, "No saved machine profiles.")
	}
	var labelWidth, targetWidth int
	for _, p := range profiles {
		labelWidth = max(labelWidth, ui.VisibleLen(p.Label))
		targetWidth = max(targetWidth, ui.VisibleLen(p.Target))
	}
	for i, p := range profiles {
		role := ui.RoleDim
		if p.Enabled {
			role = ui.RoleIdle
		}
		fmt.Fprintf(out, "[%d] %s  %s  %s  (session: %s)\n", i+1,
			st.Paint(role, ui.PadRight(stateLabel(p.Enabled), len("DISABLED"))),
			ui.PadRight(p.Label, labelWidth), ui.PadRight(p.Target, targetWidth), p.Session)
	}

	fmt.Fprintln(out)
	if notice != "" {
		fmt.Fprintln(out, notice)
	}
	return profiles
}

// toggleFromMenu alterna el perfil que la persona eligió y devuelve el aviso
// para mostrar en el siguiente refresco. Un resultado incierto no se reintenta.
func (a *app) toggleFromMenu(ctx context.Context, svc machines.Service, selected herdr.Profile) string {
	wanted := !selected.Enabled
	verb := "enable"
	if !wanted {
		verb = "disable"
	}

	res, err := svc.SetEnabled(ctx, selected, wanted)
	switch {
	case err != nil:
		return fmt.Sprintf("Action not confirmed: %v. Review the refreshed state.", err)
	case !res.Changed:
		return fmt.Sprintf("%s was already %s.", res.Profile.Label, strings.ToLower(stateLabel(wanted)))
	default:
		return fmt.Sprintf("Requested %s: %s. Refreshed state is shown above.", verb, res.Profile.Label)
	}
}
