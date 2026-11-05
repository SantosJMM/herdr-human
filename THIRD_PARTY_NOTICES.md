# Avisos de terceros

`hum palette` (paquetes `internal/palette` y `internal/tui/palette*.go`) es una
adaptación a Go del plugin **herdr-command-palette**, que se distribuye con
licencia MIT. El catálogo `internal/palette/commands.json` es una copia de su
`commands.json`, y la lógica de búsqueda difusa, recientes, lectura de atajos y
resolución de argumentos sigue el diseño de su código en JavaScript.

- Proyecto original: https://github.com/barnuri/herdr-command-palette
- Autor: barnuri

Esta es la licencia del proyecto original, tal como la publica su autor:

```text
MIT License

Copyright (c) 2026 barnuri

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

El resto de `herdr-human` se distribuye con la licencia MIT de este repositorio
(ver `LICENSE`).
