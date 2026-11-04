# herdr-human
Objetivo: plugin de herdr con CLI `hum` que humaniza los comandos del CLI de herdr. Rama: feat/v0. Entrega: single-pr. Runner de tests: `go test ./...`.

## Specs
- S1. "hacer de los comandos cli de herdr mas humanos con una interfaz amigable" (plugin llamado "herdr-human").
- S2. "todo lo que es codigo lo prefiero en ingles, solo documentacion y comentario puede ir en español".
- S3. Lenguaje que "no requiere instalacion de algo previo" -> Go, binario estatico unico; `go.mod` con `go 1.27.3` (GOTOOLCHAIN=auto).
- S4. Ruta: `C:\ruta\a\herdr-human`, repo git propio.
- S5. CLI `hum`; modelo unico Target {machine, workspace, tab, pane, agent, name}; nombres antes que IDs; maquinas transparentes (local + habilitadas, en paralelo, fallo aislado por maquina, timeout).
- S6. Vocabulario v0: `hum ls`, `hum id`, `hum say`, `hum look`, `hum name`, `hum go`; todo con `--json`.
- S7. Resolutor acepta nombre, `maquina/nombre`, coincidencia difusa o ID crudo; ambiguo -> falla listando candidatos.
- S9. "necesito que pasemos esa funcionalidad al herdr human" (menu de maquinas prefix+m que habilita/deshabilita perfiles; script: Documents\Codex\2026-10-27\he\outputs\herdr-machines.ps1).
- S8. "procura actualizar tu memoria con lo relacionado al desarrollo de plugin de herdr y sobre todo buenas practicas en go".

## Tasks
- [x] T1 (S3,S4) scaffold repo, go.mod, layout | inline | commit: 6620407
- [x] T2 (S5) internal/herdr: Runner sobre el binario herdr | inline | commit: 3b581c6
- [x] T3 (S5) internal/inventory: Target, carga paralela local+maquinas | inline | commit: 3b581c6
- [x] T4 (S7) internal/resolve | inline | commit: 8d34a98
- [x] T5 (S6) internal/actions + internal/cli + cmd/hum | inline | commit: see git log (feat: add hum CLI)
- [x] T6 (S1) herdr-plugin.toml + README (es) | inline | commit: see git log (feat: add hum CLI)
- [x] T7 (S8) memoria: plugins de herdr + buenas practicas Go | inline | commit: n/a (memoria en ~/.claude/projects/…/memory)

### Fase 2: gestion de maquinas (trasladar herdr-machines.ps1)
- [x] T8 (S9) herdr.Profile/ListProfiles/SetEnabled + paquete machines (Find, SetEnabled seguro) | inline
- [x] T9 (S9) hum machines ls|enable|disable|toggle|menu + Env.Stdin | inline
- [x] T10 (S9) manifest: accion/pane machines-*; README | inline
- [x] T11 (S9) atajo prefix+m rebindeado al plugin (config.toml, backup .bak-machines-20261103); script antiguo NO borrado | inline

## Log
- L1 (2026-11-03): "Me gusta, procede, toma en cuenta que todo lo que es codigo lo prefiero en ingles, solo documentacion y comentario puede ir en español, procura actualizar tu memoria con lo relacionado al desarrollo de plugin de herdr y sobre todo buenas practicas en go"
- Ruta: todo inline (sin razon de writer paralelo). Espejo Engram: PENDIENTE (herramientas mem_* no disponibles en esta sesion).
- Pruebas contra maquinas reales: solo `odoous` (autorizada); remoto2 no se consulta en pruebas.
- Hallazgo (2026-11-03): herdr 0.9.4 no admite `--` en `agent prompt`; toma como texto el argumento tras el pane id aunque empiece por `-`. Se corrigio actions.Say (texto primero, opciones despues) y se verifico contra herdr real con un target inexistente.
- Hallazgo: los titulos de agentes cambian con cada tarea; `hum` resuelve por nombre visible, asi que conviene `hum name` para nombres estables.
- Verificado en vivo (solo lectura): ls --local, ls/id/look en odoous. NO se envio ningun `say` ni `name` real. Plugin no enlazado (`herdr plugin link` pendiente de confirmacion del usuario).
- Espejo Engram: sigue PENDIENTE (herramientas mem_* no disponibles).
- L2 (2026-11-03): "necesito que pasemos esa funcionalidad al herdr human" (el menu para activar o desactivar maquinas, atajo prefix+m).
- Verificado en vivo (2026-11-03, con OK del usuario): enable/disable real de "WSL Ubuntu" (loopback) confirmado y restaurado; prefix+m rebindeado y accion invocada con exit_code 0.
