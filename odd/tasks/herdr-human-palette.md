# herdr-human-palette
Objetivo: traer a `hum` la paleta de comandos del plugin `herdr-command-palette` (barnuri) y hacer que abra mucho más rápido que el original. Rama: feat/palette. Entrega: commits en la rama; push, PR y merge son decisión del usuario. Runner de tests: `go test ./...` y `scripts\check.ps1` antes de cada commit.

## Specs
- S1. "primero pasa la instalcion de herdr-human a local" (el plugin deja de estar instalado desde GitHub y queda enlazado a una copia local).
- S2. "luego has una rama nueva para que acojamos las funcionalidad del siguiente plugin `C:\Users\vysist-smarte\src\dev\herdr-command-palette`".
- S3. "tambien vamos a agregar al readme una sona de agregadecimiento y referencia al repo original del plugin" (repo original: https://github.com/barnuri/herdr-command-palette, licencia MIT, copyright 2026 barnuri).
- S4. "no solo traeremos las funciones, quiero que apliques mejoras; puntualmente cuando ejecuto la accion por el atajo este tarda en abrir, eso quiero mejorar y si es necesario agregar lazy load."
- S5. Heredadas: código en inglés, documentación y comentarios en español; `scripts\check.ps1` en verde antes de cada commit; push/PR/merge y cambios en el `config.toml` del usuario solo con su orden.
- S6. (decisión mía, para confirmar) La paleta se porta a Go como `hum palette`: así no exige Node ni PowerShell y reutiliza el binario ya instalado.

## Tasks
- [x] T1 (S1) Instalación local: `herdr plugin uninstall herdr-human` + `herdr plugin link` sobre `herdr-human-clean` (con `bin\hum.exe` compilado). | inline | HECHO: `plugin list` muestra `local:...herdr-human-clean`, 8 acciones registradas. | commit: n/a (entorno)
- [x] T2 (S2) Rama `feat/palette` creada desde `main` del clon limpio. | inline | commit: n/a
- [ ] T3 (S4) Medición de la línea base y diagnóstico de la lentitud. | inline | VER Log
- [ ] T4 (S2,S4,S6) `internal/palette`: catálogo embebido (`commands.json`), entradas de plugins, difuso, recientes, atajos del `config.toml`, contexto de origen, resolución de argumentos, selectores. | inline | commit:
- [ ] T5 (S2,S4) Modelo de la TUI de la paleta con carga perezosa (pinta al instante con lo embebido y la caché; las acciones de plugins llegan después). | inline | commit:
- [ ] T6 (S2,S4) Comandos `hum palette`, `hum palette open` y `hum palette run`; manifest con comandos directos (sin PowerShell); completado; prueba del manifest. | inline | commit:
- [ ] T7 (S3) README: sección de agradecimiento y referencia al repo original; aviso de licencia MIT (`THIRD_PARTY_NOTICES.md`). | inline | commit:
- [ ] T8 (S4) Verificación en vivo y medición posterior; comparación con la línea base. | inline | commit:

## Log
- L1 (2026-11-05): "Ok, ahora te tengo una nueva tarea, primero pasa la instalcion de herdr-human a local, luego has una rama nueva para que acojamos las funcionalidad del siguiente plugin `C:\Users\vysist-smarte\src\dev\herdr-command-palette`; tambien vamos a agregar al readme una sona de agregadecimiento y referencia al repo original del plugin.\n\nAhora bien, no solo traeremos las funciones, quiero que apliques mejoras; puntualmente cuando ejecuto la accion por el atajo este tarda en abrir, eso quiero mejorar y si es necesario agregar lazy load."
- Ruta: todo inline (sin razón de writer paralelo). Espejo Engram: PENDIENTE (herramientas mem_* no disponibles).
- Fuente leída: el árbol de trabajo de `herdr-command-palette` (tiene cambios sin confirmar para Windows: `run-node.ps1`, `palette-windows`). Solo lectura, no se modifica.
- Diagnóstico (2026-11-05, medido en esta máquina): `powershell.exe -NoProfile -Command exit` tarda ~0,9–1,1 s; `node -e 0` ~0,14 s; cada llamada a `herdr` ~0,02 s (4 seguidas = 83 ms). La cadena actual en Windows lanza `powershell.exe` DOS veces (la acción `open-windows` y el pane `palette-windows`) antes de dibujar: ~2 s perdidos solo en arrancar PowerShell. Después carga 4 listados de herdr en serie antes del primer pintado.
- Palanca: `reviewr` declara `command = ["bin/herdr-reviewr", ...]` y en disco es `herdr-reviewr.exe`, o sea herdr resuelve el `.exe` sin PowerShell. Plan: acción y pane como comando directo a `bin/hum`, y pintado inmediato con carga perezosa.
- Sonda desechable (plugin `zz.probe` en el scratchpad, ya desenlazado): (1) herdr ejecuta `command = ["bin/probe", ...]` como `.exe` directo, tanto en acciones como en panes, con cwd = raíz del plugin; (2) la acción recibe `HERDR_PANE_ID`, `HERDR_TAB_ID`, `HERDR_WORKSPACE_ID` y `HERDR_PLUGIN_CONTEXT_JSON`; (3) el pane NO recibe `HERDR_PANE_ID`, pero SÍ `HERDR_PLUGIN_CONTEXT_JSON`, con `workspace_id`, `tab_id`, `focused_pane_id` y `focused_pane_cwd` del pane de origen. Consecuencia: ya no hace falta `herdr pane current` ni pasar `--env`; el pane lee su origen del JSON.
