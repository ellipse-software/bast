package cli

import (
	"strings"

	"bast/internal/feature"
)

func helpText() string {
	return filterHelp(help, feature.SyncDisabled(), feature.VaultDisabled())
}

func filterHelp(src string, noSync, noVault bool) string {
	if !noSync && !noVault {
		return src
	}
	syncHeaders := map[string]bool{
		"Sync commands:":         true,
		"Boat commands:":         true,
		"Upstash commands:":      true,
		"Vercel commands:":       true,
		"Hetzner commands:":      true,
		"BitQate commands:":      true,
		"Railway commands:":      true,
		"Fly commands:":          true,
		"DigitalOcean commands:": true,
		"boxd commands:":         true,
	}
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	skipSection := false
	skipRun := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if skipRun {
			if strings.Contains(line, "for details.") {
				skipRun = false
			}
			continue
		}
		if strings.HasPrefix(trim, `Run "`) {
			out = append(out, runHelpParagraph(noSync, noVault))
			if !strings.Contains(line, "for details.") {
				skipRun = true
			}
			continue
		}
		if trim == "" && skipSection {
			skipSection = false
			continue
		}
		if (noSync && syncHeaders[trim]) || (noVault && trim == "Vault commands:") {
			skipSection = true
			continue
		}
		if skipSection {
			continue
		}
		if (noSync && syncUsageLine(trim)) || (noVault && strings.HasPrefix(trim, "bast vault ")) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func syncUsageLine(trim string) bool {
	for _, name := range []string{"sync", "boat", "upstash", "vercel", "hetzner", "bitqate", "railway", "fly", "digitalocean", "boxd"} {
		if strings.HasPrefix(trim, "bast "+name+" ") {
			return true
		}
	}
	return false
}

func runHelpParagraph(noSync, noVault bool) string {
	parts := []string{
		`"bast doctor --help"`,
		`"bast hosts <command> --help"`,
		`"bast keys <command> --help"`,
	}
	if !noSync {
		parts = append(parts,
			`"bast sync <command> --help"`,
			`"bast boat <command> --help"`,
			`"bast upstash <command> --help"`,
			`"bast vercel <command> --help"`,
			`"bast hetzner <command> --help"`,
			`"bast bitqate <command> --help"`,
			`"bast railway <command> --help"`,
			`"bast fly <command> --help"`,
			`"bast digitalocean <command> --help"`,
			`"bast boxd <command> --help"`,
		)
	}
	if !noVault {
		parts = append(parts, `"bast vault <command> --help"`)
	}
	parts = append(parts, `"bast completion --help"`)
	last := parts[len(parts)-1]
	return "Run " + strings.Join(parts[:len(parts)-1], ", ") + ", or " + last + " for details."
}
