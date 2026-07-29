package cli

import (
	"flag"
	"sort"
	"strings"

	"bast/internal/sshconfig"
)

type hostAdvancedAddFlags struct {
	forwardAgent   *string
	startupCommand *string
	requestTTY     *string
	dynamicForward *string
	compression    *string
	keepalive      *string
	setEnv         stringsFlag
	localForward   stringsFlag
	remoteForward  stringsFlag
	customOptions  stringsFlag
}

func (f *hostAdvancedAddFlags) register(fs *flag.FlagSet) {
	f.forwardAgent = fs.String("forward-agent", "", "agent forwarding: yes or no")
	f.startupCommand = fs.String("startup-command", "", "command to run after connect (RemoteCommand)")
	f.requestTTY = fs.String("request-tty", "", "TTY allocation: force or no")
	f.dynamicForward = fs.String("dynamic-forward", "", "SOCKS port (DynamicForward)")
	f.compression = fs.String("compression", "", "compression: yes or no")
	f.keepalive = fs.String("keepalive", "", "ServerAliveInterval in seconds")
	fs.Var(&f.setEnv, "set-env", "environment variable KEY=VALUE (repeatable)")
	fs.Var(&f.localForward, "local-forward", "local forward as port target (repeatable)")
	fs.Var(&f.remoteForward, "remote-forward", "remote forward as port target (repeatable)")
	fs.Var(&f.customOptions, "ssh-option", "extra OpenSSH option (repeatable)")
}

func (f *hostAdvancedAddFlags) settings(proxyJump string, customFromSSHOption []string) (sshconfig.AdvancedSettings, error) {
	custom := append([]string(nil), f.customOptions...)
	if len(customFromSSHOption) > 0 {
		custom = append(custom, customFromSSHOption...)
	}
	settings := sshconfig.AdvancedSettings{
		ProxyJump:           strings.TrimSpace(proxyJump),
		ForwardAgent:        normalizeTriState(*f.forwardAgent),
		RemoteCommand:       strings.TrimSpace(*f.startupCommand),
		RequestTTY:          normalizeRequestTTY(*f.requestTTY),
		SetEnv:              splitSemicolonValues(f.setEnv),
		LocalForwards:       splitSemicolonValues(f.localForward),
		RemoteForwards:      splitSemicolonValues(f.remoteForward),
		DynamicForward:      strings.TrimSpace(*f.dynamicForward),
		ServerAliveInterval: strings.TrimSpace(*f.keepalive),
		Compression:         normalizeTriState(*f.compression),
		Custom:              sshconfig.ParseSSHFlags(strings.Join(custom, "; ")),
	}
	return settings, validateAdvancedFlagValues(settings)
}

type hostAdvancedEditFlags struct {
	forwardAgent   optionalString
	startupCommand optionalString
	requestTTY     optionalString
	dynamicForward optionalString
	compression    optionalString
	keepalive      optionalString
	setEnv         stringsFlag
	localForward   stringsFlag
	remoteForward  stringsFlag
	customOptions  stringsFlag

	clearForwardAgent   bool
	clearStartupCommand bool
	clearRequestTTY     bool
	clearDynamicForward bool
	clearCompression    bool
	clearKeepalive      bool
	clearSetEnv         bool
	clearLocalForward   bool
	clearRemoteForward  bool
	clearCustomOptions  bool
}

