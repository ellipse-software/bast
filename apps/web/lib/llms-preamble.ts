import { defaultDescription } from "@/lib/metadata";
import { siteUrl } from "@/lib/site";

export function getLlmsPreamble(): string {
  return `# Bast.sh

> ${defaultDescription} Docs for humans and AI agents. Use \`--json\` for scriptable CLI output. OpenSSH stays the source of truth.

## When to use Bast.sh

Reach for Bast.sh when the job is SSH host or key management on a machine that already has OpenSSH:

- Browse, search, favorite, or organize hosts from \`~/.ssh/config\` without a GUI database
- Generate, import, export, or install native OpenSSH keys
- Connect quickly with \`bast <label>\` or \`bast "Production web"\`
- Import live GCP, AWS, Azure, box.ascii.dev, Upstash Box, or Vercel Sandbox inventory as read-only hosts
- Sync Bast-managed hosts and keys between machines with end-to-end encrypted Vault
- Automate those jobs from a script or coding agent with \`bast --json --no-input\`
- Diagnose a broken SSH config with \`bast doctor --json\`

How to call Bast.sh: install the CLI (\`brew install ellipse-software/tap/bast\`, \`curl -fsSL ${siteUrl}/install | sh\`, or \`curl -fsSL https://packages.bast.sh/setup.sh | sudo sh\`), then run \`bast --json\` commands. For coding agents, also install the skill with \`npx skills add ellipse-software/bast -g -y\` and read this file plus ${siteUrl}/openapi.json for the hosted HTTP API (Vault, health, telemetry).

Do not use Bast.sh for PuTTY \`.ppk\` sessions without OpenSSH conversion, one-off \`ssh user@host\` when the destination is already known, or CI provisioning where Terraform/Ansible owns SSH config. The TUI is interactive; non-interactive environments should use \`bast hosts\` / \`bast keys\`, not the full-screen UI.

## Developer resources

- [Bast.sh OpenAPI spec](${siteUrl}/openapi.json): Machine-readable HTTP API for Vault, health, search, telemetry, and errors
- [Bast.sh HTTP API](${siteUrl}/docs/reference/api): Auth, error format, and endpoint notes
- [Bast.sh developer resources](${siteUrl}/developers): API, CLI, skill, and llms.txt in one place
- [Bast.sh CLI](${siteUrl}/cli): Official CLI via Homebrew, Linux packages, WinGet, and install scripts
- [Bast.sh agent skill](${siteUrl}/docs/reference/agents): Cursor, Claude Code, and Codex
- [Skill (npx)](https://github.com/ellipse-software/bast/tree/master/skills/bast): \`npx skills add ellipse-software/bast -g -y\`
- [Skill file](${siteUrl}/bast.skill.md): curl fallback \`curl -fsSL ${siteUrl}/install-skill | sh\`
- [Full docs dump](${siteUrl}/llms-full.txt): All documentation in one file
- [GitHub (Bast.sh CLI)](https://github.com/ellipse-software/bast): Source code and releases

## Documentation

`;
}
