# Diseño de la interfaz de herdr-human

Estado: **propuesta pendiente de aprobación** (revisión 2, tras ver el pane
nativo *Goto* de herdr). Ninguna tarea de implementación empieza hasta que este
documento se apruebe (puerta `G0` en `odd/tasks/herdr-human-ux.md`).

Objetivo: que herdr sea fácil para una persona, con el mismo nivel de calidad
que la interfaz nativa de herdr. Todo lo que se muestra al usuario (mensajes,
etiquetas, teclas) va en inglés porque es código; este documento y los
comentarios, en español.

## 0. Referencia de calidad: el pane nativo *Goto*

El usuario compartió una captura del **Goto picker** de herdr (`prefix+g`,
título "Go to") como listón de calidad. Según la documentación oficial
(`keyboard.mdx`, v0.9.5), lista cada agente o terminal en su propia fila,
agrupado por workspace, y se salta a él con Enter. Lo que hace bien, y cómo lo
aplicamos:

| Elemento nativo (visto en la captura) | Cómo lo aplica `hum` |
|---|---|
| Marco con el título incrustado en el borde ("Go to") y regla bajo la búsqueda | Mismo marco; título "Talk to" |
| Línea de búsqueda `/ search agents and terminals` con el contador a la derecha (`35 terminals`) | `/ search agents` + `14 agents · 2 need you` |
| Árbol con `├─ └─` por workspace; encabezado en negrita y la rama git alineada a la derecha | Árbol máquina → workspace → agente; el encabezado de máquina muestra su estado de carga a la derecha |
| Columnas alineadas a la derecha en tono tenue (tipo de agente, estado) | Igual |
| Fila seleccionada con una barra de color a todo el ancho | Igual, con el token de color de selección del tema |
| `○` agente, `·` terminal plana, `◆` pane actual | Se reutilizan tal cual (ver §3) |
| Pie con migas (`raw / 3 / wS:p4`), ruta del destino y una línea de teclas | Igual |
| Barra de desplazamiento, rueda del ratón, clic en fila y en la barra | Igual |
| Búsqueda por nombre, tipo de agente, tab, workspace, rama y ruta, con edición de cursor y `ctrl+u` | El resolutor se amplía a esos campos |
| `↑↓`/`j/k` mueven, `←→` saltan de workspace, `/` busca, `b/w/i/d` filtran, `a` muestra todo, `Enter` abre, `Esc` cierra | Mismas teclas; `Enter` abre el menú de acciones |

### Qué no hay que duplicar

*Goto* ya sirve para **saltar** a un pane. `hum` no lo reemplaza ni usa su
atajo. Ofrece lo que *Goto* no hace: otras **acciones** (preguntar, escribir,
leer, renombrar, esperar), una **vista de atención**, uso desde **scripts y
agentes**, y los agentes de **otras máquinas** en la misma lista. (En la captura
solo hay un grupo `Local`; no he verificado si *Goto* muestra también máquinas
remotas.) Saltar sigue disponible como una acción más.

## 1. Principios

1. **Nadie debería copiar un ID.** Se elige por nombre, por lista o por
   búsqueda; el ID solo aparece como detalle (en el pie).
2. **Se siente nativo.** Misma gramática visual y de teclas que herdr: quien ya
   usa *Goto* no tiene que aprender nada.
3. **Lo importante primero.** Lo que necesita tu atención (`blocked`, `done`)
   se ve antes que lo que trabaja o está inactivo.
4. **Nunca una pantalla muda.** Si algo tarda (una máquina remota: 1,3–5 s), se
   dibuja lo que ya se sabe y se indica qué falta y cuánto lleva.
5. **Seguro por defecto.** Lo destructivo pide `y/N`; los cambios de resultado
   incierto no se reintentan; siempre se muestra la máquina destino.
6. **Humano y máquina por separado.** Lo legible va a `stdout` con color si es
   una terminal; con `--json` o una tubería la salida es estable y sin adornos.
   Avisos y progreso van a `stderr`.

## 2. Gramática de comandos

