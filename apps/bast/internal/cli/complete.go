package cli

import (
	"fmt"
	"io"
	"strings"
)

const completionUsage = `Usage: bast completion <shell>

Print a completion script to stdout. Supported shells: bash, zsh, fish,
powershell, elvish, nushell.

The bast.sh script installers and Homebrew enable completions automatically.
Skip installer hooks with BAST_NO_COMPLETIONS=1. Open a new terminal after
install, then press Tab.

Bash:

  source <(bast completion bash)

Zsh:

  source <(bast completion zsh)

Fish:

  bast completion fish | source

PowerShell:

  bast completion powershell | Out-String | Invoke-Expression

Elvish:

  eval (bast completion elvish | slurp)

Nushell:

  bast completion nushell | save -f ~/.config/nushell/completions/bast.nu
  source ~/.config/nushell/completions/bast.nu

See https://bast.sh/docs/reference/completions
`

func takeCompletionCommand(args []string) (cmd string, rest []string, ok bool) {
	for i, arg := range args {
		switch arg {
		case "--json", "--no-input":
			continue
		case "completion", "__complete":
			return arg, append([]string{}, args[i+1:]...), true
		default:
			return "", nil, false
		}
	}
	return "", nil, false
}

func (r *Runner) completion(args []string) error {
	if len(args) == 0 {
		return usagef("usage: bast completion <shell>")
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.Out, completionUsage)
		return nil
	}
	if len(args) != 1 {
		return usagef("usage: bast completion <shell>")
	}
	shell, ok := normalizeShell(args[0])
	if !ok {
		return usagef("unknown shell %q (bash, zsh, fish, powershell, elvish, nushell)", args[0])
	}
	fmt.Fprint(r.Out, completionScript(shell))
	return nil
}

func (r *Runner) completeQuery(args []string) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	writeComplete(r.Out, r.completeTokens(args))
	return nil
}

func writeComplete(out io.Writer, result completeResult) {
	seen := map[string]bool{}
	for _, c := range result.candidates {
		if c.value == "" || seen[c.value] {
			continue
		}
		seen[c.value] = true
		if c.desc == "" {
			fmt.Fprintln(out, c.value)
			continue
		}
		fmt.Fprintf(out, "%s\t%s\n", c.value, c.desc)
	}
	switch result.directive {
	case directiveFiles:
		fmt.Fprintln(out, ":files")
	case directiveDirs:
		fmt.Fprintln(out, ":dirs")
	default:
		fmt.Fprintln(out, ":nofiles")
	}
}

func normalizeShell(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "bash":
		return "bash", true
	case "zsh":
		return "zsh", true
	case "fish":
		return "fish", true
	case "powershell", "pwsh":
		return "powershell", true
	case "elvish":
		return "elvish", true
	case "nushell", "nu":
		return "nushell", true
	default:
		return "", false
	}
}
