package cli

func completionScript(shell string) string {
	switch shell {
	case "bash":
		return bashCompletionScript
	case "zsh":
		return zshCompletionScript
	case "fish":
		return fishCompletionScript
	case "powershell":
		return powershellCompletionScript
	case "elvish":
		return elvishCompletionScript
	case "nushell":
		return nushellCompletionScript
	default:
		return ""
	}
}

const bashCompletionScript = `# bash completion for bast
_bast() {
  local cur cword
  cur="${COMP_WORDS[COMP_CWORD]}"
  cword="${COMP_CWORD}"

  local -a request
  local i
  for ((i = 1; i <= cword; i++)); do
    request+=("${COMP_WORDS[i]}")
  done

  local out
  out="$("${COMP_WORDS[0]}" __complete -- "${request[@]}" 2>/dev/null)" || return

  local directive="" line value
  local -a values
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == :* ]]; then
      directive="${line:1}"
      continue
    fi
    value="${line%%$'\t'*}"
    [[ -n "$value" ]] && values+=("$value")
  done <<< "$out"

  case "$directive" in
  files)
    COMPREPLY=($(compgen -f -- "$cur"))
    return
    ;;
  dirs)
    COMPREPLY=($(compgen -d -- "$cur"))
    return
    ;;
  esac

  COMPREPLY=()
  local v
  for v in "${values[@]}"; do
    COMPREPLY+=("$v")
  done
}

complete -F _bast bast
`

const zshCompletionScript = `#compdef bast

_bast() {
  local -a request
  integer i
  for ((i = 2; i <= CURRENT; i++)); do
    request+=("${words[i]}")
  done

  local output
  output="$("${words[1]}" __complete -- "${request[@]}" 2>/dev/null)" || return 1

  local directive="" line value desc
  local -a args
  local -a with_desc
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == :* ]]; then
      directive="${line[2,-1]}"
      continue
    fi
    if [[ "$line" == *$'\t'* ]]; then
      value="${line%%$'\t'*}"
      desc="${line#*$'\t'}"
      with_desc+=("${value}:${desc}")
    else
      value="$line"
      with_desc+=("$value")
    fi
    args+=("$value")
  done <<< "$output"

  case "$directive" in
  files)
    _files
    return
    ;;
  dirs)
    _files -/
    return
    ;;
  esac

  if (( ${#with_desc} )); then
    _describe -t bast-values 'bast' with_desc
  elif (( ${#args} )); then
    compadd -a args
  fi
}

compdef _bast bast
`

const fishCompletionScript = `function __bast_complete
    set -l args (commandline -opc)
    set -l current (commandline -ct)
    if test (count $args) -eq 0
        return
    end
    set -l bin $args[1]
    set -e args[1]
    set -l out
    if test -z "$current"
        set out (command $bin __complete -- $args '' 2>/dev/null)
    else
        set out (command $bin __complete -- $args $current 2>/dev/null)
    end
    set -l directive nofiles
    set -l values
    for line in $out
        if string match -q ':*' -- $line
            set directive (string sub -s 2 -- $line)
            continue
        end
        set -l value (string split -m 1 \t -- $line)[1]
        if test -n "$value"
            set values $values $value
        end
    end
    if test "$directive" = files
        __fish_complete_path $current
        return
    end
    if test "$directive" = dirs
        __fish_complete_directories $current
        return
    end
    for value in $values
        printf '%s\n' $value
    end
end

complete -c bast -f -a '(__bast_complete)'
`

const powershellCompletionScript = `Register-ArgumentCompleter -Native -CommandName bast, bast.exe -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $quoteChars = [char[]]@([char]34, [char]39)
    $elements = @($commandAst.CommandElements | ForEach-Object { $_.Extent.Text })
    if ($elements.Count -eq 0) {
        return
    }
    $bin = $elements[0].Trim($quoteChars)
    $request = @()
    if ($elements.Count -gt 1) {
        $request = @($elements[1..($elements.Count - 1)] | ForEach-Object { $_.Trim($quoteChars) })
    }
    $text = $commandAst.Extent.Text
    if ($cursorPosition -gt $text.Length) {
        $cursorPosition = $text.Length
    }
    $before = $text.Substring(0, [Math]::Max(0, $cursorPosition))
    if ($before.EndsWith(' ')) {
        $request += ''
    }

    $out = & $bin __complete -- @request 2>$null
    foreach ($line in @($out)) {
        if ($null -eq $line -or $line.StartsWith(':')) {
            continue
        }
        $parts = $line.Split([char]9, 2)
        $value = $parts[0]
        $desc = if ($parts.Count -gt 1 -and $parts[1]) { $parts[1] } else { $value }
        if ($wordToComplete -and ($value.StartsWith($wordToComplete, [System.StringComparison]::OrdinalIgnoreCase) -eq $false)) {
            continue
        }
        [System.Management.Automation.CompletionResult]::new($value, $value, 'ParameterValue', $desc)
    }
}
`

const elvishCompletionScript = `set edit:completion:arg-completer[bast] = {|@args|
    var request = $args[1..]
    try {
        var cmd = (external $args[0])
        var lines = [($cmd __complete -- $@request)]
        for line $lines {
            if (has-prefix $line ':') {
                continue
            }
            put (splits "\t" &max=2 $line | take 1)
        }
    } catch { }
}
`

const nushellCompletionScript = `def nu-complete-bast [context: string] {
    let trailing = ($context | str ends-with ' ')
    let words = ($context | str trim | split row -r '\s+')
    let base = if ($words | length) <= 1 { [""] } else { $words | skip 1 }
    let args = if $trailing { $base | append "" } else { $base }
    try {
        ^bast __complete -- ...$args
        | lines
        | where {|l| not ($l | str starts-with ':')}
        | each {|line|
            let parts = ($line | split row "\t")
            {
                value: ($parts | get 0)
                description: ($parts | get 1? | default '')
            }
        }
    } catch {
        []
    }
}

extern "bast" [
    ...args: string@nu-complete-bast
]
`
