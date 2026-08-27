package cli

type valueKind int

const (
	valueFree valueKind = iota
	valueHost
	valueKey
	valueBoxHost
	valueUpstashHost
	valueVercelHost
	valueHetznerHost
	valueFile
	valueDir
	valueEnum
	valueProvider
	valueShell
)

type directive int

const (
	directiveNoFiles directive = iota
	directiveFiles
	directiveDirs
)

type flagSpec struct {
	name       string
	desc       string
	kind       valueKind
	enum       []string
	boolean    bool
	repeatable bool
}

type argSpec struct {
	kind          valueKind
	desc          string
	optional      bool
	includeHidden bool
	enum          []string
}

type specNode struct {
	name     string
	aliases  []string
	desc     string
	flags    []flagSpec
	args     []argSpec
	children []specNode
}

func (n specNode) names() []string {
	out := make([]string, 0, 1+len(n.aliases))
	out = append(out, n.name)
	out = append(out, n.aliases...)
	return out
}

var completionShells = []string{"bash", "zsh", "fish", "powershell", "elvish", "nushell"}

var syncProviders = []string{"gcp", "aws", "azure", "box", "upstash", "vercel", "hetzner"}

var globalFlagSpecs = []flagSpec{
	{name: "json", desc: "structured JSON", boolean: true},
	{name: "no-input", desc: "never prompt", boolean: true},
	{name: "help", desc: "help", boolean: true},
	{name: "h", desc: "help", boolean: true},
}

func completionRoot() specNode {
	return specNode{
		desc: "bast",
		flags: []flagSpec{
			{name: "version", desc: "print version", boolean: true},
		},
		args: []argSpec{{kind: valueHost, desc: "host", optional: true}},
		children: []specNode{
			{name: "tui", desc: "open the TUI"},
			{name: "update", desc: "update installer-managed copies"},
			{name: "connect", desc: "connect to a host", args: []argSpec{{kind: valueHost, desc: "host"}}},
			hostsSpec(),
			keysSpec(),
			syncSpec(),
			boxSpec(),
			upstashSpec(),
			vercelSpec(),
			hetznerSpec(),
			vaultSpec(),
			{
				name: "completion",
				desc: "print a shell completion script",
				args: []argSpec{{kind: valueShell, desc: "shell"}},
			},
		},
	}
}

func hostsSpec() specNode {
	hostArg := argSpec{kind: valueHost, desc: "host"}
	hiddenHostArg := argSpec{kind: valueHost, desc: "host", includeHidden: true}
	return specNode{
		name: "hosts",
		desc: "manage SSH hosts",
		children: []specNode{
			{
				name: "list",
				desc: "list hosts",
				flags: []flagSpec{
					{name: "search", desc: "filter hosts"},
					enumFlag("sort", "sort order", "smart", "label", "recent", "group"),
					boolFlag("all", "include hidden hosts"),
					boolFlag("hidden", "include hidden hosts"),
				},
			},
			{name: "show", desc: "show a host", args: []argSpec{hostArg}},
			{name: "add", desc: "add a host", flags: hostAddFlags(), args: []argSpec{{desc: "label", optional: true}}},
			{name: "edit", desc: "edit a host", flags: hostEditFlags(), args: []argSpec{hostArg}},
			{name: "delete", desc: "delete a host", flags: []flagSpec{boolFlag("yes", "skip confirmation")}, args: []argSpec{hostArg}},
			{name: "promote", desc: "copy an external host into Bast", args: []argSpec{hostArg}},
			{name: "favorite", desc: "mark a host as favorite", args: []argSpec{hostArg}},
			{name: "unfavorite", desc: "remove favorite", args: []argSpec{hostArg}},
			{name: "hide", desc: "hide a host", args: []argSpec{hostArg}},
			{name: "show-hidden", aliases: []string{"unhide"}, desc: "unhide a host", args: []argSpec{hiddenHostArg}},
			{
				name: "sort",
				desc: "set default host sort",
				args: []argSpec{{kind: valueEnum, desc: "order", enum: []string{"smart", "label", "recent", "group"}}},
			},
			{
				name: "known-host",
				desc: "known_hosts entries",
				children: []specNode{{
					name:  "remove",
					desc:  "remove a known-host entry",
					flags: []flagSpec{boolFlag("yes", "skip confirmation")},
					args:  []argSpec{hostArg},
				}},
			},
		},
	}
}

