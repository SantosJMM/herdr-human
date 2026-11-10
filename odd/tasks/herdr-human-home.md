# herdr-human home: a TUI panel to act on everything from one popup

Objective: `hum home`, a popup opened from a keyboard shortcut that is the "home" of herdr human (agents, attention, machines, commands).
Branch: `feat/home` (from `main` at v0.1.3). Delivery strategy: ask-on-risk. Test runner: `scripts/check.ps1` (gofmt -s, goimports, vet, staticcheck, `go test -race`).
Status: PLAN, waiting for the user's approval. Nothing is implemented yet.

## Specs
- S1: "Lo quiero en un popup que pueda llamar desde un atajo de teclado" — `hum home` runs in a herdr popup (`placement = "popup"`), opened by the plugin action `home` so it can be bound with `hum keys set` / ctrl+k like any other action. No persistent tab or split.
- S2: "una TUI panel de herd para que podamos accionar todo desde ahí. Es decir, que sea como el home del herdr human." — one screen with tabs that reuse what exists, switched with `tab` / `shift+tab` / `1`-`4`: Agents (the `hum pick` selector and its action menu), Attention (who needs you), Machines (enable/disable), Commands (the palette). Nothing is reimplemented: the existing models are hosted.
- S3: Same behavior as the current popups: `--embedded` draws no second frame, the chosen action runs after the popup closes through the detached `hum palette run`, `Esc` closes, `HUM_TRACE` measures the first frame, and it opens fast (lazy load, no PowerShell).
- S4 (assumption, not asked): bare `hum` in a terminal keeps opening the `pick` selector as today; `hum home` is the new entry.
- S5 (assumption, not asked): the Agents tab is the one that opens first; Attention shows the same grouping as `hum attention`.
- S6: Existing commands, keys and tests of `pick`, `attention`, `machines menu` and `palette` keep their behavior.

## Tasks
- [x] T1 (S2, S6) Let a model be hosted: the `pick` model reports "done / chosen" without quitting the program itself, so a parent can embed it. Behavior unchanged; existing tests stay green. Route: inline. Risk: item 3 (shared code used by `pick`). Done: `Options.Hosted` + `LeaveMsg`, one exit point `Model.leave()` replacing the 4 `tea.Quit`. Evidence: RED (build failed on the missing `Hosted`/`LeaveMsg`), GREEN with the 4 new tests + the existing `pick` tests, mutation (`leave` ignoring `hosted`) fails 3 tests, `scripts\check.ps1` passes. The machines menu is NOT a Bubble Tea model (it is a line prompt in `internal/cli/machines.go`), so it needed no change here; see T5. The palette's 4 exits are left for T6.
- [x] T2 (S1, S3) `hum home` skeleton: tab bar, key routing, embedded frame, manifest action + pane `home`, help, completion, README. Route: inline. Risk: none. Done: `tui.HomeModel` + `Tab` interface (Title/Init/Update/View/Captures) + `RunHome` + `NewTextTab` (`internal/tui/home.go`); `hum home [--embedded | open]` (`internal/cli/home.go`); manifest action `home` ("Open home screen") and pane `home` (popup 90% x 80%, `--embedded`); help, completion (`home`, `--embedded`), README (marked as under construction). Keys: `tab`/`shift+tab`/`1`-`N` switch, `q`/`esc` close, `ctrl+c` always closes; a tab that `Captures()` receives every other key; non-key messages and the window size reach every tab. The four tabs are placeholders ("Not available yet.") until T3-T6. Evidence: 9 model tests + 4 CLI tests (one end-to-end with the real program) + the manifest tests extended (action opens a pane that exists, pane command, size, `--embedded`); mutation (dropping the `Captures` guard) fails 1 test; `scripts\check.ps1` passes; real herdr lists the action `home` and the pane `home`; `hum home` without a terminal prints the error and hint. Not verified: opening the popup in the real herdr (left to the user).
- T3 (S2) Agents tab hosting the `pick` model with its menu. Route: inline. Risk: none.
- T4 (S2, S5) Attention tab: a model over the existing attention grouping. Route: inline. Risk: none.
- T5 (S2) Machines tab: a NEW Bubble Tea model (the existing `hum machines menu` is a line prompt, not hostable) that lists saved machines and enables/disables them through the same `machines.Service`. Bigger than first planned. Route: inline. Risk: none.
- T6 (S2, S3) Commands tab hosting the palette; its result goes through the same detached run. Route: inline. Risk: none.
- T7 (S1, S3) Trace of the first frame, bind it to a key with `hum keys set` on a config copy, README, final check. Route: inline. Risk: none.

## Open questions for the user
- Q1 (S4): should bare `hum` in a terminal open the home instead of `pick`? Default: no.
- Q2: which tab is used most? Default: Agents first.

## Log
- L1 (2026-11-10): "Quiero que analices la posibilidad de tener una TUI panel de herd para que podamos accionar todo desde ahí. Es decir, que sea como el home del herdr human."
- L2 (2026-11-10): "Lo quiero en un popup que pueda llamar desde un atajo de teclado"
- L3 (2026-11-10): "Aprobado, empieza con la primera tarea"
- L4 (2026-11-10): T1 done. Finding while reading the code: the machines menu is a line-based prompt, so T5 becomes a new model instead of hosting an existing one.
- L5 (2026-11-10): "Continua con el T2". T2 done. Two of my own tests were wrong at first (each key sent twice; the `+` of "shift+tab" counted as a frame corner) and were fixed, not the code.
- Analysis: `herdr plugin pane open` accepts `--placement overlay|split|tab|zoomed`, so a persistent panel is possible, but the user chose the popup (L2), which removes the need for live refresh and keeps the detached-run pattern.