Verbo + quién + resto. `<quién>` es siempre lo mismo (nombre, `maquina/nombre`,
parcial, difuso, alias o ID).

| Intención | Comando | Estado |
|---|---|---|
| Ver todo | `hum ls` | existe (se pule) |
| ¿Quién me necesita? | `hum attention [--watch]` | nuevo |
| Hablar | `hum say <quién> <texto>` | existe |
| Preguntar y ver la respuesta | `hum ask <quién> <texto>` | nuevo |
| Esperar | `hum wait <quién> [--notify]` | nuevo |
| Leer | `hum look <quién>` | existe |
| Nombrar | `hum name <quién> <nombre>` | existe |
| Ir a él | `hum go <quién>` | existe |
| Crear | `hum new <tipo> [carpeta] [--name N]` | nuevo |
| Máquinas | `hum machines …` | existe |
| Alias | `hum alias set\|rm\|ls` | nuevo |
| Autocompletado | `hum completion powershell\|zsh\|bash` | nuevo |
| Todo en una pantalla | `hum` (sin argumentos, en terminal) | nuevo: abre el selector |

`hum` sin argumentos en una terminal abre el selector; sin terminal (tubería)
imprime la ayuda, como hasta ahora.

Códigos de salida: `0` correcto · `1` error · `2` uso incorrecto · `3` el
agente quedó `blocked` (en `ask`/`wait`) · `4` se acabó el tiempo.

## 3. Lenguaje visual

### Tema: igualar el de herdr

Un programa dentro de un pane no hereda el tema de herdr: sus colores ANSI salen
de la terminal anfitriona. Para verse nativo hay que usar los mismos colores
que herdr. herdr **no expone** los colores resueltos por CLI ni por la API
(`herdr api schema` y `herdr config` no los devuelven), así que la propuesta es:

1. Leer `[theme]` del `config.toml` del usuario (`name`, `auto_switch`,
   `light_name`/`dark_name` y los tokens de `[theme.custom]`: `accent`,
   `panel_bg`, `selection_bg`, `active_row_bg`, `surface0/1`, `overlay0/1`,
   `text`…).
2. Llevar embebida la paleta de los temas incorporados, **empezando por
   `catppuccin` y `catppuccin-latte`** (los del usuario), y color de 24 bits.
3. Aplicar encima los tokens `custom` del usuario.
4. Tema `terminal`, tema desconocido, `NO_COLOR` o salida sin terminal: usar la
   paleta ANSI de la terminal o ninguna.

Esto es una **aproximación** mantenida a mano; si herdr cambia sus paletas hay
que actualizarla. Por eso `hum` debe verse bien aunque el tema no coincida.

### Estados y glifos

Se reutiliza el vocabulario de herdr (`concepts.mdx`):

| Estado | Significado en herdr |
|---|---|
| `blocked` | El agente necesita entrada, aprobación o una decisión |
| `working` | El agente está ejecutándose |
| `done` | Terminó y todavía no lo has mirado |
| `idle` | Terminó o espera, y ya lo viste |

Glifos confirmados en la captura: `○` agente (verde cuando está `idle`), `·`
terminal plana, `◆` pane actual. **Decidido en B0** (sin capturas, por
delegación del usuario): todos los agentes llevan `○` pintado con el color del
estado (`blocked` rojo, `working` amarillo, `done` azul, `idle` verde). `hum` no
usa `◆` para ningún estado, porque en herdr significa "pane actual".

Reglas: sin color si `NO_COLOR` está definido o la salida no es una terminal;
`--color auto|always|never` lo fuerza; `HUM_ASCII=1` usa respaldo ASCII. Nunca
se depende solo del color: el texto del estado siempre está en su columna.

### Nombres

- Se recortan al ancho disponible con `…`. El ancho sale de la terminal o de
  `COLUMNS` (80 si no se sabe).
- Los títulos de Codex traen ruido (`Tarea | proyecto | Tarea | 5h 8% left |
  weekly 67% left`; la captura los muestra cortados a media palabra). Propuesta:
  mostrar solo el primer segmento antes de ` | `, desactivable con
  `--full-names`.