func keysSpec() specNode {
	keyArg := argSpec{kind: valueKey, desc: "key"}
	return specNode{
		name: "keys",
		desc: "manage SSH keys",
		children: []specNode{
			{name: "list", desc: "list keys", flags: []flagSpec{{name: "search", desc: "filter keys"}}},
			{name: "show", desc: "show a key", args: []argSpec{keyArg}},
			{
				name: "generate",
				desc: "generate a key",
				flags: []flagSpec{
					enumFlag("algorithm", "key algorithm", "ed25519", "rsa"),
					boolFlag("no-passphrase", "create without a passphrase"),
				},
				args: []argSpec{{desc: "name", optional: true}},
			},
			{
				name: "import",
				desc: "import a key",
				flags: []flagSpec{
					fileFlag("private", "private key path"),
					fileFlag("public", "public key path"),
					{name: "comment", desc: "public-key comment"},
				},
				args: []argSpec{{desc: "name", optional: true}},
			},
			{name: "promote", desc: "copy a key into Bast", flags: []flagSpec{{name: "name", desc: "managed key name"}}, args: []argSpec{keyArg}},
			{
				name: "comment",
				desc: "set a key comment",
				flags: []flagSpec{
					{name: "comment", desc: "new comment"},
					boolFlag("clear-comment", "remove the comment"),
				},
				args: []argSpec{keyArg},
			},
			{
				name: "export",
				desc: "export a key",
				flags: []flagSpec{
					dirFlag("directory", "destination directory"),
					boolFlag("yes", "acknowledge private-key export"),
				},
				args: []argSpec{keyArg},
			},
			{
				name:  "install",
				desc:  "install a public key on a host",
				flags: []flagSpec{{name: "host", desc: "target host", kind: valueHost}},
				args:  []argSpec{keyArg},
			},
			{name: "passphrase", desc: "change a key passphrase", args: []argSpec{keyArg}},
			{name: "public", desc: "print a public key", args: []argSpec{keyArg}},
			{name: "copy", desc: "copy a public key", args: []argSpec{keyArg}},
			{name: "delete", desc: "delete a key", flags: []flagSpec{boolFlag("yes", "skip confirmation")}, args: []argSpec{keyArg}},
		},
	}
}

func syncSpec() specNode {
	return specNode{
		name: "sync",
		desc: "sync cloud VMs into Bast",
		children: []specNode{
			{name: "gcp", desc: "import GCP VMs"},
			{name: "aws", desc: "import Amazon EC2 instances"},
			{name: "azure", desc: "import Azure Linux VMs"},
			{name: "box", desc: "import box.ascii.dev hosts"},
			{name: "upstash", desc: "import Upstash Box hosts"},
			{name: "vercel", desc: "import Vercel Sandboxes"},
			{name: "hetzner", desc: "import Hetzner Cloud servers"},
			{name: "status", desc: "show sync status"},
			{
				name: "disable",
				desc: "disconnect a provider",
				args: []argSpec{{kind: valueProvider, desc: "provider"}},
			},
		},
	}
}

func boxSpec() specNode {
	boxArg := argSpec{kind: valueBoxHost, desc: "host or id", includeHidden: true}
	typeFlag := enumFlag("type", "machine size", "small", "default", "large")
	return specNode{
		name: "box",
		desc: "create and manage ASCII Box sandboxes",
		children: []specNode{
			{
				name: "new",
				desc: "create a box",
				flags: []flagSpec{
					typeFlag,
					{name: "ttl", desc: "auto-stop TTL in seconds"},
					boolFlag("no-auto-stop", "disable automatic stop"),
					boolFlag("no-env", "create a no-env box"),
				},
			},
			{name: "fork", desc: "fork a box", flags: []flagSpec{typeFlag, boolFlag("no-env", "fork as no-env")}, args: []argSpec{boxArg}},
			{name: "stop", desc: "stop a box", args: []argSpec{boxArg}},
			{name: "resume", desc: "resume a box", flags: []flagSpec{typeFlag, boolFlag("no-env", "resume as no-env")}, args: []argSpec{boxArg}},
		},
	}
}

