package complete

import (
	"fmt"
	"strings"
)

// Format escribe el resultado como lo espera el script de cada shell, una
// sugerencia por línea. Si se pide completar archivos, la única línea es "@files".
func Format(shell string, r Result) string {
	if r.Files {
		return "@files\n"
	}
	var sb strings.Builder
	for _, c := range r.Candidates {
		switch shell {
		case "zsh":
			// _describe separa valor y descripción con ":", y los ":" del valor se escapan.
			value := strings.NewReplacer(`\`, `\\`, ":", `\:`).Replace(c.Value)
			sb.WriteString(value)
			if c.Desc != "" {
				sb.WriteString(":" + c.Desc)
			}
		case "powershell":
			sb.WriteString(c.Value)
			if c.Desc != "" {
				sb.WriteString("\t" + c.Desc)
			}
		default:
			sb.WriteString(c.Value)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// Script devuelve el script de autocompletado de una shell. Todos llaman a
// "hum __complete --shell S --line TEXTO", con el texto de la línea hasta el
// cursor, y entienden la línea "@files" (completar archivos con la shell).
func Script(shell string) (string, error) {
	switch shell {
	case "bash":
		return bashScript, nil
	case "zsh":
		return zshScript, nil
	case "powershell":
		return powershellScript, nil
	}
	return "", fmt.Errorf("unsupported shell %q (want %s)", shell, strings.Join(Shells, ", "))
}

const bashScript = `# hum: autocompletado para bash.
# Instálalo con:   source <(hum completion bash)
# o añade esa línea a ~/.bashrc.
_hum_complete() {
    local line="${COMP_LINE:0:$COMP_POINT}" word prefix out entry
    out="$("${COMP_WORDS[0]}" __complete --shell bash --line "$line" 2>/dev/null)" || return 0
    COMPREPLY=()
    if [[ "$out" == "@files"* ]]; then
        compopt -o default 2>/dev/null
        return 0
    fi
    # bash trata ":" como separador de palabra; se quita de cada sugerencia lo que
    # ya está escrito antes del último ":".
    word="${line##*[[:space:]]}"
    prefix=""
    if [[ "$word" == *:* ]]; then
        prefix="${word%"${word##*:}"}"
    fi
    while IFS= read -r entry; do
        [[ -z "$entry" ]] && continue
        entry="${entry#"$prefix"}"
        COMPREPLY+=("$(printf '%q' "$entry")")
    done <<<"$out"
}
complete -F _hum_complete hum
`

const zshScript = `#compdef hum
# hum: autocompletado para zsh.
# Instálalo con:   hum completion zsh > "${fpath[1]}/_hum"   (y abre una shell nueva)
# o, en la sesión actual con compinit ya cargado:   source <(hum completion zsh)
_hum() {
    local out
    local -a items
    out="$(${words[1]} __complete --shell zsh --line "${BUFFER[1,CURSOR]}" 2>/dev/null)"
    if [[ "$out" == "@files"* ]]; then
        _files
        return
    fi
    [[ -n "$out" ]] || return 1
    items=("${(@f)out}")
    _describe -t hum 'hum' items
}
compdef _hum hum
`

const powershellScript = `# hum: autocompletado para PowerShell.
# Instálalo con:   hum completion powershell | Out-String | Invoke-Expression
# o añade esa línea a tu $PROFILE.
Register-ArgumentCompleter -Native -CommandName hum -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $exe = $commandAst.CommandElements[0].Value
    $line = $commandAst.Extent.Text
    $upto = $cursorPosition - $commandAst.Extent.StartOffset
    if ($upto -lt 0) { $upto = 0 }
    if ($upto -gt $line.Length) {
        # El AST no incluye los espacios finales: se añaden hasta el cursor, porque
        # indican que se empieza una palabra nueva.
        $line = $line + (' ' * ($upto - $line.Length))
    } else {
        $line = $line.Substring(0, $upto)
    }
    $lines = & $exe __complete --shell powershell --line $line 2>$null
    foreach ($l in @($lines)) {
        if ($l -eq '@files') { return }
        $value, $desc = $l -split "` + "`t" + `", 2
        if (-not $desc) { $desc = $value }
        $text = $value
        if ($value -match "[\s']") { $text = "'" + ($value -replace "'", "''") + "'" }
        [System.Management.Automation.CompletionResult]::new($text, $value, 'ParameterValue', $desc)
    }
}
`