func (f *hostAdvancedEditFlags) register(fs *flag.FlagSet) {
	fs.Var(&f.forwardAgent, "forward-agent", "agent forwarding: yes or no")
	fs.Var(&f.startupCommand, "startup-command", "command to run after connect (RemoteCommand)")
	fs.Var(&f.requestTTY, "request-tty", "TTY allocation: force or no")
	fs.Var(&f.dynamicForward, "dynamic-forward", "SOCKS port (DynamicForward)")
	fs.Var(&f.compression, "compression", "compression: yes or no")
	fs.Var(&f.keepalive, "keepalive", "ServerAliveInterval in seconds")
	fs.Var(&f.setEnv, "set-env", "replacement environment variable KEY=VALUE (repeatable)")
	fs.Var(&f.localForward, "local-forward", "replacement local forward port target (repeatable)")
	fs.Var(&f.remoteForward, "remote-forward", "replacement remote forward port target (repeatable)")
	fs.Var(&f.customOptions, "ssh-option", "replacement extra OpenSSH option (repeatable)")
	fs.BoolVar(&f.clearForwardAgent, "clear-forward-agent", false, "use OpenSSH default agent forwarding")
	fs.BoolVar(&f.clearStartupCommand, "clear-startup-command", false, "clear startup command")
	fs.BoolVar(&f.clearRequestTTY, "clear-request-tty", false, "use OpenSSH default TTY allocation")
	fs.BoolVar(&f.clearDynamicForward, "clear-dynamic-forward", false, "clear dynamic forward")
	fs.BoolVar(&f.clearCompression, "clear-compression", false, "use OpenSSH default compression")
	fs.BoolVar(&f.clearKeepalive, "clear-keepalive", false, "clear keepalive interval")
	fs.BoolVar(&f.clearSetEnv, "clear-set-env", false, "clear environment variables")
	fs.BoolVar(&f.clearLocalForward, "clear-local-forward", false, "clear local forwards")
	fs.BoolVar(&f.clearRemoteForward, "clear-remote-forward", false, "clear remote forwards")
	fs.BoolVar(&f.clearCustomOptions, "clear-ssh-options", false, "clear custom OpenSSH options")
}

func (f *hostAdvancedEditFlags) conflicts() error {
	for _, conflict := range []struct {
		set, clear bool
		name       string
	}{
		{f.forwardAgent.set, f.clearForwardAgent, "forward-agent"},
		{f.startupCommand.set, f.clearStartupCommand, "startup-command"},
		{f.requestTTY.set, f.clearRequestTTY, "request-tty"},
		{f.dynamicForward.set, f.clearDynamicForward, "dynamic-forward"},
		{f.compression.set, f.clearCompression, "compression"},
		{f.keepalive.set, f.clearKeepalive, "keepalive"},
		{len(f.setEnv) > 0, f.clearSetEnv, "set-env"},
		{len(f.localForward) > 0, f.clearLocalForward, "local-forward"},
		{len(f.remoteForward) > 0, f.clearRemoteForward, "remote-forward"},
		{len(f.customOptions) > 0, f.clearCustomOptions, "ssh-options"},
	} {
		if conflict.set && conflict.clear {
			return usagef("cannot set and clear %s in the same command", conflict.name)
		}
	}
	return nil
}

func (f *hostAdvancedEditFlags) changed() bool {
	return f.forwardAgent.set || f.startupCommand.set || f.requestTTY.set || f.dynamicForward.set ||
		f.compression.set || f.keepalive.set || len(f.setEnv) > 0 || len(f.localForward) > 0 ||
		len(f.remoteForward) > 0 || len(f.customOptions) > 0 || f.clearForwardAgent ||
		f.clearStartupCommand || f.clearRequestTTY || f.clearDynamicForward || f.clearCompression ||
		f.clearKeepalive || f.clearSetEnv || f.clearLocalForward || f.clearRemoteForward || f.clearCustomOptions
}