func upstashSpec() specNode {
	upstashArg := argSpec{kind: valueUpstashHost, desc: "host or id", includeHidden: true}
	return specNode{
		name: "upstash",
		desc: "create and manage Upstash Box sandboxes",
		children: []specNode{
			{
				name: "new",
				desc: "create an Upstash box",
				flags: []flagSpec{
					{name: "name", desc: "box name"},
					enumFlag("runtime", "runtime", "node", "python", "golang", "ruby", "rust", "node-alpine", "python-alpine", "golang-alpine", "ruby-alpine", "rust-alpine"),
					enumFlag("size", "size", "small", "medium", "large"),
					boolFlag("keep-alive", "keep the box on between sessions"),
				},
			},
			{name: "fork", desc: "fork an Upstash box", args: []argSpec{upstashArg}},
			{name: "stop", desc: "pause an Upstash box", args: []argSpec{upstashArg}},
			{name: "resume", desc: "resume an Upstash box", args: []argSpec{upstashArg}},
			{name: "delete", desc: "delete an Upstash box", flags: []flagSpec{boolFlag("yes", "skip confirmation")}, args: []argSpec{upstashArg}},
			{name: "key", desc: "store an Upstash Box API key", flags: []flagSpec{fileFlag("key-file", "read the API key from a file")}},
		},
	}
}

func vercelSpec() specNode {
	vercelArg := argSpec{kind: valueVercelHost, desc: "host or id", includeHidden: true}
	return specNode{
		name: "vercel",
		desc: "create and manage Vercel Sandboxes",
		children: []specNode{
			{
				name: "new",
				desc: "create a sandbox",
				flags: []flagSpec{
					{name: "name", desc: "sandbox name"},
					enumFlag("vcpus", "vCPUs", "1", "2", "4"),
					enumFlag("timeout", "session timeout", "15m", "1h", "5h"),
					boolFlag("ephemeral", "disable filesystem persistence"),
				},
			},
			{name: "fork", desc: "fork a sandbox", flags: []flagSpec{{name: "name", desc: "fork name"}}, args: []argSpec{vercelArg}},
			{name: "stop", desc: "stop a sandbox", args: []argSpec{vercelArg}},
			{name: "resume", desc: "resume a sandbox", args: []argSpec{vercelArg}},
			{name: "delete", desc: "delete a sandbox", flags: []flagSpec{boolFlag("yes", "skip confirmation")}, args: []argSpec{vercelArg}},
			{name: "cleanup", desc: "delete unrestorable sandboxes", flags: []flagSpec{boolFlag("yes", "skip confirmation")}},
			{name: "token", desc: "store a Vercel access token", flags: []flagSpec{
				fileFlag("token-file", "read the access token from a file"),
				{name: "team", desc: "team ID"},
				{name: "project", desc: "project ID or name"},
			}},
		},
	}
}

func hetznerSpec() specNode {
	hetznerArg := argSpec{kind: valueHetznerHost, desc: "host or id", includeHidden: true}
	return specNode{
		name: "hetzner",
		desc: "start, stop, and restart Hetzner Cloud servers",
		children: []specNode{
			{name: "start", desc: "power on a server", args: []argSpec{hetznerArg}},
			{name: "stop", desc: "ACPI shutdown a server", flags: []flagSpec{boolFlag("force", "hard poweroff")}, args: []argSpec{hetznerArg}},
			{name: "restart", desc: "ACPI reboot a server", flags: []flagSpec{boolFlag("force", "hard reset")}, args: []argSpec{hetznerArg}},
			{name: "key", desc: "store a Hetzner Cloud API token", flags: []flagSpec{
				{name: "name", desc: "project name"},
				fileFlag("key-file", "read the API token from a file"),
				{name: "remove", desc: "delete a stored project token"},
			}},
		},
	}
}