- Si el usuario puso un nombre con `hum name`, ese manda sobre el título.

## 4. Pantallas

### 4.1 `hum ls` (pulido, mismo lenguaje que el selector)

```
Local
 raw                                                                 main
 ├─ ○ wS:p1  Busca el plan de implementación               codex    idle
 ├─ ○ wS:p3  mi-proyecto                            codex    idle
 └─ ◆ wS:p4  Pane ID del workspace odoous                  claude   working
odoous                                                           1.3s
 odoo-inc
 └─ ○ w1:p2  odoous-odoo-dev                               codex    idle
remoto2                                                    ✖ timeout 8s
```

- Árbol y columnas alineadas a la derecha, como el pane nativo; aquí el ID sí
  se muestra porque es salida para copiar y pegar.
- El estado de cada máquina (tiempo, o el error) aparece en su encabezado, no
  solo como aviso en `stderr`.
- En terminal es **progresivo**: lo local aparece al instante y las máquinas se
  añaden según responden (spinner junto al nombre mientras esperan). Con
  tubería o `--json` espera a todas y la salida es estable.

### 4.2 `hum attention`

```
Needs you (2)
  ○ odoous/odoo-dev      codex   blocked
  ○ local/importer       codex   done

Working (1)
  ○ local/wS:p4          claude  working

Idle: 3 more (hum ls)
```

(Los glifos y colores finales se fijan en B0.)

- Orden: blocked, done, working. Los `idle` solo se cuentan.
- `--watch` redibuja en el sitio cada pocos segundos. Cuando un agente pasa a
  `blocked` o `done` lanza `herdr notification show` (una vez por transición).
- No se muestran duraciones ("hace 4 min"): herdr no expone marcas de tiempo.
- `done` significa "terminó y no lo has mirado" **en el cliente**; la CLI/API
  usa el estado visto del servidor, donde `idle` y `done` son ambos "listo para
  recibir entrada". La vista lo trata como un solo grupo "listo".

### 4.3 `hum ask`

```
$ hum ask odoous/dev "run the tests"
→ odoous/w1:p2  odoous-odoo-dev
  ⠹ working 12s
  ✔ done in 18s

── reply ──────────────────────────────────────────
All 42 tests passed.
```

- Envía con `agent prompt --wait` y, al terminar, lee la salida reciente.
- **Extracción de la respuesta (decisión abierta):** se toma lo que aparece en
  el terminal después del texto enviado; si no se puede localizar, se muestran
  las últimas `--lines` (40). Es una heurística y la pantalla lo dice:
  `(last 40 lines)`.
- Si el agente queda `blocked`, no se espera más: "waiting for you: <última
  línea>" y sale con código `3`.
- `Ctrl+C` deja de esperar; el agente sigue trabajando.

### 4.4 El selector (`hum`)

Mismo marco, mismas teclas y mismo orden visual que *Goto*, pero la acción
principal es **actuar** sobre el agente, no solo saltar a él.

```
┌ Talk to ─────────────────────────────────────────────────────────────────┐
│ / search agents                                    14 agents · 2 need you │
├──────────────────────────────────────────────────────────────────────────┤
│ Local                                                                    │
│  raw                                                                main │
│  ├─ ○ Busca el plan de implementación                  codex       done  │
│  ├─ ○ mi-proyecto                               codex       idle  │
│██└─ ◆ ○ Pane ID del workspace odoous███████████████████ claude  working██│
│  agente-demo                                  claude/vector-authoring-files│
│  ├─ ○ agente-gestor                                claude      idle  │
│  └─ ○ Grafo del código con graphify                    claude      idle  │
│ odoous                                                              1.3s │
│  odoo-inc                                                                │
│  ├─ ○ Analiza login de Codex                           codex    blocked  │
│  └─ ○ odoous-odoo-dev                                  codex       idle  │
│ remoto2                                                ⠋ connecting 3.1s │
├──────────────────────────────────────────────────────────────────────────┤
│ local / raw / wS:p4                                                      │
│ C:\ruta\a\raw                                       │
│ ↑↓/j/k rows · ←→ group · / search · b/w/i/d/a filter · enter actions · esc│
└──────────────────────────────────────────────────────────────────────────┘
```

