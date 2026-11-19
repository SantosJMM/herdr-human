# herdr-human

[Español](README.es.md)

Human-friendly herdr commands: names instead of IDs, every machine in a single
view, and one verb per intent. The command is called `hum`.

Author: [SantosJMM](https://github.com/SantosJMM) · Go module
`github.com/SantosJMM/herdr-human`.

```text
$ hum ls
Local
 raw
 └─ ◆ wS:p4  Pane ID del workspace odoous            claude  working
odoous                                                          1.3s
 odoo-inc
 └─ ○ w1:p2  odoous-odoo-dev                           codex      idle

$ hum say odoous/odoo-dev "run the tests"
sent to odoous/w1:p2 (odoous-odoo-dev)
```

## Installation

### As a herdr plugin

```powershell
herdr plugin install SantosJMM/herdr-human
```

The install step (`scripts/install.sh` on Linux and macOS, `scripts/install.ps1`
on Windows) downloads the prebuilt binary from the GitHub release whose tag
matches the manifest's `version`, checks its SHA-256 against the release's
`SHA256SUMS`, and places it in `bin/hum`. If anything fails (no release, no
network, your platform has no binary, or the SHA-256 does not match) **and `go` is
in the `PATH`**, it builds from source. If there is no Go either, the install
ends with a message saying what is missing and how to continue. A downloaded
binary that fails the check is never installed.

Platforms with a binary: Linux, macOS and Windows, on `amd64` and `arm64`. The
`SHA256SUMS` ships in the same release as the binaries, so it protects against
corrupted downloads, not against someone who controls the release.

### Building by hand

You need nothing installed to *use* it: it is a single static executable. To
build it you need Go (`go.mod` asks for `go 1.27.3`; with `GOTOOLCHAIN=auto`,
the default, Go downloads that version by itself).

```powershell
go build -o bin\hum.exe .\cmd\hum                                # binary used by the plugin
go install .\cmd\hum                                             # puts it in %USERPROFILE%\go\bin
GOOS=linux GOARCH=amd64 go build -o bin/hum ./cmd/hum            # Linux
herdr plugin link C:\path\to\herdr-human     # register the plugin
```

herdr does **not** add the plugin's `bin` to the `PATH`: the popup works because
the manifest calls `$HERDR_PLUGIN_ROOT\bin\hum.exe`. To type `hum` in a terminal,
use `go install` (the `go\bin` directory is usually already in the `PATH`).

> The command is called `hum` and not `hh` because `C:\Windows\hh.exe` (the
> Windows HTML Help viewer) comes first in the `PATH` and would run instead.

### On WSL

Meant for a WSL that already has herdr but not Go: the Linux binary is built
from Windows and WSL uses **its own clone** of the repository (example with the
`Ubuntu` distro).

> **Windows and WSL must not share the `bin/` folder.** On Windows,
> `CreateProcessW` resolves `bin\hum` to the file named `hum` before `hum.exe`;
> if that is a Linux executable, every plugin action (F1 included) fails silently
> with error 193. Never copy a Linux `bin/hum` into the folder Windows has linked.

```powershell
# 1. Clone inside WSL (the clone lives on the Linux disk, which is faster)
wsl.exe -d Ubuntu -- bash -lc 'git clone https://github.com/SantosJMM/herdr-human ~/.local/share/herdr-human'

# 2. Build the Linux binary from Windows, OUTSIDE the folder Windows uses
$env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
go build -o $env:TEMP\hum-linux .\cmd\hum
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED

# 3. Install it in the WSL clone and in its PATH, and register the plugin
$src = (wsl.exe -d Ubuntu -- wslpath -a "$env:TEMP\hum-linux").Trim()
wsl.exe -d Ubuntu -- bash -lc "mkdir -p ~/.local/share/herdr-human/bin ~/.local/bin && install -m 755 '$src' ~/.local/share/herdr-human/bin/hum && install -m 755 '$src' ~/.local/bin/hum && herdr plugin link ~/.local/share/herdr-human"
```

- After updating the code: `git pull` in the WSL clone and repeat steps 2 and 3
  (the plugin link is done only once).
- If WSL does have Go, `git pull` and `go build -o bin/hum ./cmd/hum` inside the
  clone are enough.

The plugin adds these actions to the command palette (F1):

| Action | Id (Windows / Linux and macOS) | Opens |
|---|---|---|
| *Talk to agents (hum pick)* | `pick-windows` / `pick-unix` | the picker, in a 90 % × 80 % popup |
| *Who needs me? (hum attention)* | `attention-windows` / `attention-unix` | who needs you |
| *Show all agents (hum ls)* | `overview-windows` / `overview-unix` | the list of all agents |
| *Manage saved machines (hum machines menu)* | `machines-windows` / `machines-unix` | the machines menu |
| *Open command palette* | `palette` | the [command palette](#command-palette-hum-palette) |

They all start `bin/hum` directly, with no PowerShell or `sh` in between (which
on Windows cost about a second per launch). The `-windows` and `-unix` ids of
each action do the same thing; both are kept because the `config.toml` shortcuts
already use them. The views that only show text (*Show all agents*, *Who needs
me?*) wait for **any key** before closing, so you have time to read them.

You can bind a key to them in `config.toml`; the picker complements herdr's
*Goto* (`prefix+g`, which is left untouched) with actions and all machines:

```toml
[[keys.command]]
key = "prefix+shift+l"
type = "plugin_action"
command = "herdr-human.pick-windows"   # on Linux/macOS: herdr-human.pick-unix
description = "talk to agents (hum pick)"
```

The plugin uses the binary in `bin/`: after updating the code, rebuild it
(`go build -o bin\hum.exe .\cmd\hum`) or the popups will keep running the old
version. The `herdr-rename` plugin keeps working as a fallback; the picker
already renames (*Rename…* action).

## Commands

| Command | What it does |
|---|---|
| `hum ls [--json] [--verbose] [--local] [--machine NAME] [--server-timeout 3s] [--fresh] [--color auto\|always\|never] [--full-names]` | Lists agents: machine → workspace → agent, with each machine's status in its header |
| `hum id <who> [--json] [--cmd]` | Prints the pane id; `--cmd` gives the `herdr --machine … agent prompt <id>` prefix |
| `hum say <who> <message…> [--wait] [--timeout MS]` | Sends a message to an agent |
| `hum ask <who> <message…> [--timeout MS] [--lines N]` | Sends, waits for the agent to settle and prints only the reply on stdout (progress and origin on stderr) |
| `hum attention [--json] [--watch [--interval 5s] [--notify]]` | Who needs you: `blocked` and `done` first, then `working`; `idle` ones are only counted. `--watch` redraws; `--notify` alerts once per transition to `blocked`/`done` |
| `hum pick [--local] [--machine NAME] [--full-names]` | Full-screen interactive picker (needs a terminal); see [Picker](#picker-hum-pick) |
| `hum palette` | Command palette: fuzzy-search and run any herdr command or any plugin's action (needs a terminal; opens with a key); see [Command palette](#command-palette-hum-palette) |
| `hum keys ls [--all]` / `set <entry> <key> [--force]` / `rm <entry>` | Keyboard shortcuts for the palette entries in herdr's `config.toml`, with prior validation and a backup; see [Assigning or changing keyboard shortcuts](#assigning-or-changing-keyboard-shortcuts-ctrlk) |
| `hum version` (`-v`, `--version`) | Prints hum's version (the plugin manifest's), the commit if it was built inside the repository, and the platform: `hum 0.1.2 (commit abc1234, windows/amd64)` |
| `hum trace on\|off\|show\|clear` | Timing trace of the popups, to measure how long they take to draw the first frame; see [Command palette](#command-palette-hum-palette) |
| `hum new <kind> [folder] [--name N] [--machine M] [--no-focus] [--timeout MS]` | Opens a tab, starts an agent in it and names it; prints `machine/pane`. With no arguments, in a terminal it asks for kind, folder and name; see [Creating an agent](#creating-an-agent-hum-new) |
| `hum look <who> [--lines N]` | Reads an agent's recent output |
| `hum name <who> <name>` / `--clear` | Renames the agent **and** the pane at once |
| `hum worktree ls [<who>]` | Lists the Git worktrees (of the agent's workspace, local or remote; without `<who>`, of the current folder) |
| `hum worktree new <branch> [<who>] [--base REF] [--label TEXT] [--no-focus]` | Creates a worktree with a new branch. **hum never passes `--trust-repository`**: if herdr asks you to trust the repository, it tells you and you do it with herdr |
| `hum worktree rm <who> [--force] [--yes]` | **Deletes** the agent's worktree checkout. Asks (`[y/N]`); without a terminal it requires `--yes` |
| `hum alias set <name> <who>` / `rm <name>` / `ls` | Short names for agents: after `hum alias set docs odoous/odoo-dev`, `hum say docs "…"` works even if the agent's title changes. **The alias wins** over any other match, it is followed only once (an alias does not point to another alias) and it is not checked when saved: it is resolved when used. Aliases are stored in `aliases.json` inside `HERDR_PLUGIN_CONFIG_DIR` or, if that does not exist, in the user's config folder (`…\herdr-human\`). A corrupt file does not break normal resolution: `hum alias ls` tells you what is wrong and the file is not overwritten |
| `hum close --tab\|--workspace <who> [--yes]` | **Closes** the agent's tab or workspace and stops everything running inside it. In a terminal it asks (`[y/N]`; any other answer cancels); without a terminal it requires `--yes` |
| `hum rename --tab\|--workspace <who> <label…>` | Renames the agent's tab or workspace (the label may contain spaces; it cannot start with `-` or contain control characters) |
| `hum go <who>` | Focuses an agent |
| `hum wait <who> [--until STATE]… [--timeout MS] [--notify]` | Waits until it finishes (by default `idle`, `done` or `blocked`); `--notify` shows a herdr notification on this machine |
| `hum machines [--json]` | Lists the saved SSH machines with their status (ENABLED/DISABLED) |
| `hum machines enable\|disable\|toggle <name>` | Enables or disables a machine by label (or unique prefix) |
| `hum machines rename <name> <new label…>` | Changes a saved machine's label (the new one may contain spaces; if it starts with `-`, put it after `--`). Refuses if another machine already uses that label (case-insensitive), or if it is empty or contains control characters |
| `hum machines menu` | Interactive menu: number = toggle, `R` refreshes, `Q` quits |

### Saved machines

It replaces the old `herdr-machines.ps1` script. *Enabled* is configuration
state (automatic SSH connection), not a check that the machine is reachable, and
disabling a profile leaves its remote sessions and processes running. So as not
to step on another client, the change keeps the same guarantees as the original
script: it re-reads the profile by its immutable ID before mutating, refuses if
it disappeared or if its state changed since you saw it, does nothing if it is
already in the requested state, confirms the result by reading again and
**never retries** a mutation with an uncertain outcome.

`hum machines rename` keeps those same guarantees: it re-reads the profile by its
ID before renaming, refuses if it disappeared or if another client changed its
label since you saw it, does nothing if it already has that label, confirms by
reading again and does not retry. Renaming clears the inventory cache, because
`machine/` names are resolved by label. A rename does not affect remote
sessions. Implementation note: the real herdr requires
`machine rename <id> --label <label>` (the id first) even though clap's help
suggests another order; hum invokes it that way.

The plugin adds the *Manage saved machines (hum machines menu)* action to the
palette (F1); you can bind a key to it:

```toml
[[keys.command]]
key = "prefix+m"
type = "plugin_action"
command = "herdr-human.machines-windows"
description = "manage SSH machines"
```

### Look: colors, glyphs and width

`ls`, `attention` and `machines menu` share the same render kit (`internal/ui`):

- **Theme:** `[theme]` (and `[theme.custom]`) is read from herdr's `config.toml`
  (`%APPDATA%\herdr\config.toml` on Windows). There are built-in palettes for
  `catppuccin` and `catppuccin-latte`; they approximate herdr's colors (which it
  does not expose) and with `auto_switch` the `name` is used. Any other theme, or
  `terminal`, uses the terminal's ANSI colors. The status colors (`blocked` red,
  `working` yellow, `done` blue, `idle` green) were chosen by the author; they
  were not copied from herdr's sidebar.
- **No color:** `NO_COLOR`, a pipe, or `--color never`. `--color always` forces
  it. `--json` never carries decoration.
- **Glyphs:** `○` agent, `◆` focused pane, `├─ └─` tree; `HUM_ASCII=1` uses `o`,
  `*` and `|- \`-`.
- **Width:** in a terminal the columns are pushed to the right and names are cut
  with `…`; in a pipe nothing is cut. Codex-style titles
  (`Task | project | 5h 8% left`) show only the first segment; `--full-names`
  shows them whole.

### Picker (`hum pick`)

A frame with a title, search, a machine → workspace → agent tree, right-aligned
columns, a full-width selected row and a footer with breadcrumbs, path and keys,
in the style of herdr's Goto. Agents that need you (`blocked`, `done`) come
first within their workspace.

| Key | Action |
|---|---|
| `↑` `↓` `j` `k`, `PgUp` `PgDn`, `g` `G` | move the selection |
| `←` `→` | jump to the previous / next workspace |
| `/` | search by name, kind, workspace, machine, tab, path or pane (letters in order also work); matches are highlighted. Edit with `←` `→` `Home` `End` `Del` and `Backspace`; the Ask, Say and Rename lines are edited the same way |
| `b` `w` `i` `d` `a` | filter `blocked`, `working`, `idle`, `done` / all |
| `Enter` | open the agent's action menu (see below) |
| `Ctrl+A` `Ctrl+S` `Ctrl+L` `Ctrl+N` | direct shortcuts: ask, say, look, rename |
| `Esc` / `q` / `Ctrl+C` | quit (in the search, `Esc` clears it) |
| `r` | refresh the status of every machine (rows are kept; the header shows `refreshing`) |
| `Ctrl+U` | clear the search |
| mouse | click a row, wheel to scroll |

Each machine shows up when it responds (`⠋ connecting 3.1s`, then its time or
`✖ timeout`), without blocking the keyboard.

**Action menu** (`Enter` on an agent; each action has its letter):

| Action | Letter | What it does |
|---|---|---|
| Ask… | `a` | asks for the message on the bottom line (`message >`), waits and shows the reply inside the picker (`── reply ──`, with its origin: after your message or the last 40 lines; if the agent ends up `blocked` it says so). Any key returns to the list; `Esc` while waiting stops waiting and the agent keeps working |
| Say… | `s` | sends the message without waiting |
| Look | `l` | shows the terminal's recent output (scroll with `↑↓` `PgUp` `PgDn`) |
| Rename… | `n` | asks for the name (`name >`) and validates the format as you type; empty clears the name |
| Go to it | `g` | moves herdr's focus to the agent and closes the picker |
| Wait and notify | `w` | waits in the background and shows a herdr notification when it settles; you can keep browsing |
| Copy pane id | `c` | copies the pane id (`clip`, `pbcopy`, `wl-copy`, `xclip` or `xsel`) |

For local workspaces the git branch is shown on the right (read from
`.git/HEAD`, without running `git`; `r` reads it again). The result of each
action appears on the footer's second line. After Ask, Say, Rename or Wait, that
agent's machine is refreshed so its status is current; if the refresh fails you
are told and the rows are kept. Whatever an agent prints is shown without
control characters, so it cannot move the cursor or change the terminal. `hum`
with no arguments opens the picker when input and output are a terminal; with a
pipe it prints the help (exit code 0).

### Command palette (`hum palette`)

An F1-style palette: one key opens a popup with **the herdr actions that make
sense to launch from a palette** (workspaces, tabs, panes, worktrees, machines,
plugins…; see [Which commands are included](#which-commands-are-included-and-which-are-not))
and **every action of every installed plugin**, filtered with fuzzy matching as
you type. Pick one, press Enter and it runs. Commands that need a value (a name,
a folder, a target workspace) ask for it; **Esc** at a question cancels
everything. If something fails, the popup shows the reason instead of closing
silently.

To open it, bind this plugin's `palette` action to a key in herdr's
`config.toml` and reload the configuration:

```toml
[[keys.command]]
key = "f1"
type = "plugin_action"
command = "herdr-human.palette"
description = "command palette"
```

`type = "plugin_action"` matters: with a `shell` shortcut that opens the pane by
hand, the palette loses the context it was opened from.

| Key | Action |
|---|---|
| type | filters |
| `↑` `↓` / `ctrl+p` `ctrl+n` | moves the selection |
| `PgUp` / `PgDn`, `Home` / `End` | page or ends of the list |
| `Enter` | runs the selection |
| `ctrl+w` / `ctrl+u` | deletes the last word / the whole search |
| `Esc` / `ctrl+c` | closes without running anything |

Next to each entry you see its origin (the plugin, or `Herdr`) and the key bound
to it in your `config.toml`. What you used recently appears first.

While you type, the matching letters are highlighted in the title and in the
origin (they are exactly the ones the filter uses, so what is highlighted
explains why that row shows up). Since the search also looks at the description
and the id, a row may appear with no highlighted letters in the title. The help
line in the footer describes the selected entry; if it is a plugin action with no
description, it shows its id (`Plugin action plugin.action`), which is what you
bind to a key.

#### Assigning or changing keyboard shortcuts (`ctrl+k`)

With a row selected, **`ctrl+k`** opens that entry's shortcut screen: it shows
the one it has now and waits for you to **press the combination** you want. It is
written as in herdr's `config.toml` (`ctrl+alt+k`, `alt+left`, `f5`,
`alt+minus`…). Inside that screen:

| Key | What it does |
|---|---|
| the combination | is captured and validated instantly |
| `tab` | adds or removes herdr's prefix (`prefix+…`); useful for single keys, which without the prefix would swallow what you type |
| `ctrl+t` | type the combination by hand, for what your terminal cannot tell apart (`ctrl+h`, `prefix+enter`…) |
| `delete` | removes the current shortcut (asks for confirmation) |
| `enter` | saves |
| `esc` | closes the screen without changing anything (a second `esc` closes the palette) |

**Every entry can be bound**, in three ways depending on what it needs (the
shortcut screen tells you which one applies, on the *when pressed* line):

- **Any plugin's actions** (those of `hum`, `herdr-rename`, `reviewr`…) are
  saved as `type = "plugin_action"` with their qualified id: herdr invokes them.
- **herdr commands that ask nothing**, for example *Config: Reload* or
  *Tab: Close current*, are saved as `type = "shell"` with the system shell's
  syntax (`cmd.exe` on Windows, `sh` elsewhere) and the ids herdr gives to
  commands of that type (`HERDR_ACTIVE_TAB_ID`…): herdr runs them as is, with no
  interface, which is why they are instant.
- **Everything else** — the commands that ask you to **choose** (*Agent: Focus…*,
  *Workspace: Switch…*), to **type** (*Worktree: New…*), the next/previous ones
  and those of `hum` — cannot be run blindly from a key, because they need to ask
  you something. They are also saved as `type = "shell"`, but the command
  **opens the palette popup already inside that entry**:

  ```toml
  [[keys.command]]
  key = "ctrl+alt+f"
  type = "shell"
  command = "\"%HERDR_BIN_PATH%\" plugin pane open --plugin herdr-human --entrypoint palette --focus --env HUM_PALETTE_ENTRY=agent.focus"
  description = "Agent: Focus…"
  ```

  `hum palette` reads `HUM_PALETTE_ENTRY` and goes straight into that entry: you
  see the list of agents (or whatever question applies) without searching or
  picking the row, you choose and it runs as if you had done it from the list.
  **Esc** at the first question closes everything. If the entry is not available
  from where you pressed the key (for example, *Pane: Close current* with no
  source pane), the palette opens normally with a notice and runs nothing. Only
  the id of a catalog entry is valid; the shortcut passes the variable with
  `--env`, so do not put it in herdr's environment, or `F1` would always enter
  directly.

Every change to `config.toml` is made with a safety net:

1. It is validated **on a copy**, before touching your file, with
   `herdr config check` (compared with the result for the file as it was: problems
   it already had do not block an unrelated change). If herdr rejects it,
   nothing is written.
2. A backup is saved, `config.toml.bak-hum-YYYYMMDD`: the first change of each
   day; later ones the same day do not create more.
3. It is written in one go (temporary file and rename) and **only what is
   necessary is touched**: changing a block's key changes that line and keeps its
   comments and alignment; a new shortcut is a block at the end; removing it
   deletes its block. Line endings (`LF` or `CRLF`) are preserved.
4. herdr is asked to reload the configuration (`herdr server reload-config`).

**Conflicts.** A key already used by another `[[keys.command]]`, a `[keys]`
action or a `[keys.indexed]` modifier (`alt` with `1..9`) **is rejected**, naming
who uses it: herdr would disable one of the two. A **herdr default** shortcut
(`prefix+n`, `prefix+g`, `prefix+x`…) only **warns**, and you have to press
`enter` a second time: `herdr config check` does not detect it and it is not
known which of the two wins. The list of defaults is a copy of herdr v0.9.5's
documentation; if herdr changes it, the copy falls behind.

The same is available on the command line, in case the terminal does not deliver
a combination or to automate it:

```powershell
hum keys ls [--all]                 # the palette's shortcuts (--all: everything bindable, with its id)
hum keys set <entry> <key>          # assigns or changes; --force for a herdr default
hum keys rm <entry>                 # removes the shortcut
```

`<entry>` is the id shown by `hum keys ls --all` (`herdr-human.pick-windows`,
`tab.close`…); a partial id suggests the similar ones.

**Inside a herdr popup (`--embedded`).** A herdr popup already draws its own
frame with the pane's title; if the TUI draws another one on top, you get a box
inside a box. That is why `hum palette` and `hum pick` accept `--embedded`: they
draw no frame or title, just the line that separates the search and a one-column
margin, like herdr's native views (*Go to*). The picker keeps its scrollbar on
the right edge and the palette gains the rows the borders used to take. The
plugin manifest passes it in the palette and picker panes. Without the flag (in
any terminal) the usual own frame is drawn, with its title; there is no automatic
detection, so what you see depends on what you type.

**Why it opens fast.** The palette is meant to open instantly, not just to work:

- The action and the pane start `bin/hum` **directly**, with no PowerShell or
  `sh` in between (on Windows, `powershell.exe` takes about a second to start and
  the original version launched it twice). Measured on this machine with the
  system's process tooling, from the moment the action fires until the palette
  process exists: ~70–95 ms with `hum`, against ~2.3–2.4 s with the original Node
  palette.
- **Lazy loading.** The first frame is drawn with the commands that ship inside
  the binary and the plugin actions from last time (a cache in the plugin's state
  folder). The current actions are requested from herdr in the background — a
  single call, `plugin list --json`, instead of two — and are merged in when they
  arrive; meanwhile the header says `loading plugins…` and, if herdr does not
  respond, `plugins unavailable` without losing what was already drawn.
- If you have not moved the selection, it follows the best result when the new
  actions arrive; if you already navigated, it stays where it was.
- Workspaces, tabs and agents are requested from herdr **only when a command
  needs them** (for example, "Workspace: Next" or a picker), not on open.
- The source workspace, tab and pane are read from the
  `HERDR_PLUGIN_CONTEXT_JSON` variable that herdr already hands to the popup:
  there is no need to ask `pane current` or to pass variables between the action
  and the pane.

**Measuring the opening (`hum trace`).** To check it with numbers instead of by
eye:

```powershell
hum trace on        # turns the trace on (HUM_TRACE=1 in the terminal also works)
# ... open the palette with its key a few times ...
hum trace show      # last lines of the log
hum trace off       # turns it off (the log is kept); "hum trace clear" deletes it
```

`hum trace on` creates a marker file in hum's cache folder
(`%LOCALAPPDATA%\herdr-human` on Windows) instead of depending on an environment
variable, because popups inherit the environment of herdr's **server**, not that
of your terminal. Each line carries the time, the process and the milliseconds
since Windows created the process (not since `main`, so it includes loading the
`.exe`):

```text
12:25:12.036Z pid=54840 palette open +24.2ms open: asking herdr to open the pane
12:25:12.075Z pid=54840 palette open +63.2ms open: herdr answered
12:25:12.183Z pid=58372 palette +115.3ms environment ready
12:25:12.185Z pid=58372 palette +117.1ms tui: first view rendered (the terminal gets it within one frame)
12:25:12.202Z pid=58372 palette +134.3ms palette: first frame written to the terminal (190.3ms since the trigger)
12:25:12.212Z pid=58372 palette +144.6ms palette: herdr answered with the plugin list
```

"First frame" is when the first frame with the title is written to the terminal
(the output is wrapped while keeping the file, so Bubble Tea still knows the
window size). "Since the trigger" counts from when the action's process started;
`hum` passes it to the popup with `--env HUM_TRACE_T0`, only if the trace is on.
On this machine (Windows, 5 openings): ~175–231 ms (median ~190 ms) from the
trigger to the first frame. Of that, what `hum` does from `main` until writing it
is ~19 ms; ~100 ms is the pane's startup in herdr (a process in a normal console
reaches `main` in ~11 ms, the popup's in ~106–147 ms), which `hum` does not
control. With the trace off it writes nothing.

What you pick is not run inside the popup: while it is open, herdr rejects
(`ui_busy`) anything that opens a pane or popup. That is why `hum palette`
launches a separate process (`hum palette run`) that waits for the popup to close
— retrying while herdr is still busy — and then runs the command. If it fails, it
says so with a herdr notification.

The native commands are in `internal/palette/commands.json` (46 entries: 42 from
herdr, which use 25 distinct commands, and 4 from `hum`) and ship inside the
binary. Shortcuts are read from `HERDR_CONFIG_PATH` or, if it is not set, from
herdr's `config.toml` in the system's config folder (`%APPDATA%\herdr` on
Windows). Recents and the cache live in `HERDR_PLUGIN_STATE_DIR`.

#### Which commands are included (and which are not)

**Plugin actions** are read from herdr on every open, so they are complete:
install a plugin and it shows up. **Native herdr commands**, on the other hand,
are a hand-written list, because herdr does not offer a command list meant for
this. So that none slips through without anyone noticing:

- Every herdr command must be in `commands.json` **or** in
  `internal/palette/excluded.go` with the reason for leaving it out.
- The test `TestCatalogAndExclusionsAccountForEveryInstalledHerdrCommand` walks
  the installed `herdr`'s help and fails if a command is in neither list (or an
  entry points to a command herdr no longer has). With herdr 0.9.4-preview that
  is 95 commands: 25 in the palette and 70 excluded. If herdr is not installed
  (for example, in CI) the test is skipped.
- It walks them from the nouns (`workspace`, `pane`, `plugin`…); the loose
  top-level commands (`status`, `update`, `completion`) are not checked.

They are excluded on purpose: what only prints information (`list`, `get`,
`read`; the palette runs the command separately and nobody would see the output),
what agent integrations use, what is destructive (`worktree remove`,
`machine remove`, `session delete`, `server stop`…, best done from a terminal
where you can confirm), what needs that terminal, what takes free-form
arguments, what installs third-party code, and what `hum` already solves with its
own flow (the `hum` commands below take care of `agent prompt`, `agent rename`
and `agent start`).

#### `hum` commands in the palette

Five entries with origin `hum` (in `commands.json` they carry `"runner": "hum"`):

| Entry | Runs | Asks |
|---|---|---|
| *Agent: New…* | `hum new <kind> <folder> [--name N]` | agent kind (the `hum new` list), folder (starts from the source pane's) and optional name |
| *Agent: Say…* | `hum say <agent> -- <message>` | agent and message |
| *Agent: Rename…* | `hum name <agent> <name>` | agent and name |
| *Agent: Notify me when it settles…* | `hum wait <agent> --notify` | agent; notifies with a herdr notification when it finishes or needs you |
| *Machine: Rename…* | `hum machines rename <machine> -- <label>` | saved machine and new label; refuses if another machine already uses that label |

They run in a separate process (`hum palette run --hum …`) with the same `ui_busy`
retry as the herdr ones. If they fail, they notify; when `hum wait` ends blocked
or by timeout it is not notified twice, because it already reports that itself.
*Agent: New…* leaves the name empty to use the kind: an empty optional value adds
neither `--name` nor the value. A test checks that every command and every option
the catalog uses exists in `hum`'s CLI, so renaming one breaks the test and not
the palette.

To add a command: an entry in `commands.json` (argument sources `literal`,
`context`, `input` and `select`; in an `input`, `flag` makes it optional;
selectors `workspaces`, `tabs`, `agents`, `machines`, `plugins` and
`agent_kinds`; `"runner": "hum"` if it is a `hum` command) and remove it from
`excluded.go` if it was there.

### Creating an agent (`hum new`)

```text
$ hum new claude C:\src\dev\raw --name docs
creating claude...
✔ tab created (wS:tA)
✔ claude is ready
✔ named "docs"
local/wS:pA
```

It is the `tab create` + `agent start` + `pane rename` sequence in a single
command. Progress goes to stderr and only `machine/pane` to stdout, so it can be
captured (`$ref = hum new codex`).

- **Kind:** one of the kinds herdr knows how to start (`claude`, `codex`,
  `gemini`, `pi`…; the full list is in `herdr agent start --help`). It is checked
  before creating anything, so a typo does not leave an empty tab behind.
- **Folder:** the current one by default; `~` is your home folder and a relative
  path hangs from the current one. On a remote machine it is sent as is, and if
  omitted herdr decides (this machine's folder does not exist there).
- **Machine:** `hum new odoous/claude …` or `hum new claude … --machine odoous`
  (equivalent); `local` is this machine.
- **Name:** `--name` (same rules as `hum name`); without it the agent is named
  after its kind and the pane is not renamed.
- `--no-focus` does not move the focus to the new tab; `--timeout MS` changes how
  long to wait for the agent to be ready (30 s by default).
- **If a step fails** it says which one and what was left created, with a hint to
  clean it up (`herdr tab close …` or `hum name …`); nothing is silently undone.

### Autocompletion (`hum completion`)

`hum completion bash|zsh|powershell` prints the shell script. It completes
subcommands, options (`--fresh`, `--color`…), their values
(`--color auto|always|never`, `--until idle|working|…`, `--machine` with your
machines), the kinds for `hum new`, the `hum machines` subcommands and, wherever
a `<who>` goes, agents.

| Shell | Installation |
|---|---|
| PowerShell | `hum completion powershell \| Out-String \| Invoke-Expression` (add it to `$PROFILE`) |
| bash | `source <(hum completion bash)` (add it to `~/.bashrc`) |
| zsh | `hum completion zsh > "${fpath[1]}/_hum"` and open a new shell, or `source <(hum completion zsh)` with `compinit` already loaded |

- **It never waits for the network.** Local agents are queried live
  (milliseconds) and remote ones come from whatever the [cache](#performance)
  last saved, even if it has expired (up to an hour). The cache is filled by
  `ls`, `attention`, `pick` and when resolving names, so it is enough to have
  used them recently.
- **Which names it suggests:** `machine/pane` (`odoous/w1:p2`) and the names
  `hum` understands (`docs-agent`, `odoous/docs-agent`). Titles with spaces
  (`Configura Odoo 18 | …`) are not suggested; give it a name with `hum name`
  and it will show up.
- Descriptions (kind, status and title) in zsh and PowerShell. After
  `hum new <kind>`, the folder is completed by the shell.
- Everything that comes from an agent or from your configuration is stripped of
  control characters before reaching the command line.
- The scripts call `hum __complete --shell S --line TEXT` (an internal command
  that never fails or writes to stderr); it is useful to debug what it suggests.

### Asking and waiting

`hum ask` uses `agent prompt --wait` and then reads the terminal. The reply is a
**heuristic**: whatever appears after the sent message (the start of the message
is searched for on screen) and, if it is not found or there is nothing after it,
the last `--lines` lines (40); stderr says which of the two was used. If the
agent ends up `blocked` no reply is printed: it tells you what it is waiting for
(last line) and exits with `3`; `Ctrl+C` stops waiting and the agent keeps
working. `hum wait` is the same without sending anything.

Options can go in any position. If the `say` message contains words that start
with `-`, put them after `--`.

### How the agent is chosen (`<who>`)

They are tried in this order and the first level with matches wins, ignoring
case:

1. equal to the pane id (`w1:p2`), to the name or to the agent kind (`codex`);
2. the name starts with what was typed;
3. the name contains what was typed;
4. the letters appear in order within the name (`cnf` → `Configura…`).

If a level has several matches, `hum` fails and lists them as `machine/pane` so
you can choose. The `machine/` (or `local/`) prefix restricts the search to one
server and also avoids querying the other machines.

`--machine NAME` does the same with another syntax: `hum ask odoous/codex "…"`
is equivalent to `hum ask codex "…" --machine odoous`. `--machine local` names
the local server and can be repeated to search only among several machines. With
either form the machines not mentioned are not queried, so an enabled but down
machine (for example *WSL Ubuntu*) produces no warnings. (In `ls`, `--machine`
still always includes the local server.)

## Errors with hints

An error says what happened and, when the cause is known, a `hint:` line with
what to do: ambiguous query (`use "machine/pane", e.g. …`), nonexistent agent
(`run "hum ls"`), unreachable machine that might hold the agent, `blocked` agent
(`hum go …`), closed agent and invalid name. In an interactive terminal (input
**and** output), an ambiguous query asks `Which one? [1-N, Enter to cancel]`
instead of failing. Enter, `q` or `Ctrl+C` cancel instantly (`cancelled`, exit
code 1); an answer that is not a number from the list keeps the original error
with its hint. A failure with no explanation does not invent hints.

## Exit codes

`0` success · `1` error (not found, ambiguous, herdr failed) · `2` incorrect
usage · `3` the agent ended up `blocked` waiting for a person (`wait`; with
`--until blocked` that state is a success) · `4` the `--timeout` expired (`wait`).

## Design

```text
cmd/hum                entry point (signals, os.Exit)
internal/cli           subcommands, flags and output format
internal/resolve       a person's query → one agent
internal/inventory     local + machines in parallel, with timeout and isolated failure
internal/actions       say, look, name, focus on top of herdr commands
internal/herdr         Runner over the executable, over JSON and API errors
internal/herdrtest     fake Runner for tests
internal/gitinfo       a folder's git branch, read from .git/HEAD
internal/complete      autocompletion (bash, zsh, PowerShell) without network
```

A machine that does not respond is reported on `stderr` and does not prevent
seeing the others; only a failure of the local server is fatal. Each query has a
limit of 8 s (`--server-timeout 3s` changes it).

### Performance

Each remote herdr call costs ~1.3 s because of SSH, so `hum` makes **one** per
machine (`herdr api snapshot`, which brings agents and workspaces together) and
all machines in parallel; the total time is that of the slowest. Measured with
`odoous`: 2.75 s with two consecutive calls → 1.4 s with the snapshot. If
`hum ls` is slow, `hum ls --verbose` prints on `stderr` how long each server took
(`timings: local 21ms, odoous 1.3s, …`) to see which one is slowing it down; with
`--machine`/`--local` or `--server-timeout` you can skip or cut the slow one. The
two lists are used only if a server answers that it does not support the
snapshot.

**Progressive `ls`.** In a terminal, `hum ls` draws the tree as soon as a moment
passes (150 ms) and keeps **redrawing it in place**: local and cached data show
up right away, and each machine that has not responded yet shows
`⠋ connecting 2.1s` until its time or its `✖ timeout` arrives. The final result
is the same tree as always. If everything responds within that margin it is
printed once, without flicker. It is not redrawn (and printed once at the end) if
the tree does not fit the terminal's height, if the height is unknown, with a
pipe or with `--json`.

**Cache.** Each **remote** machine's response is kept for 10 s in the user's
cache folder (`%LocalAppData%\herdr-human\inventory.json` on Windows;
`~/.cache/herdr-human/` on Linux), with permissions for you only. That way,
repeating `hum ls` or resolving a name (`hum say`, `ask`, `look`, `id`, `name`,
`go`, `wait`) right afterwards does not pay the SSH wait again.

- The local server is **never** cached: it responds in milliseconds and its
  states change all the time. `hum attention` and `hum pick` do not read the
  cache either (always live), but what they query **renews** it so `ls` and
  autocompletion have recent data.
- A machine that fails or times out is not saved, so it is retried.
- `ls` marks what comes from the cache in the machine's header (`cached 3s`);
  `--json` does not change.
- `--fresh` (in `ls` and in the commands that resolve a name) ignores what is
  stored and renews it.
- If a name does not appear and part of the data came from the cache, `hum`
  queries live before declaring it nonexistent (it may have been born a few
  seconds ago).
- `hum new` and `hum name` clear the cache, so as not to serve data from before
  the change.
- The per-machine deadline (`--server-timeout`, 8 s by default) is respected even
  if `herdr` leaves a hung `ssh` process: it is cut at most 1 s later.

## Development

```powershell
pwsh -NoProfile -File scripts\check.ps1
```

It runs, in this order, `gofmt -s`, `goimports`, `go vet`, `staticcheck` and
`go test -race`, and fails on the first problem. It must pass before every
commit. `goimports` and `staticcheck` are optional: if they are missing (or if
`staticcheck` is too old for the Go toolchain) you are warned and it continues,
but the warning does not mean the code passed that linter.

Tests use a fake `Runner`; they do not run herdr. All code and CLI messages are
in English; the documentation exists in English (`README.md`, the source of
truth) and Spanish (`README.es.md`, a translation kept in sync).

### Smoke test against a real herdr

```powershell
go build -o bin\hum.exe .\cmd\hum
pwsh -NoProfile -File scripts\smoke.ps1                     # this machine only
pwsh -NoProfile -File scripts\smoke.ps1 -Machine odoous     # and a remote machine
```

It is **read-only**: it runs `help`, `ls`, `attention`, `machines`, `id`, `look`,
`completion`, `__complete` and `alias ls` (the latter with an empty temporary
folder); it never sends, creates, renames, closes or deletes. A test
(`cmd\hum\smoke_test.go`) fails if the script runs any other command. Each step
ends in `PASS`, `FAIL` or `SKIP`; a `SKIP` (for example, a machine that does not
respond) means "not checked" and does not count as passing. It exits with 1 if
any step fails. It does not print titles, paths or agent names. It needs herdr
running, which is why `check.ps1` does not run it. Like any `ls`, it may update
hum's inventory cache.

## Acknowledgements

The command palette (`hum palette`) originates from the
**[herdr-command-palette](https://github.com/barnuri/herdr-command-palette)**
plugin by [barnuri](https://github.com/barnuri), MIT licensed. Thanks for the
idea, for the herdr command catalog (`commands.json`) and for the interaction
design — the fuzzy search, the recents, the shortcuts shown next to each action
and the split between the action that opens the popup and the process that runs
the chosen command — which have been ported to Go here.

If you only want the palette, without the rest of `hum`, the original plugin
remains a great option: `herdr plugin install barnuri/herdr-command-palette`. The
full license notice is in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Known limitations (v0)

- The picker (`hum pick`) is a first version: the plugin's popup still shows only
  `hum ls`, and the git branch only appears in local-server workspaces (on a
  remote machine it would cost another SSH call).
- The `say` text must go immediately after the pane id: herdr does not support
  `--`, and `hum` already accounts for it.
- `--machine` limits which saved machines are queried; the local server is
  always included.