func vaultSpec() specNode {
	mode := enumFlag("mode", "merge mode", "merge", "replace_local", "replace_remote")
	return specNode{
		name: "vault",
		desc: "sync Bast-managed config via encrypted vault",
		children: []specNode{
			{
				name: "login",
				desc: "link this machine to Vault",
				flags: []flagSpec{
					{name: "email", desc: "account email"},
					{name: "api", desc: "vault API base URL"},
					mode,
					boolFlag("accept-terms", "accept Terms of Service and Privacy Policy"),
				},
			},
			{name: "status", desc: "show vault status"},
			{name: "push", desc: "push local state"},
			{name: "pull", desc: "pull remote state", flags: []flagSpec{mode}},
			{name: "logout", desc: "unlink this machine"},
			{name: "passphrase", desc: "change vault passphrase", flags: []flagSpec{boolFlag("force", "overwrite remote under a new passphrase")}},
		},
	}
}

func hostAddFlags() []flagSpec {
	return []flagSpec{
		{name: "hostname", desc: "server hostname or IP"},
		{name: "user", desc: "remote user"},
		{name: "port", desc: "remote port"},
		fileFlag("identity", "identity file"),
		boolFlag("password-only", "disable public-key authentication"),
		{name: "proxy-jump", desc: "jump host", kind: valueHost},
		{name: "group", desc: "group path"},
		{name: "environment", desc: "environment"},
		{name: "color", desc: "label colour"},
		{name: "notes", desc: "notes"},
		{name: "tag", desc: "tag", repeatable: true},
		enumFlag("forward-agent", "agent forwarding", "yes", "no"),
		{name: "startup-command", desc: "command to run after connect"},
		enumFlag("request-tty", "TTY allocation", "force", "no"),
		{name: "dynamic-forward", desc: "SOCKS port"},
		enumFlag("compression", "compression", "yes", "no"),
		{name: "keepalive", desc: "ServerAliveInterval in seconds"},
		{name: "set-env", desc: "environment variable KEY=VALUE", repeatable: true},
		{name: "local-forward", desc: "local forward", repeatable: true},
		{name: "remote-forward", desc: "remote forward", repeatable: true},
		{name: "ssh-option", desc: "extra OpenSSH option", repeatable: true},
	}
}

func hostEditFlags() []flagSpec {
	flags := []flagSpec{{name: "label", desc: "display label"}}
	flags = append(flags, hostAddFlags()...)
	flags = append(flags,
		boolFlag("clear-user", "clear remote user"),
		boolFlag("clear-port", "clear port"),
		boolFlag("clear-identity", "use OpenSSH defaults"),
		boolFlag("clear-proxy-jump", "clear jump host"),
		boolFlag("clear-group", "clear group"),
		boolFlag("clear-tags", "clear tags"),
		boolFlag("clear-environment", "clear environment"),
		boolFlag("clear-color", "clear colour"),
		boolFlag("clear-notes", "clear notes"),
		boolFlag("clear-forward-agent", "use OpenSSH default agent forwarding"),
		boolFlag("clear-startup-command", "clear startup command"),
		boolFlag("clear-request-tty", "use OpenSSH default TTY allocation"),
		boolFlag("clear-dynamic-forward", "clear dynamic forward"),
		boolFlag("clear-compression", "use OpenSSH default compression"),
		boolFlag("clear-keepalive", "clear keepalive interval"),
		boolFlag("clear-set-env", "clear environment variables"),
		boolFlag("clear-local-forward", "clear local forwards"),
		boolFlag("clear-remote-forward", "clear remote forwards"),
		boolFlag("clear-ssh-options", "clear custom OpenSSH options"),
	)
	return flags
}

func boolFlag(name, desc string) flagSpec {
	return flagSpec{name: name, desc: desc, boolean: true}
}

func enumFlag(name, desc string, values ...string) flagSpec {
	return flagSpec{name: name, desc: desc, kind: valueEnum, enum: values}
}

func fileFlag(name, desc string) flagSpec {
	return flagSpec{name: name, desc: desc, kind: valueFile}
}

func dirFlag(name, desc string) flagSpec {
	return flagSpec{name: name, desc: desc, kind: valueDir}
}