(La barra de selección se dibuja con el color `selection_bg` del tema; aquí se
marca con bloques.)

- **Búsqueda** (`/`): nombre, tipo, tab, workspace, rama, ruta y máquina, con
  edición de cursor, `Backspace` y `ctrl+u`, y los mismos niveles de coincidencia
  que el resolutor (exacto, prefijo, contiene, difuso). Las coincidencias se
  resaltan.
- **Filtros** fuera de la búsqueda, como en *Goto*: `b` blocked, `w` working,
  `i` idle, `d` done, `a` todo. Por defecto solo se listan agentes (se puede
  actuar sobre ellos); `a` añade las terminales planas (`·`).
- **Orden:** lo que necesita atención primero dentro de cada workspace; los
  grupos conservan el orden de herdr.
- **Streaming:** lo local aparece al instante; cada máquina añade sus filas al
  llegar. El encabezado de cada máquina muestra su estado a la derecha
  (`⠋ connecting 3.1s`, `1.3s`, `✖ timeout 8s`). Nunca bloquea el teclado.
- **Rama git:** a la derecha del workspace, cuando es barata de obtener
  (checkout local). Para máquinas remotas se omite (costaría otra llamada por
  SSH). Decisión abierta.
- **Pie:** migas `máquina / workspace / pane`, la ruta del destino y la línea de
  teclas. En el pie sí aparece el ID.
- **Ratón:** clic en fila, rueda y barra de desplazamiento, como en *Goto*.
- `Enter` abre el menú de acciones sobre la fila elegida:

```
┌ odoous / odoo-dev ─────────┐
│ ▸ Ask…          a          │
│   Say…          s          │
│   Look          l          │
│   Rename…       n          │
│   Go to it      g          │
│   Wait and notify  w       │
│   Copy pane id  c          │
└────────────────────────────┘
```

- `Ask…`, `Say…` y `Rename…` piden el texto en una línea inferior (`message >`),
  con `Esc` para cancelar; `Rename…` valida el formato mientras se escribe.
- `Ask…` pasa a la vista de la pantalla 4.3 dentro del selector y vuelve a la
  lista al pulsar cualquier tecla.
- `Go to it` delega en `agent focus` y cierra el selector (el equivalente a
  *Goto*).
- Atajos directos sin abrir el menú: `ctrl+a` ask, `ctrl+s` say, `ctrl+l` look,
  `ctrl+n` rename (con `ctrl` para no chocar con los filtros `a/b/w/i/d`).
- `?` muestra todas las teclas; `Esc` sube un nivel y, en la lista, cierra.

### 4.5 `hum new`

```
$ hum new claude ~/src/dev/raw --name docs
✔ tab created in local / raw
✔ claude is ready
✔ named "docs"
→ local/w3:p5
```

Con argumentos omitidos, en una terminal se pregunta: tipo (lista), carpeta (por
defecto la actual) y nombre (opcional). Es la secuencia `tab create` +
`agent start` + `name` en una sola orden; si un paso falla, se dice cuál y qué
quedó creado (no se deshace nada en silencio).

### 4.6 `hum machines menu` (pulido)

Mismas teclas de siempre; las columnas se alinean y se usa el marco y la
barra de selección del tema:

```
Herdr machines
Enabled = automatic SSH connection; not a live connection check.
Disabling a profile leaves remote sessions and processes running.

[1] ENABLED   odoous      servidor-odoous           (session: default)
[2] DISABLED  remoto1     servidor-1      (session: default)
[3] DISABLED  remoto2     servidor-2     (session: default)
[4] DISABLED  WSL Ubuntu  127.0.0.1:2222       (session: default)

Number: toggle | R: refresh | Q: quit >
```

### 4.7 Errores y ambigüedad

Forma fija: qué pasó y, si hay, qué hacer.