func (f *hostAdvancedEditFlags) apply(current sshconfig.AdvancedSettings, proxyJump string, proxySet, clearProxy bool) (sshconfig.AdvancedSettings, error) {
	next := current
	if proxySet {
		next.ProxyJump = strings.TrimSpace(proxyJump)
	}
	if clearProxy {
		next.ProxyJump = ""
	}
	if f.forwardAgent.set {
		next.ForwardAgent = normalizeTriState(f.forwardAgent.value)
	}
	if f.clearForwardAgent {
		next.ForwardAgent = ""
	}
	if f.startupCommand.set {
		next.RemoteCommand = strings.TrimSpace(f.startupCommand.value)
	}
	if f.clearStartupCommand {
		next.RemoteCommand = ""
	}
	if f.requestTTY.set {
		next.RequestTTY = normalizeRequestTTY(f.requestTTY.value)
	}
	if f.clearRequestTTY {
		next.RequestTTY = ""
	}
	if f.dynamicForward.set {
		next.DynamicForward = strings.TrimSpace(f.dynamicForward.value)
	}
	if f.clearDynamicForward {
		next.DynamicForward = ""
	}
	if f.compression.set {
		next.Compression = normalizeTriState(f.compression.value)
	}
	if f.clearCompression {
		next.Compression = ""
	}
	if f.keepalive.set {
		next.ServerAliveInterval = strings.TrimSpace(f.keepalive.value)
	}
	if f.clearKeepalive {
		next.ServerAliveInterval = ""
	}
	if len(f.setEnv) > 0 {
		next.SetEnv = splitSemicolonValues(f.setEnv)
	}
	if f.clearSetEnv {
		next.SetEnv = nil
	}
	if len(f.localForward) > 0 {
		next.LocalForwards = splitSemicolonValues(f.localForward)
	}
	if f.clearLocalForward {
		next.LocalForwards = nil
	}
	if len(f.remoteForward) > 0 {
		next.RemoteForwards = splitSemicolonValues(f.remoteForward)
	}
	if f.clearRemoteForward {
		next.RemoteForwards = nil
	}
	if len(f.customOptions) > 0 {
		next.Custom = sshconfig.ParseSSHFlags(strings.Join(f.customOptions, "; "))
	}
	if f.clearCustomOptions {
		next.Custom = nil
	}
	return next, validateAdvancedFlagValues(next)
}

func hostInputFromAdvanced(input sshconfig.HostInput, adv sshconfig.AdvancedSettings) sshconfig.HostInput {
	input.ProxyJump = adv.ProxyJump
	input.ExtraOptions = adv.ExtraOptions()
	if input.IdentityFile != "" && !input.PasswordOnly && !sshconfig.HasDirective(input.ExtraOptions, "IdentitiesOnly") {
		input.ExtraOptions = append([]string{"IdentitiesOnly yes"}, input.ExtraOptions...)
	}
	return input
}

func loadHostAdvanced(m sshconfig.Manager, host sshconfig.Host, resolvedProxyJump string) (sshconfig.AdvancedSettings, error) {
	extras := []string(nil)
	if host.Managed && host.ManagedID != "" {
		var err error
		extras, err = m.ManagedExtras(host.ManagedID)
		if err != nil {
			return sshconfig.AdvancedSettings{}, err
		}
	}
	proxyJump := strings.TrimSpace(resolvedProxyJump)
	if proxyJump == "none" {
		proxyJump = ""
	}
	return sshconfig.ParseAdvanced(extras, proxyJump), nil
}

