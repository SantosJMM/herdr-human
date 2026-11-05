# herdr-human

Comandos de herdr pensados para personas: nombres en vez de IDs, todas las
máquinas en una sola vista y un verbo por intención. El comando se llama `hum`.

Autor: [SantosJMM](https://github.com/SantosJMM) · módulo Go
`github.com/SantosJMM/herdr-human`.

```text
$ hum ls
Local
 raw
 └─ ◆ wS:p4  Pane ID del workspace odoous            claude  working
odoous                                                          1.3s
 odoo-inc
 └─ ○ w1:p2  odoous-odoo-dev                           codex      idle

$ hum say odoous/odoo-dev "corre los tests"
sent to odoous/w1:p2 (odoous-odoo-dev)
```

## Instalación

### Como plugin de herdr

```powershell
herdr plugin install SantosJMM/herdr-human
```

El paso de instalación (`scripts/install.sh` en Linux y macOS,
`scripts/install.ps1` en Windows) descarga el binario ya compilado del release de
GitHub cuya etiqueta coincide con la `version` del manifest, comprueba su
SHA-256 contra el `SHA256SUMS` del mismo release y lo deja en `bin/hum`. Si algo
falla (no hay release, no hay red, tu plataforma no tiene binario o el SHA-256 no
coincide) **y hay `go` en el `PATH`**, lo compila desde el código. Si tampoco hay
Go, la instalación termina con un mensaje que dice qué falta y cómo seguir. Un
binario descargado que no pasa la comprobación nunca se instala.

Plataformas con binario: Linux, macOS y Windows, en `amd64` y `arm64`. El
`SHA256SUMS` viaja en el mismo release que los binarios, así que protege contra
descargas corruptas, no contra alguien que controle el release.

### Compilar a mano

No necesita nada instalado para *usarlo*: es un único ejecutable estático.
Para compilarlo hace falta Go (el `go.mod` pide `go 1.27.3`; con
`GOTOOLCHAIN=auto`, el valor por defecto, Go descarga esa versión solo).

```powershell
go build -o bin\hum.exe .\cmd\hum                                # binario que usa el plugin
go install .\cmd\hum                                             # lo deja en %USERPROFILE%\go\bin
GOOS=linux GOARCH=amd64 go build -o bin/hum ./cmd/hum            # Linux
herdr plugin link C:\ruta\a\herdr-human     # registrar el plugin
```

herdr **no** añade el `bin` del plugin al `PATH`: el popup funciona porque el
manifest llama a `$HERDR_PLUGIN_ROOT\bin\hum.exe`. Para escribir `hum` en una
terminal usa `go install` (el directorio `go\bin` suele estar ya en el `PATH`).

> El comando se llama `hum` y no `hh` porque `C:\Windows\hh.exe` (el visor de
> ayuda HTML de Windows) aparece antes en el `PATH` y se ejecutaría en su lugar.

El plugin añade estas acciones a la paleta de comandos (F1):

| Acción | Id (Windows / Linux y macOS) | Abre |
|---|---|---|
| *Talk to agents (hum pick)* | `pick-windows` / `pick-unix` | el selector, en un popup del 90 % × 80 % |
| *Who needs me? (hum attention)* | `attention-windows` / `attention-unix` | quién te necesita |
| *Show all agents (hum ls)* | `overview-windows` / `overview-unix` | la lista de todos los agentes |
| *Manage saved machines (hum machines menu)* | `machines-windows` / `machines-unix` | el menú de máquinas |
| *Open command palette* | `palette` | la [paleta de comandos](#paleta-de-comandos-hum-palette) |

Todas arrancan `bin/hum` directamente, sin PowerShell ni `sh` de por medio (que en
Windows cuestan cerca de un segundo por arranque). Los ids `-windows` y `-unix` de
cada acción hacen lo mismo; se conservan los dos porque los atajos de `config.toml`
ya los usan. Las vistas que solo muestran texto (*Show all agents*, *Who needs me?*)
esperan **cualquier tecla** antes de cerrarse, para que dé tiempo a leerlas.

Puedes asignarles un atajo en `config.toml`; el selector complementa al *Goto* de
herdr (`prefix+g`, que no se toca) con acciones y todas las máquinas:

```toml
[[keys.command]]
key = "prefix+shift+l"
type = "plugin_action"
command = "herdr-human.pick-windows"   # en Linux/macOS: herdr-human.pick-unix
description = "talk to agents (hum pick)"
```

El plugin usa el binario de `bin/`: tras actualizar el código, vuelve a compilarlo
(`go build -o bin\hum.exe .\cmd\hum`) o los popups seguirán con la versión anterior.
El plugin `herdr-rename` sigue funcionando como respaldo; el selector ya renombra
(acción *Rename…*).

## Comandos

| Comando | Qué hace |
|---|---|
| `hum ls [--json] [--verbose] [--local] [--machine NOMBRE] [--server-timeout 3s] [--fresh] [--color auto\|always\|never] [--full-names]` | Lista agentes: máquina → workspace → agente, con el estado de cada máquina en su encabezado |
| `hum id <quién> [--json] [--cmd]` | Imprime el pane id; `--cmd` da el prefijo `herdr --machine … agent prompt <id>` |
| `hum say <quién> <mensaje…> [--wait] [--timeout MS]` | Envía un mensaje a un agente |
| `hum ask <quién> <mensaje…> [--timeout MS] [--lines N]` | Envía, espera a que el agente se asiente y imprime solo la respuesta en stdout (progreso y origen en stderr) |
| `hum attention [--json] [--watch [--interval 5s] [--notify]]` | Quién te necesita: `blocked` y `done` primero, luego `working`; los `idle` solo se cuentan. `--watch` redibuja; `--notify` avisa una vez por transición a `blocked`/`done` |
| `hum pick [--local] [--machine NOMBRE] [--full-names]` | Selector interactivo a pantalla completa (necesita una terminal); ver [Selector](#selector-hum-pick) |
| `hum palette` | Paleta de comandos: busca con coincidencia difusa y ejecuta cualquier comando de herdr o acción de cualquier plugin (necesita una terminal; se abre con una tecla); ver [Paleta de comandos](#paleta-de-comandos-hum-palette) |
| `hum trace on\|off\|show\|clear` | Traza de tiempos de los popups, para medir cuánto tardan en dibujar el primer cuadro; ver [Paleta de comandos](#paleta-de-comandos-hum-palette) |
| `hum new <tipo> [carpeta] [--name N] [--machine M] [--no-focus] [--timeout MS]` | Abre un tab, arranca un agente en él y lo nombra; imprime `maquina/pane`. Sin argumentos, en una terminal pregunta tipo, carpeta y nombre; ver [Crear un agente](#crear-un-agente-hum-new) |
| `hum look <quién> [--lines N]` | Lee la salida reciente de un agente |
| `hum name <quién> <nombre>` / `--clear` | Renombra agente **y** pane a la vez |
| `hum worktree ls [<quién>]` | Lista los worktrees de Git (del workspace del agente, local o remoto; sin `<quién>`, de la carpeta actual) |
| `hum worktree new <rama> [<quién>] [--base REF] [--label TEXTO] [--no-focus]` | Crea un worktree con una rama nueva. **hum nunca pasa `--trust-repository`**: si herdr pide confiar en el repositorio, te lo indica y lo haces tú con herdr |
| `hum worktree rm <quién> [--force] [--yes]` | **Borra** el checkout del worktree del agente. Pregunta (`[y/N]`); sin terminal exige `--yes` |
| `hum alias set <nombre> <quién>` / `rm <nombre>` / `ls` | Nombres cortos para agentes: tras `hum alias set docs odoous/odoo-dev`, `hum say docs "…"` funciona aunque el título del agente cambie. **El alias gana** sobre cualquier otra coincidencia, se sigue una sola vez (un alias no apunta a otro alias) y no se comprueba al guardarlo: se resuelve al usarlo. Se guardan en `aliases.json` dentro de `HERDR_PLUGIN_CONFIG_DIR` o, si no existe, en la carpeta de configuración del usuario (`…\herdr-human\`). Un archivo corrupto no rompe la resolución normal: `hum alias ls` te dice qué pasa y no se sobrescribe |
| `hum close --tab\|--workspace <quién> [--yes]` | **Cierra** el tab o el workspace del agente y detiene todo lo que corre dentro. En una terminal pregunta (`[y/N]`; cualquier otra respuesta cancela); sin terminal exige `--yes` |
| `hum rename --tab\|--workspace <quién> <etiqueta…>` | Renombra el tab o el workspace del agente (la etiqueta puede llevar espacios; no puede empezar por `-` ni llevar caracteres de control) |
| `hum go <quién>` | Enfoca un agente |
| `hum wait <quién> [--until ESTADO]… [--timeout MS] [--notify]` | Espera a que termine (por defecto `idle`, `done` o `blocked`); `--notify` muestra un aviso de herdr en esta máquina |
| `hum machines [--json]` | Lista las máquinas SSH guardadas con su estado (ENABLED/DISABLED) |
| `hum machines enable\|disable\|toggle <nombre>` | Habilita o deshabilita una máquina por etiqueta (o prefijo único) |
| `hum machines menu` | Menú interactivo: número = alternar, `R` refresca, `Q` sale |

### Máquinas guardadas

Sustituye al antiguo script `herdr-machines.ps1`. *Enabled* es estado de
configuración (conexión SSH automática), no una comprobación de que la máquina
esté alcanzable, y deshabilitar un perfil deja corriendo las sesiones y procesos
remotos. Para no pisar a otro cliente, el cambio sigue las mismas garantías del
script original: relee el perfil por su ID inmutable antes de mutar, se niega si
desapareció o si su estado cambió desde que lo viste, no hace nada si ya está en
el estado pedido, confirma el resultado leyendo de nuevo y **nunca reintenta** una
mutación de resultado incierto.

El plugin añade la acción *Manage saved machines (hum machines menu)* a la paleta
(F1); puedes asignarle un atajo:

```toml
[[keys.command]]
key = "prefix+m"
type = "plugin_action"
command = "herdr-human.machines-windows"
description = "manage SSH machines"
```

### Aspecto: colores, glifos y ancho

`ls`, `attention` y `machines menu` usan el mismo kit de render (`internal/ui`):

- **Tema:** se lee `[theme]` (y `[theme.custom]`) de `config.toml` de herdr
  (`%APPDATA%\herdr\config.toml` en Windows). Hay paletas embebidas para
  `catppuccin` y `catppuccin-latte`; es una aproximación de los colores de herdr
  (que no los expone) y con `auto_switch` se usa `name`. Cualquier otro tema, o
  `terminal`, usa los colores ANSI de la terminal. Los colores de estado
  (`blocked` rojo, `working` amarillo, `done` azul, `idle` verde) los fijó el
  autor; no se copiaron de la barra lateral de herdr.
- **Sin color:** `NO_COLOR`, una tubería, o `--color never`. `--color always` lo
  fuerza. `--json` nunca lleva adornos.
- **Glifos:** `○` agente, `◆` pane enfocado, `├─ └─` árbol; `HUM_ASCII=1` usa
  `o`, `*` y `|- \`-`.
- **Ancho:** en una terminal las columnas se pegan a la derecha y los nombres se
  recortan con `…`; en una tubería no se recorta nada. Los títulos tipo Codex
  (`Tarea | proyecto | 5h 8% left`) muestran solo el primer segmento;
  `--full-names` los muestra enteros.

### Selector (`hum pick`)

Marco con título, búsqueda, árbol máquina → workspace → agente, columnas a la
derecha, fila seleccionada a todo el ancho y pie con migas, ruta y teclas, en el
estilo del Goto de herdr. Los agentes que te necesitan (`blocked`, `done`) salen
primero dentro de su workspace.

| Tecla | Acción |
|---|---|
| `↑` `↓` `j` `k`, `PgUp` `PgDn`, `g` `G` | mover la selección |
| `←` `→` | saltar al workspace anterior / siguiente |
| `/` | buscar por nombre, tipo, workspace, máquina, tab, ruta o pane (también letras en orden); lo que coincide se resalta. Se edita con `←` `→` `Inicio` `Fin` `Supr` y `Retroceso`; las líneas de Ask, Say y Rename se editan igual |
| `b` `w` `i` `d` `a` | filtrar `blocked`, `working`, `idle`, `done` / todos |
| `Enter` | abrir el menú de acciones del agente (ver abajo) |
| `Ctrl+A` `Ctrl+S` `Ctrl+L` `Ctrl+N` | atajos directos: ask, say, look, rename |
| `Esc` / `q` / `Ctrl+C` | salir (en la búsqueda, `Esc` la borra) |
| `r` | refrescar los estados de todas las máquinas (sin quitar las filas; el encabezado muestra `refreshing`) |
| `Ctrl+U` | borrar la búsqueda |
| ratón | clic en una fila, rueda para desplazar |

Cada máquina aparece cuando responde (`⠋ connecting 3.1s`, luego su tiempo o
`✖ timeout`), sin bloquear el teclado.

**Menú de acciones** (`Enter` sobre un agente; cada acción tiene su letra):

| Acción | Letra | Qué hace |
|---|---|---|
| Ask… | `a` | pide el mensaje en la línea inferior (`message >`), espera y muestra la respuesta dentro del selector (`── reply ──`, con el origen: tras tu mensaje o las últimas 40 líneas; si el agente queda `blocked` lo dice). Cualquier tecla vuelve a la lista; `Esc` mientras espera deja de esperar y el agente sigue trabajando |
| Say… | `s` | envía el mensaje sin esperar |
| Look | `l` | muestra la salida reciente del terminal (se desplaza con `↑↓` `PgUp` `PgDn`) |
| Rename… | `n` | pide el nombre (`name >`) y valida el formato mientras escribes; vacío borra el nombre |
| Go to it | `g` | lleva el foco de herdr al agente y cierra el selector |
| Wait and notify | `w` | espera en segundo plano y avisa con una notificación de herdr al asentarse; puedes seguir navegando |
| Copy pane id | `c` | copia el pane id (`clip`, `pbcopy`, `wl-copy`, `xclip` o `xsel`) |

En los workspaces locales se muestra la rama git a la derecha (se lee de
`.git/HEAD`, sin ejecutar `git`; `r` la vuelve a leer). El resultado de cada acción
aparece en la segunda línea del pie. Tras Ask, Say,
Rename o Wait se refresca la máquina de ese agente para que su estado sea el
actual; si el refresco falla se avisa y se conservan las filas. Lo que imprime un
agente se muestra sin caracteres de control, para que no pueda mover el cursor ni
cambiar la terminal. `hum` sin argumentos abre el selector cuando la entrada y la
salida son una terminal; con una tubería imprime la ayuda (código 0).

### Paleta de comandos (`hum palette`)

Una paleta estilo F1: una tecla abre un popup con **las acciones de herdr que
tiene sentido lanzar desde una paleta** (workspaces, tabs, panes, worktrees,
máquinas, plugins…; ver [Qué comandos incluye](#qué-comandos-incluye-y-cuáles-no))
y **todas las acciones de todos los plugins instalados**, filtrados con
coincidencia difusa mientras escribes. Elige
uno, pulsa Enter y se ejecuta. Los comandos que necesitan un dato (un nombre, una
carpeta, un workspace de destino) lo preguntan; **Esc** en una pregunta lo cancela
todo. Si algo falla, el popup muestra el motivo en vez de cerrarse en silencio.

Para abrirla, enlaza la acción `palette` de este plugin a una tecla en el
`config.toml` de herdr y recarga la configuración:

```toml
[[keys.command]]
key = "f1"
type = "plugin_action"
command = "herdr-human.palette"
description = "command palette"
```

`type = "plugin_action"` importa: con un atajo de tipo `shell` que abra el pane a
mano, la paleta pierde el contexto desde el que se abrió.

| Tecla | Acción |
|---|---|
| escribir | filtra |
| `↑` `↓` / `ctrl+p` `ctrl+n` | mueve la selección |
| `RePág` / `AvPág`, `Inicio` / `Fin` | página o extremos de la lista |
| `Enter` | ejecuta lo seleccionado |
| `ctrl+w` / `ctrl+u` | borra la última palabra / toda la búsqueda |
| `Esc` / `ctrl+c` | cierra sin ejecutar nada |

Junto a cada entrada se ve su origen (el plugin, o `Herdr`) y la tecla que tiene
enlazada en tu `config.toml`. Lo que usaste hace poco aparece primero.

Al escribir, las letras que coinciden se resaltan en el título y en el origen
(son exactamente las que usa el filtro, así que lo resaltado explica por qué sale
esa fila). Como la búsqueda también mira la descripción y el id, una fila puede
aparecer sin letras resaltadas en el título. La línea de ayuda del pie describe la
entrada seleccionada; si es una acción de plugin sin descripción, muestra su id
(`Plugin action plugin.accion`), que es lo que se enlaza a una tecla.

**Dentro de un popup de herdr (`--embedded`).** Un popup de herdr ya dibuja su
propio marco con el título del pane; si la TUI dibuja otro encima, queda una caja
dentro de otra. Por eso `hum palette` y `hum pick` aceptan `--embedded`: no
dibujan marco ni título, solo la línea que separa la búsqueda y un margen de una
columna, como las vistas nativas de herdr (*Go to*). El selector conserva su barra
de desplazamiento en el borde derecho y la paleta gana las filas que ocupaban los
bordes. El manifest del plugin lo pasa en los panes de la paleta y del selector.
Sin el flag (en una terminal cualquiera) se dibuja el marco propio de siempre, con
su título; no hay detección automática, para que lo que ves dependa de lo que
escribes.

**Por qué abre rápido.** La paleta está pensada para abrir al instante, no solo
para funcionar:

- La acción y el pane arrancan `bin/hum` **directamente**, sin PowerShell ni `sh`
  de por medio (en Windows, `powershell.exe` tarda cerca de un segundo en
  arrancar y la versión original lo lanzaba dos veces). Medido en esta máquina
  con la herramienta de procesos del sistema, desde que se dispara la acción
  hasta que el proceso de la paleta existe: ~70–95 ms con `hum`, contra
  ~2,3–2,4 s con la paleta original en Node.
- **Carga perezosa.** El primer cuadro se dibuja con los comandos que van dentro
  del binario y las acciones de plugins de la última vez (una caché en la
  carpeta de estado del plugin). Las acciones actuales se piden a herdr en
  segundo plano —una sola llamada, `plugin list --json`, en vez de dos— y se
  incorporan cuando llegan; mientras tanto el encabezado dice `loading plugins…`
  y, si herdr no responde, `plugins unavailable` sin perder lo ya dibujado.
- Si no has movido la selección, esta sigue al mejor resultado cuando llegan las
  acciones nuevas; si ya navegaste, se queda donde estaba.
- Los workspaces, las pestañas y los agentes se piden a herdr **solo cuando un
  comando los necesita** (por ejemplo, «Workspace: Next» o un selector), no al abrir.
- El workspace, la pestaña y el pane de origen se leen de la variable
  `HERDR_PLUGIN_CONTEXT_JSON` que herdr ya entrega al popup: no hace falta
  preguntar `pane current` ni pasar variables entre la acción y el pane.

**Medir la apertura (`hum trace`).** Para comprobarlo con números y no a ojo:

```powershell
hum trace on        # enciende la traza (también sirve HUM_TRACE=1 en la terminal)
# ... abre la paleta con su tecla unas cuantas veces ...
hum trace show      # últimas líneas del log
hum trace off       # la apaga (el log se conserva); "hum trace clear" lo borra
```

`hum trace on` crea un archivo marcador en la carpeta de caché de hum
(`%LOCALAPPDATA%\herdr-human` en Windows) en vez de depender de una variable de
entorno, porque los popups heredan el entorno del **servidor** de herdr y no el de
tu terminal. Cada línea lleva la hora, el proceso y los milisegundos desde que
Windows creó el proceso (no desde `main`, así que incluye cargar el `.exe`):

```text
12:25:12.036Z pid=54840 palette open +24.2ms open: asking herdr to open the pane
12:25:12.075Z pid=54840 palette open +63.2ms open: herdr answered
12:25:12.183Z pid=58372 palette +115.3ms environment ready
12:25:12.185Z pid=58372 palette +117.1ms tui: first view rendered (the terminal gets it within one frame)
12:25:12.202Z pid=58372 palette +134.3ms palette: first frame written to the terminal (190.3ms since the trigger)
12:25:12.212Z pid=58372 palette +144.6ms palette: herdr answered with the plugin list
```

«Primer cuadro» es cuando se escribe en la terminal el primer fotograma con el
título (la salida se envuelve conservando el archivo, para que Bubble Tea siga
conociendo el tamaño de la ventana). «Desde el disparo» cuenta desde que arrancó el
proceso de la acción; `hum` se lo pasa al popup con `--env HUM_TRACE_T0`, solo si
la traza está encendida. En esta máquina (Windows, 5 aperturas): ~175–231 ms
(mediana ~190 ms) desde el disparo hasta el primer cuadro. De ellos, lo que hace
`hum` desde `main` hasta escribirlo son ~19 ms; ~100 ms son el arranque del pane
en herdr (un proceso en una consola normal llega a `main` en ~11 ms, el del
popup en ~106–147 ms), que `hum` no controla. Con la traza apagada no escribe nada.

Lo elegido no se ejecuta dentro del popup: mientras está abierto, herdr rechaza
(`ui_busy`) todo lo que abra un pane o popup. Por eso `hum palette` lanza un
proceso aparte (`hum palette run`) que espera a que el popup se cierre —
reintentando mientras herdr siga ocupado— y entonces ejecuta el comando. Si
falla, lo avisa con una notificación de herdr.

Los comandos nativos están en `internal/palette/commands.json` (42 entradas, que
usan 25 comandos distintos de herdr) y viajan dentro del binario. Los
atajos se leen de `HERDR_CONFIG_PATH` o, si no está, del `config.toml` de herdr en
la carpeta de configuración del sistema (`%APPDATA%\herdr` en Windows). Los
recientes y la caché viven en `HERDR_PLUGIN_STATE_DIR`.

#### Qué comandos incluye (y cuáles no)

Las **acciones de plugins** se leen de herdr en cada apertura, así que están
completas: instala un plugin y aparece. Los **comandos nativos de herdr**, en
cambio, son una lista escrita a mano, porque herdr no ofrece una lista de comandos
pensada para esto. Para que no se escape ninguno sin que nadie se entere:

- Cada comando de herdr tiene que estar en `commands.json` **o** en
  `internal/palette/excluded.go` con el motivo de dejarlo fuera.
- El test `TestCatalogAndExclusionsAccountForEveryInstalledHerdrCommand` recorre el
  ayuda del `herdr` instalado y falla si hay un comando en ninguna de las dos
  listas (o una entrada que apunta a un comando que herdr ya no tiene). Con herdr
  0.9.4-preview son 95 comandos: 25 en la paleta y 70 excluidos. Si herdr no está
  instalado (por ejemplo, en CI) el test se salta.
- Lo recorre desde los sustantivos (`workspace`, `pane`, `plugin`…); los comandos
  sueltos de nivel superior (`status`, `update`, `completion`) no se comprueban.

Se excluyen a propósito: lo que solo imprime información (`list`, `get`, `read`;
la paleta ejecuta el comando aparte y nadie vería la salida), lo que usan las
integraciones de agentes, lo destructivo (`worktree remove`, `machine remove`,
`session delete`, `server stop`…, que conviene hacer desde una terminal donde se
pueda confirmar), lo que necesita esa terminal, lo de argumentos libres, lo que
instala código de terceros y lo que `hum` ya resuelve con su propio flujo
(`pick`, `ask`, `name`, `new`).

Para añadir un comando: una entrada en `commands.json` (fuentes de argumento
`literal`, `context`, `input` y `select`; selectores `workspaces`, `tabs`,
`agents`, `machines` y `plugins`) y quitarlo de `excluded.go` si estaba ahí.

### Crear un agente (`hum new`)

```text
$ hum new claude C:\src\dev\raw --name docs
creating claude...
✔ tab created (wS:tA)
✔ claude is ready
✔ named "docs"
local/wS:pA
```

Es la secuencia `tab create` + `agent start` + `pane rename` en una sola orden. El
progreso va a stderr y solo `maquina/pane` a stdout, así que se puede capturar
(`$ref = hum new codex`).

- **Tipo:** uno de los que herdr sabe arrancar (`claude`, `codex`, `gemini`, `pi`…;
  la lista completa está en `herdr agent start --help`). Se comprueba antes de crear
  nada, para que una errata no deje un tab vacío.
- **Carpeta:** por defecto la actual; `~` es tu carpeta personal y una ruta relativa
  cuelga de la actual. En una máquina remota se envía tal cual, y si se omite decide
  herdr (la carpeta de este equipo no existe allí).
- **Máquina:** `hum new odoous/claude …` o `hum new claude … --machine odoous`
  (equivalentes); `local` es este equipo.
- **Nombre:** `--name` (mismas reglas que `hum name`); sin él, el agente se llama
  como su tipo y el pane no se renombra.
- `--no-focus` no lleva el foco al tab nuevo; `--timeout MS` cambia cuánto se espera
  a que el agente esté listo (por defecto 30 s).
- **Si un paso falla** se dice cuál y qué quedó creado, con una pista para limpiarlo
  (`herdr tab close …` o `hum name …`); no se deshace nada en silencio.

### Autocompletado (`hum completion`)

`hum completion bash|zsh|powershell` imprime el script de la shell. Completa
subcomandos, opciones (`--fresh`, `--color`…), sus valores (`--color auto|always|never`,
`--until idle|working|…`, `--machine` con tus máquinas), los tipos de `hum new`, las
subórdenes de `hum machines` y, donde va un `<quién>`, los agentes.

| Shell | Instalación |
|---|---|
| PowerShell | `hum completion powershell \| Out-String \| Invoke-Expression` (añádelo a `$PROFILE`) |
| bash | `source <(hum completion bash)` (añádelo a `~/.bashrc`) |
| zsh | `hum completion zsh > "${fpath[1]}/_hum"` y abre una shell nueva, o `source <(hum completion zsh)` con `compinit` ya cargado |

- **Nunca espera a la red.** Los agentes locales se consultan en vivo (milisegundos)
  y los remotos salen de lo último que guardó la [caché](#rendimiento), aunque haya
  caducado (hasta una hora). La caché se llena con `ls`, `attention`, `pick` y al
  resolver nombres, así que basta con haberlos usado hace poco.
- **Qué nombres sugiere:** `maquina/pane` (`odoous/w1:p2`) y los nombres que `hum`
  entiende (`docs-agent`, `odoous/docs-agent`). Los títulos con espacios (`Configura
  Odoo 18 | …`) no se sugieren; ponle un nombre con `hum name` y aparecerá.
- Descripciones (tipo, estado y título) en zsh y PowerShell. Tras `hum new <tipo>`,
  la carpeta se completa con la shell.
- Todo lo que viene de un agente o de tu configuración se limpia de caracteres de
  control antes de llegar a la línea de comandos.
- Los scripts llaman a `hum __complete --shell S --line TEXTO` (comando interno que
  nunca falla ni escribe en stderr); sirve para depurar lo que sugiere.

### Preguntar y esperar

`hum ask` usa `agent prompt --wait` y luego lee el terminal. La respuesta es una
**heurística**: lo que aparece después del mensaje enviado (se busca el comienzo
del mensaje en pantalla) y, si no se encuentra o no hay nada detrás, las últimas
`--lines` líneas (40); stderr dice cuál de las dos se usó. Si el agente queda
`blocked` no se imprime respuesta: se avisa de qué espera (última línea) y se sale
con `3`; `Ctrl+C` deja de esperar y el agente sigue trabajando. `hum wait` es lo
mismo sin enviar nada.

Las opciones pueden ir en cualquier posición. Si el mensaje de `say` contiene
palabras que empiezan por `-`, ponlas después de `--`.

### Cómo se elige el agente (`<quién>`)

Se prueba en este orden y gana el primer nivel con coincidencias, sin distinguir
mayúsculas:

1. igual al pane id (`w1:p2`), al nombre o al tipo de agente (`codex`);
2. el nombre empieza por lo escrito;
3. el nombre contiene lo escrito;
4. las letras aparecen en orden dentro del nombre (`cnf` → `Configura…`).

Si un nivel tiene varias coincidencias, `hum` falla y las lista como
`maquina/pane` para que elijas. El prefijo `maquina/` (o `local/`) restringe la
búsqueda a un servidor y, además, evita consultar las demás máquinas.

`--machine NOMBRE` hace lo mismo con otra sintaxis: `hum ask odoous/codex "…"` es
equivalente a `hum ask codex "…" --machine odoous`. `--machine local` nombra el
servidor local y se puede repetir para buscar solo entre varias máquinas. Con
cualquiera de las dos formas no se consulta a las máquinas no mencionadas, así que
una máquina habilitada pero caída (por ejemplo *WSL Ubuntu*) no genera avisos.
(En `ls`, `--machine` sigue incluyendo siempre el servidor local.)

## Errores con pista

Un error dice qué pasó y, cuando la causa es conocida, una línea `hint:` con qué
hacer: consulta ambigua (`use "maquina/pane", e.g. …`), agente inexistente
(`run "hum ls"`), máquina inalcanzable que podría tener al agente, agente
`blocked` (`hum go …`), agente cerrado y nombre inválido. En una terminal
interactiva (entrada **y** salida), una consulta ambigua pregunta
`Which one? [1-N, Enter to cancel]` en vez de fallar. Enter, `q` o `Ctrl+C`
cancelan al instante (`cancelled`, código 1); una respuesta que no sea un número de
la lista conserva el error original con su pista. Un fallo sin explicación no inventa pistas.

## Códigos de salida

`0` correcto · `1` error (no encontrado, ambiguo, herdr falló) · `2` uso incorrecto ·
`3` el agente quedó `blocked` esperando a la persona (`wait`; con `--until blocked`
ese estado es un éxito) · `4` venció el plazo de `--timeout` (`wait`).

## Diseño

```text
cmd/hum                punto de entrada (señales, os.Exit)
internal/cli           subcomandos, flags y formato de salida
internal/resolve       consulta de una persona → un agente
internal/inventory     local + máquinas en paralelo, con timeout y fallo aislado
internal/actions       say, look, name, focus sobre comandos de herdr
internal/herdr         Runner sobre el ejecutable, sobre JSON y errores de la API
internal/herdrtest     Runner falso para pruebas
internal/gitinfo       rama de git de una carpeta, leída de .git/HEAD
internal/complete      autocompletado (bash, zsh, PowerShell) sin red
```

Una máquina que no responde se avisa por `stderr` y no impide ver las demás;
solo un fallo del servidor local es fatal. Cada consulta tiene un límite de 8 s
(`--server-timeout 3s` lo cambia).

### Rendimiento

Cada llamada remota de herdr cuesta ~1,3 s por el SSH, así que `hum` hace **una**
por máquina (`herdr api snapshot`, que trae agentes y workspaces juntos) y todas
las máquinas en paralelo; el tiempo total es el de la más lenta. Medido con
`odoous`: 2,75 s con dos llamadas seguidas → 1,4 s con el snapshot. Si `hum ls`
tarda, `hum ls --verbose` imprime en `stderr` cuánto tardó cada servidor
(`timings: local 21ms, odoous 1.3s, …`) para ver cuál lo frena; con
`--machine`/`--local` o `--server-timeout` puedes saltarte o recortar el lento.
Solo si un servidor responde que no soporta el snapshot se usan las dos listas.

**`ls` progresivo.** En una terminal, `hum ls` dibuja el árbol en cuanto pasa un
instante (150 ms) y lo va **redibujando en su sitio**: lo local y lo cacheado
aparecen ya, y cada máquina que aún no responde muestra `⠋ connecting 2.1s` hasta
que llega su tiempo o su `✖ timeout`. El resultado final es el mismo árbol de
siempre. Si todo responde en ese margen se imprime una sola vez, sin parpadeo. No se
redibuja (y se imprime una vez al final) si el árbol no cabe en la altura de la
terminal, si la altura no se conoce, con una tubería o con `--json`.

**Caché.** La respuesta de cada máquina **remota** se guarda 10 s en la carpeta de
caché del usuario (`%LocalAppData%\herdr-human\inventory.json` en Windows;
`~/.cache/herdr-human/` en Linux), con permisos solo para ti. Así, repetir `hum ls`
o resolver un nombre (`hum say`, `ask`, `look`, `id`, `name`, `go`, `wait`) justo
después no vuelve a pagar la espera de SSH.

- El servidor local **nunca** se cachea: responde en milisegundos y sus estados
  cambian a cada rato. `hum attention` y `hum pick` tampoco la leen (siempre en
  vivo), pero lo que consultan **renueva** la caché para que `ls` y el
  autocompletado tengan datos recientes.
- Una máquina que falla o da timeout no se guarda, así que se reintenta.
- `ls` marca lo que viene de la caché en el encabezado de la máquina
  (`cached 3s`); `--json` no cambia.
- `--fresh` (en `ls` y en los comandos que resuelven un nombre) ignora lo guardado
  y lo renueva.
- Si un nombre no aparece y parte de los datos eran de la caché, `hum` consulta en
  vivo antes de darlo por inexistente (puede haber nacido hace unos segundos).
- `hum new` y `hum name` borran la caché, para no servir datos anteriores al cambio.
- El plazo por máquina (`--server-timeout`, 8 s por defecto) se respeta aunque
  `herdr` deje un proceso `ssh` colgado: se corta como mucho 1 s después.

## Desarrollo

```powershell
pwsh -NoProfile -File scripts\check.ps1
```

Ejecuta, en este orden, `gofmt -s`, `goimports`, `go vet`, `staticcheck` y
`go test -race`, y falla en el primer problema. Hay que pasarlo antes de cada
commit. `goimports` y `staticcheck` son opcionales: si faltan (o si
`staticcheck` es demasiado antiguo para el toolchain de Go) se avisa y se
continúa, pero el aviso no significa que el código haya pasado ese linter.

Las pruebas usan un `Runner` falso, no ejecutan herdr. Todo el código y los
mensajes de la CLI van en inglés; la documentación y los comentarios, en español.

### Prueba de humo contra herdr real

```powershell
go build -o bin\hum.exe .\cmd\hum
pwsh -NoProfile -File scripts\smoke.ps1                     # solo esta máquina
pwsh -NoProfile -File scripts\smoke.ps1 -Machine odoous     # y una máquina remota
```

Es de **solo lectura**: lanza `help`, `ls`, `attention`, `machines`, `id`, `look`,
`completion`, `__complete` y `alias ls` (esta, con una carpeta temporal vacía);
nunca envía, crea, renombra, cierra ni borra. Un test
(`cmd\hum\smoke_test.go`) falla si el script lanza otra orden. Cada paso acaba en
`PASS`, `FAIL` o `SKIP`; un `SKIP` (por ejemplo, una máquina que no responde)
significa «no comprobado» y no cuenta como correcto. Sale con 1 si algún paso
falla. No imprime títulos, rutas ni nombres de agentes. Necesita herdr en marcha,
por eso `check.ps1` no la ejecuta. Como cualquier `ls`, puede actualizar la caché
de inventario de hum.

## Agradecimientos

La paleta de comandos (`hum palette`) nace del plugin
**[herdr-command-palette](https://github.com/barnuri/herdr-command-palette)** de
[barnuri](https://github.com/barnuri), con licencia MIT. Gracias por la idea, por
el catálogo de comandos de herdr (`commands.json`) y por el diseño de la
interacción —la búsqueda difusa, los recientes, los atajos mostrados junto a cada
acción y el reparto entre la acción que abre el popup y el proceso que ejecuta lo
elegido—, que aquí se han portado a Go.

Si solo quieres la paleta, sin el resto de `hum`, el plugin original sigue siendo
una gran opción: `herdr plugin install barnuri/herdr-command-palette`. El aviso de
licencia completo está en [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Limitaciones conocidas (v0)

- El selector (`hum pick`) es una primera versión: el popup del plugin sigue
  mostrando solo `hum ls`, y la rama git solo aparece en workspaces del servidor
  local (en una máquina remota costaría otra llamada por SSH).
- El texto de `say` debe ir inmediatamente después del pane id: herdr no admite
  `--`, y `hum` ya lo tiene en cuenta.
- `--machine` limita las máquinas guardadas que se consultan; el servidor local
  siempre se incluye.