```
hum: "codex" matches 3 agents
  local/wS:p1     codex  mi-proyecto
  odoous/w1:p1    codex  Analiza login de Codex
  odoous/w1:p2    codex  odoous-odoo-dev
hint: use machine/name, e.g. "odoous/odoo-dev"
```

En una terminal, la ambigüedad se resuelve con una pregunta numerada ("Which
one? [1-3]") en vez de fallar. Mensajes con pista para los casos frecuentes:
`agent_blocked` ("the agent is waiting for you; hum go odoous/dev"), máquina
inalcanzable, nombre inválido.

## 5. Velocidad

- `ls` y el selector: render progresivo (§4.1 y §4.4).
- **Caché** de inventario con vida corta (propuesta: 10 s) en el directorio de
  estado del usuario: sirve al autocompletado y a las aperturas repetidas.
  `--fresh` lo ignora. Se muestra "cached 3s ago" cuando se usa.
- El autocompletado solo lee el caché (nunca espera a la red).

## 6. Descubrimiento y autocompletado

- `hum completion powershell|zsh|bash` imprime el script; completa subcomandos,
  opciones y `<quién>` con los nombres del caché.
- `hum` sin argumentos abre el selector, que es la mejor ayuda.
- `hum help <comando>` con ejemplos reales.

## 7. Alias y configuración

- Archivo en el directorio de configuración del plugin
  (`HERDR_PLUGIN_CONFIG_DIR` o equivalente de usuario):
  `alias docs = odoous/odoo-dev`.
- El alias gana en el resolutor (nivel 0), así que `hum say docs "…"` funciona
  aunque el título del agente cambie.
- `hum alias set docs odoous/odoo-dev`, `hum alias rm docs`, `hum alias ls`.
  **Hecho en D4.** El archivo se llama `aliases.json`; se sigue un solo salto
  (un alias no apunta a otro) y el destino no se valida al guardar.

## 8. Puntos de entrada en herdr

| Atajo | Hoy | Propuesta |
|---|---|---|
| `prefix+g` | *Goto* nativo | no se toca |
| `prefix+shift+l` | `hum ls` en popup | abre el **selector** ("Talk to"). **Hecho en B6** |
| `prefix+m` | menú de máquinas | igual |
| `prefix+shift+a` | plugin `herdr-rename` | el selector ya renombra; `herdr-rename` queda como respaldo |
| `F1` | paleta con todas las acciones | añade "Agents", "Needs you", "Machines" |

## 9. Decisiones abiertas (necesito tu OK)

1. **Biblioteca de TUI.** Con este listón de calidad (marco, color de 24 bits,
   ratón, barra de desplazamiento) la recomiendo con más fuerza: Bubble Tea +
   Lip Gloss, versiones fijas. El binario sigue siendo estático y no hay que
   instalar nada para usarlo, pero el repo deja de tener cero dependencias.
   Alternativa: ANSI a mano (más código propio y más errores de terminal).
2. **Glifos:** reutilizar los de herdr (confirmar con capturas, tarea B0), con
   respaldo ASCII, o fijar un juego propio.
3. **`hum` sin argumentos abre el selector** en terminal (recomendado) o sigue
   mostrando la ayuda.
4. **Extracción de la respuesta en `ask`:** ¿te sirve la heurística "lo que
   aparece tras el texto enviado, con respaldo a las últimas 40 líneas"?
5. **Recorte de títulos de Codex** (primer segmento antes de ` | `): ¿activo por
   defecto? (*Goto* los muestra completos y cortados por el ancho.)
6. **Nombre de la vista de atención:** `attention` o `inbox`.
7. **Tema:** ¿igualar el de herdr leyendo `config.toml` y con paletas
   embebidas (empezando por catppuccin), o usar la paleta ANSI de la terminal?
8. **Posicionamiento:** ¿de acuerdo en que `hum` complementa a *Goto* (acciones
   y todas las máquinas) y no lo duplica?
9. **Rama git** junto al workspace: solo para checkouts locales (barato) o
   ninguna.