func normalizeTriState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "default":
		return ""
	case "yes", "true", "on":
		return "yes"
	case "no", "false", "off":
		return "no"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeRequestTTY(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "default", "auto":
		return ""
	case "force", "yes", "true":
		return "force"
	case "no", "false", "disable", "disabled":
		return "no"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func splitSemicolonValues(values []string) []string {
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ";") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func validateAdvancedFlagValues(settings sshconfig.AdvancedSettings) error {
	if err := sshconfig.ValidateAdvanced(settings); err != nil {
		return fail("validation", err.Error())
	}
	for _, value := range []struct {
		name, raw string
		allowed   map[string]bool
	}{
		{"forward-agent", settings.ForwardAgent, map[string]bool{"": true, "yes": true, "no": true}},
		{"compression", settings.Compression, map[string]bool{"": true, "yes": true, "no": true}},
		{"request-tty", settings.RequestTTY, map[string]bool{"": true, "force": true, "no": true}},
	} {
		if !value.allowed[value.raw] {
			return usagef("%s must be one of: %s", value.name, strings.Join(allowedList(value.allowed), ", "))
		}
	}
	return nil
}

func allowedList(values map[string]bool) []string {
	out := []string{}
	for value := range values {
		if value == "" {
			out = append(out, "default")
			continue
		}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func promptAdvancedSettings(r *Runner, current sshconfig.AdvancedSettings) (sshconfig.AdvancedSettings, error) {
	next := current
	var err error
	next.ProxyJump, err = r.prompt("Proxy jump", current.ProxyJump, false)
	if err != nil {
		return sshconfig.AdvancedSettings{}, err
	}
	next.ForwardAgent, err = r.prompt("Agent forwarding (default, yes, no)", displayTriState(current.ForwardAgent), false)
	if err != nil {
		return sshconfig.AdvancedSettings{}, err
	}
	next.ForwardAgent = normalizeTriState(next.ForwardAgent)
	next.RemoteCommand, err = r.prompt("Startup command", current.RemoteCommand, false)
	if err != nil {
		return sshconfig.AdvancedSettings{}, err
	}
	next.RequestTTY, err = r.prompt("TTY allocation (default, force, no)", displayTriState(current.RequestTTY), false)
	if err != nil {
		return sshconfig.AdvancedSettings{}, err
	}
	next.RequestTTY = normalizeRequestTTY(next.RequestTTY)
	localText, err := r.prompt("Local forwards (; separated)", sshconfig.FormatForwardList(current.LocalForwards), false)
	if err != nil {
		return sshconfig.AdvancedSettings{}, err
	}
	next.LocalForwards = sshconfig.ParseForwardList(localText)
	remoteText, err := r.prompt("Remote forwards (; separated)", sshconfig.FormatForwardList(current.RemoteForwards), false)
	if err != nil {
		return sshconfig.AdvancedSettings{}, err
	}
	next.RemoteForwards = sshconfig.ParseForwardList(remoteText)
	next.DynamicForward, err = r.prompt("Dynamic forward", current.DynamicForward, false)
	if err != nil {
		return sshconfig.AdvancedSettings{}, err
	}
	next.Compression, err = r.prompt("Compression (default, yes, no)", displayTriState(current.Compression), false)
	if err != nil {
		return sshconfig.AdvancedSettings{}, err
	}
	next.Compression = normalizeTriState(next.Compression)
	next.ServerAliveInterval, err = r.prompt("Keepalive (seconds)", current.ServerAliveInterval, false)
	if err != nil {
		return sshconfig.AdvancedSettings{}, err
	}
	envText, err := r.prompt("Environment variables (; separated)", sshconfig.FormatSetEnvList(current.SetEnv), false)
	if err != nil {
		return sshconfig.AdvancedSettings{}, err
	}
	next.SetEnv = sshconfig.ParseSetEnvList(envText)
	customText, err := r.prompt("Custom SSH options (; separated)", sshconfig.FormatSSHFlags(current.Custom), false)
	if err != nil {
		return sshconfig.AdvancedSettings{}, err
	}
	next.Custom = sshconfig.ParseSSHFlags(customText)
	return next, validateAdvancedFlagValues(next)
}

func displayTriState(value string) string {
	if value == "" {
		return "default"
	}
	return value
}

func formatAdvancedSummary(adv sshconfig.AdvancedSettings) string {
	parts := []string{}
	if adv.ProxyJump != "" {
		parts = append(parts, "ProxyJump="+adv.ProxyJump)
	}
	if adv.ForwardAgent != "" {
		parts = append(parts, "ForwardAgent="+adv.ForwardAgent)
	}
	if adv.RemoteCommand != "" {
		parts = append(parts, "RemoteCommand="+adv.RemoteCommand)
	}
	if adv.RequestTTY != "" {
		parts = append(parts, "RequestTTY="+adv.RequestTTY)
	}
	if len(adv.SetEnv) > 0 {
		parts = append(parts, "SetEnv="+sshconfig.FormatSetEnvList(adv.SetEnv))
	}
	if len(adv.LocalForwards) > 0 {
		parts = append(parts, "LocalForward="+sshconfig.FormatForwardList(adv.LocalForwards))
	}
	if len(adv.RemoteForwards) > 0 {
		parts = append(parts, "RemoteForward="+sshconfig.FormatForwardList(adv.RemoteForwards))
	}
	if adv.DynamicForward != "" {
		parts = append(parts, "DynamicForward="+adv.DynamicForward)
	}
	if adv.ServerAliveInterval != "" {
		parts = append(parts, "ServerAliveInterval="+adv.ServerAliveInterval)
	}
	if adv.Compression != "" {
		parts = append(parts, "Compression="+adv.Compression)
	}
	if len(adv.Custom) > 0 {
		parts = append(parts, "Options="+sshconfig.FormatSSHFlags(adv.Custom))
	}
	return strings.Join(parts, "\n")
}
