package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"bast/internal/cloud/sync"
	"bast/internal/connectbanner"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
	"bast/internal/telemetry"
)

func (r *Runner) hosts(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast hosts <list|show|add|edit|delete|favorite|unfavorite|hide|show-hidden|sort|known-host>")
		return nil
	}
	switch args[0] {
	case "list":
		return r.hostList(args[1:])
	case "show":
		return r.hostShow(args[1:])
	case "add":
		return r.hostAdd(args[1:])
	case "edit":
		return r.hostEdit(args[1:])
	case "delete":
		return r.hostDelete(args[1:])
	case "favorite":
		return r.hostState(args[1:], "favorite")
	case "unfavorite":
		return r.hostState(args[1:], "unfavorite")
	case "hide":
		return r.hostState(args[1:], "hide")
	case "show-hidden", "unhide":
		return r.hostState(args[1:], "show")
	case "sort":
		return r.hostSort(args[1:])
	case "known-host":
		return r.knownHost(args[1:])
	default:
		return usagef("unknown hosts command %q", args[0])
	}
}

func (r *Runner) hostList(args []string) error {
	fs := newFlagSet("hosts list")
	search := fs.String("search", "", "filter hosts")
	order := fs.String("sort", "", "sort order")
	all := fs.Bool("all", false, "include hidden hosts")
	fs.BoolVar(all, "hidden", false, "include hidden hosts")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast hosts list [--search text] [--sort order] [--all]")
	}
	hosts, _, err := r.snapshot()
	if err != nil {
		return err
	}
	filtered := hosts[:0]
	q := strings.ToLower(*search)
	for _, host := range hosts {
		if host.Hidden && !*all {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{host.Alias, host.Label, host.Hostname, host.User, host.Group, strings.Join(host.Tags, " "), host.Environment, host.Notes}, " "))
		if q == "" || strings.Contains(haystack, q) {
			filtered = append(filtered, host)
		}
	}
	if *order == "" {
		*order = r.store.Preferences().Sort
	}
	if err := sortHosts(filtered, *order); err != nil {
		return err
	}
	if r.JSON {
		return r.success(filtered, "")
	}
	printHostTable(r.Out, filtered)
	return nil
}

func (r *Runner) hostShow(args []string) error {
	fs := newFlagSet("hosts show")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast hosts show <host>")
	}
	hosts, _, err := r.snapshot()
	if err != nil {
		return err
	}
	host, err := r.findHost(fs.Arg(0), hosts)
	if err != nil {
		return err
	}
	if r.JSON {
		return r.success(host, "")
	}
	fmt.Fprintf(r.Out, "Alias: %s\nLabel: %s\nDestination: %s@%s:%s\nAuthentication: %s\nProxy jump: %s\nGroup: %s\nTags: %s\nEnvironment: %s\nNotes: %s\nManaged: %t\nSource: %s\nKnown host: %t\nFavorite: %t\nHidden: %t\nConnections: %d\n",
		host.Alias, host.Label, host.User, host.Hostname, host.Port, host.Authentication, emptyDefault(host.ProxyJump), host.Group, strings.Join(host.Tags, ", "), host.Environment, host.Notes, host.Managed, host.Source, host.KnownHost, host.Favorite, host.Hidden, host.ConnectionCount)
	if summary := formatAdvancedSummary(host.Advanced); summary != "" {
		fmt.Fprintf(r.Out, "Advanced:\n%s\n", summary)
	}
	return nil
}

func (r *Runner) hostAdd(args []string) error {
	fs := newFlagSet("hosts add")
	hostname := fs.String("hostname", "", "server hostname or IP")
	user := fs.String("user", "", "remote user")
	port := fs.String("port", "", "remote port")
	identity := fs.String("identity", "", "identity file")
	passwordOnly := fs.Bool("password-only", false, "disable public-key authentication")
	proxy := fs.String("proxy-jump", "", "jump host")
	group := fs.String("group", "", "group path")
	environment := fs.String("environment", "", "environment")
	color := fs.String("color", "", "label colour")
	notes := fs.String("notes", "", "notes")
	var tags stringsFlag
	var advanced hostAdvancedAddFlags
	fs.Var(&tags, "tag", "tag (repeatable)")
	advanced.register(fs)
	args = positionalFirst(args)
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() > 1 {
		return usagef("usage: bast hosts add [label] --hostname host [options]")
	}
	wizard := fs.NArg() == 0 || *hostname == ""
	label := ""
	if fs.NArg() == 1 {
		label = fs.Arg(0)
	}
	var err error
	if label == "" {
		label, err = r.prompt("Label", "", true)
		if err != nil {
			return err
		}
	}
	if *hostname == "" {
		*hostname, err = r.prompt("Hostname", "", true)
		if err != nil {
			return err
		}
	}
	if wizard && r.interactive() && !r.NoInput {
		*user, err = r.prompt("User", *user, false)
		if err != nil {
			return err
		}
		*port, err = r.prompt("Port", *port, false)
		if err != nil {
			return err
		}
		auth := *identity
		if *passwordOnly {
			auth = "password"
		}
		auth, err = r.prompt("Identity file (blank for defaults, or password)", auth, false)
		if err != nil {
			return err
		}
		*passwordOnly = strings.EqualFold(auth, "password")
		if *passwordOnly {
			*identity = ""
		} else {
			*identity = auth
		}
	}
	var adv sshconfig.AdvancedSettings
	groupPrompted := false
	if wizard && r.interactive() && !r.NoInput {
		adv, err = promptAdvancedSettings(r, sshconfig.AdvancedSettings{ProxyJump: *proxy})
		if err != nil {
			return err
		}
		*proxy = adv.ProxyJump
		if !strings.Contains(label, "/") {
			*group, err = r.prompt("Group", *group, false)
			if err != nil {
				return err
			}
			groupPrompted = true
		}
		tagText, promptErr := r.prompt("Tags (comma separated)", strings.Join(tags, ", "), false)
		if promptErr != nil {
			return promptErr
		}
		tags = stringsFlag{tagText}
		*environment, err = r.prompt("Environment", *environment, false)
		if err != nil {
			return err
		}
		*color, err = r.prompt("Color", *color, false)
		if err != nil {
			return err
		}
		*notes, err = r.prompt("Notes", *notes, false)
		if err != nil {
			return err
		}
	}
	if *identity != "" && *passwordOnly {
		return usagef("--identity and --password-only cannot be used together")
	}
	groupFlagSet := groupPrompted
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "group" {
			groupFlagSet = true
		}
	})
	leaf, normalizedGroup, err := resolveLabelAndGroup(label, groupFlagSet, *group, false)
	if err != nil {
		return err
	}
	if !(wizard && r.interactive() && !r.NoInput) {
		adv, err = advanced.settings(*proxy, nil)
		if err != nil {
			return err
		}
	}
	input := hostInputFromAdvanced(sshconfig.HostInput{
		Alias: sshconfig.NormalizeAlias(leaf), HostName: *hostname, User: *user, Port: *port,
		IdentityFile: *identity, PasswordOnly: *passwordOnly,
	}, adv)
	host, err := r.config.Add(input)
	if err != nil {
		return err
	}
	meta := metadata.Host{Label: leaf, Group: normalizedGroup, Tags: splitValues(tags), Environment: *environment, Color: *color, Notes: *notes}
	if meta.Label == input.Alias {
		meta.Label = ""
	}
	if err := r.store.SetHost(input.Alias, meta); err != nil {
		return err
	}
	return r.success(map[string]string{"alias": host.Alias}, "Host saved: "+host.Alias)
}

func (r *Runner) hostEdit(args []string) error {
	fs := newFlagSet("hosts edit")
	var label, hostname, user, port, identity, proxy, group, environment, color, notes optionalString
	fs.Var(&label, "label", "new display label and managed alias")
	fs.Var(&hostname, "hostname", "server hostname or IP")
	fs.Var(&user, "user", "remote user")
	fs.Var(&port, "port", "remote port")
	fs.Var(&identity, "identity", "identity file")
	fs.Var(&proxy, "proxy-jump", "jump host")
	fs.Var(&group, "group", "group path")
	fs.Var(&environment, "environment", "environment")
	fs.Var(&color, "color", "label colour")
	fs.Var(&notes, "notes", "notes")
	passwordOnly := fs.Bool("password-only", false, "use password only")
	clearUser := fs.Bool("clear-user", false, "clear remote user")
	clearPort := fs.Bool("clear-port", false, "clear port")
	clearIdentity := fs.Bool("clear-identity", false, "use OpenSSH defaults")
	clearProxy := fs.Bool("clear-proxy-jump", false, "clear jump host")
	clearGroup := fs.Bool("clear-group", false, "clear group")
	clearTags := fs.Bool("clear-tags", false, "clear tags")
	clearEnvironment := fs.Bool("clear-environment", false, "clear environment")
	clearColor := fs.Bool("clear-color", false, "clear colour")
	clearNotes := fs.Bool("clear-notes", false, "clear notes")
	var tags stringsFlag
	var advanced hostAdvancedEditFlags
	fs.Var(&tags, "tag", "replacement tag (repeatable)")
	advanced.register(fs)
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast hosts edit <host> [options]")
	}
	if (identity.set && (*passwordOnly || *clearIdentity)) || (*passwordOnly && *clearIdentity) {
		return usagef("--identity, --password-only, and --clear-identity are mutually exclusive")
	}
	for _, conflict := range []struct {
		set   bool
		clear bool
		name  string
	}{{user.set, *clearUser, "user"}, {port.set, *clearPort, "port"}, {proxy.set, *clearProxy, "proxy-jump"}, {group.set, *clearGroup, "group"}, {environment.set, *clearEnvironment, "environment"}, {color.set, *clearColor, "color"}, {notes.set, *clearNotes, "notes"}, {len(tags) > 0, *clearTags, "tags"}} {
		if conflict.set && conflict.clear {
			return usagef("cannot set and clear %s in the same command", conflict.name)
		}
	}
	if err := advanced.conflicts(); err != nil {
		return err
	}
	hosts, _, err := r.snapshot()
	if err != nil {
		return err
	}
	host, err := r.findHost(fs.Arg(0), hosts)
	if err != nil {
		return err
	}
	configChanged := hostname.set || user.set || port.set || identity.set || proxy.set || *passwordOnly || *clearUser || *clearPort || *clearIdentity || *clearProxy || advanced.changed()
	metadataChanged := label.set || group.set || environment.set || color.set || notes.set || len(tags) > 0 || *clearGroup || *clearTags || *clearEnvironment || *clearColor || *clearNotes
	var wizardAdvanced *sshconfig.AdvancedSettings
	if !configChanged && !metadataChanged {
		if r.NoInput || !r.interactive() {
			return usagef("no changes supplied")
		}
		label.value, err = r.prompt("Label", metadata.JoinLabelPath(host.Group, host.Label), true)
		if err != nil {
			return err
		}
		label.set = true
		if host.Managed {
			hostname.value, err = r.prompt("Hostname", host.Hostname, true)
			if err != nil {
				return err
			}
			hostname.set = true
			user.value, err = r.prompt("User", host.User, false)
			if err != nil {
				return err
			}
			user.set = true
			port.value, err = r.prompt("Port", host.Port, false)
			if err != nil {
				return err
			}
			port.set = true
			auth := "default"
			if host.Authentication == "password" {
				auth = "password"
			} else if host.Authentication == "identity" && len(host.IdentityFiles) > 0 {
				auth = host.IdentityFiles[0]
			}
			auth, err = r.prompt("Identity file (default, password, or path)", auth, false)
			if err != nil {
				return err
			}
			switch strings.ToLower(auth) {
			case "", "default":
				*clearIdentity = true
			case "password":
				*passwordOnly = true
			default:
				identity.value, identity.set = auth, true
			}
			currentAdv, advErr := loadHostAdvanced(r.config, host.raw, host.ProxyJump)
			if advErr != nil {
				return advErr
			}
			nextAdv, promptErr := promptAdvancedSettings(r, currentAdv)
			if promptErr != nil {
				return promptErr
			}
			wizardAdvanced = &nextAdv
			proxy.value, proxy.set = nextAdv.ProxyJump, true
		}
		if !strings.Contains(label.value, "/") {
			group.value, err = r.prompt("Group", host.Group, false)
			if err != nil {
				return err
			}
			group.set = true
		}
		tagText, promptErr := r.prompt("Tags (comma separated)", strings.Join(host.Tags, ", "), false)
		if promptErr != nil {
			return promptErr
		}
		tags = stringsFlag{tagText}
		if tagText == "" {
			*clearTags = true
		}
		environment.value, err = r.prompt("Environment", host.Environment, false)
		if err != nil {
			return err
		}
		environment.set = true
		color.value, err = r.prompt("Color", host.Color, false)
		if err != nil {
			return err
		}
		color.set = true
		notes.value, err = r.prompt("Notes", host.Notes, false)
		if err != nil {
			return err
		}
		notes.set = true
		configChanged, metadataChanged = host.Managed, true
	}
	if host.Synced && (configChanged || metadataChanged || label.set) {
		return fail("synced_host", "synced hosts are read-only; manage them with bast sync")
	}
	if host.Managed && label.set {
		configChanged = true
	}
	if !host.Managed && configChanged {
		return fail("external_host", "externally managed hosts only support metadata changes")
	}
	newAlias := host.Alias
	newMeta := host.meta
	if label.set {
		leaf, parsedGroup, pathErr := resolveLabelAndGroup(label.value, group.set, group.value, *clearGroup)
		if pathErr != nil {
			return pathErr
		}
		if host.Managed {
			newAlias = sshconfig.NormalizeAlias(leaf)
		}
		newMeta.Label = leaf
		// Only apply a group from the label when the label carries path info.
		// A plain label rename must preserve the existing host group.
		if !group.set && !*clearGroup && strings.Contains(strings.TrimSpace(label.value), "/") {
			newMeta.Group = parsedGroup
		}
	}
	if group.set {
		newMeta.Group, err = normalizeGroup(group.value)
		if err != nil {
			return err
		}
	}
	if len(tags) > 0 {
		newMeta.Tags = splitValues(tags)
	}
	if environment.set {
		newMeta.Environment = environment.value
	}
	if color.set {
		newMeta.Color = color.value
	}
	if notes.set {
		newMeta.Notes = notes.value
	}
	if *clearGroup {
		newMeta.Group = ""
	}
	if *clearTags {
		newMeta.Tags = nil
	}
	if *clearEnvironment {
		newMeta.Environment = ""
	}
	if *clearColor {
		newMeta.Color = ""
	}
	if *clearNotes {
		newMeta.Notes = ""
	}
	if newMeta.Label == newAlias {
		newMeta.Label = ""
	}
	if host.Managed && configChanged {
		currentIdentity := ""
		if host.Authentication == "identity" && len(host.IdentityFiles) > 0 {
			currentIdentity = host.IdentityFiles[0]
		}
		var nextAdv sshconfig.AdvancedSettings
		if wizardAdvanced != nil {
			nextAdv = *wizardAdvanced
		} else {
			currentAdv, advErr := loadHostAdvanced(r.config, host.raw, host.ProxyJump)
			if advErr != nil {
				return advErr
			}
			var advErr2 error
			nextAdv, advErr2 = advanced.apply(currentAdv, proxy.value, proxy.set, *clearProxy)
			if advErr2 != nil {
				return advErr2
			}
		}
		input := hostInputFromAdvanced(sshconfig.HostInput{
			Alias: newAlias, HostName: host.Hostname, User: host.User, Port: host.Port,
			IdentityFile: currentIdentity, PasswordOnly: host.Authentication == "password",
		}, nextAdv)
		if hostname.set {
			input.HostName = hostname.value
		}
		if user.set {
			input.User = user.value
		}
		if port.set {
			input.Port = port.value
		}
		if identity.set {
			input.IdentityFile, input.PasswordOnly = identity.value, false
		}
		if *passwordOnly {
			input.IdentityFile, input.PasswordOnly = "", true
		}
		if *clearUser {
			input.User = ""
		}
		if *clearPort {
			input.Port = ""
		}
		if *clearIdentity {
			input.IdentityFile, input.PasswordOnly = "", false
		}
		if err := r.config.Update(host.raw.ManagedID, input); err != nil {
			return err
		}
	}
	if newAlias != host.Alias {
		if err := r.store.RenameHost(host.Alias, newAlias); err != nil {
			return err
		}
	}
	if err := r.store.SetHost(newAlias, newMeta); err != nil {
		return err
	}
	return r.success(map[string]string{"alias": newAlias}, "Host saved: "+newAlias)
}

func (r *Runner) hostDelete(args []string) error {
	fs := newFlagSet("hosts delete")
	yes := fs.Bool("yes", false, "skip confirmation")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast hosts delete <host> [--yes]")
	}
	hosts, _, err := r.snapshot()
	if err != nil {
		return err
	}
	host, err := r.findHost(fs.Arg(0), hosts)
	if err != nil {
		return err
	}
	if host.Synced {
		return fail("synced_host", "synced hosts cannot be deleted; use bast sync disable "+host.SyncSource)
	}
	if !host.Managed {
		return fail("external_host", "externally managed hosts cannot be deleted by Bast")
	}
	if err := r.confirm(host.Label, *yes); err != nil {
		return err
	}
	if err := r.config.Delete(host.raw.ManagedID); err != nil {
		return err
	}
	if err := r.store.DeleteHost(host.Alias); err != nil {
		return err
	}
	return r.success(map[string]string{"alias": host.Alias}, "Host deleted: "+host.Alias)
}

func (r *Runner) hostState(args []string, action string) error {
	fs := newFlagSet("hosts " + action)
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast hosts %s <host>", action)
	}
	hosts, _, err := r.snapshot()
	if err != nil {
		return err
	}
	host, err := r.findHost(fs.Arg(0), hosts)
	if err != nil {
		return err
	}
	meta := host.meta
	switch action {
	case "favorite":
		meta.Favorite = true
	case "unfavorite":
		meta.Favorite = false
	case "hide":
		meta.Hidden = true
	case "show":
		meta.Hidden = false
	}
	if err := r.store.SetHost(host.Alias, meta); err != nil {
		return err
	}
	return r.success(map[string]any{"alias": host.Alias, "favorite": meta.Favorite, "hidden": meta.Hidden}, "Host updated: "+host.Alias)
}

func (r *Runner) hostSort(args []string) error {
	fs := newFlagSet("hosts sort")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast hosts sort <smart|label|recent|group>")
	}
	order := fs.Arg(0)
	if order == "label" {
		order = "alias"
	}
	if err := sortHosts(nil, order); err != nil {
		return err
	}
	if err := r.store.SetSort(order); err != nil {
		return err
	}
	display := order
	if display == "alias" {
		display = "label"
	}
	return r.success(map[string]string{"sort": display}, "Sort: "+display)
}

func (r *Runner) knownHost(args []string) error {
	if len(args) == 0 || args[0] != "remove" {
		return usagef("usage: bast hosts known-host remove <host> [--yes]")
	}
	fs := newFlagSet("hosts known-host remove")
	yes := fs.Bool("yes", false, "skip confirmation")
	if err := fs.Parse(positionalFirst(args[1:])); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast hosts known-host remove <host> [--yes]")
	}
	hosts, _, err := r.snapshot()
	if err != nil {
		return err
	}
	host, err := r.findHost(fs.Arg(0), hosts)
	if err != nil {
		return err
	}
	if host.Hostname == "" {
		return fail("operation_failed", "host could not be resolved by OpenSSH")
	}
	if err := r.confirm(host.Label, *yes); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := r.OpenSSH.RemoveKnownHost(ctx, host.Hostname, host.Port); err != nil {
		return err
	}
	return r.success(map[string]string{"alias": host.Alias}, "Known-host entry removed: "+host.Alias)
}

func (r *Runner) connect(args []string) error {
	fs := newFlagSet("connect")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast connect <host>")
	}
	if r.JSON {
		return fail("interactive_required", "connect requires an interactive terminal and does not support --json")
	}
	hosts, _, err := r.snapshot()
	if err != nil {
		return err
	}
	host, err := r.findHost(fs.Arg(0), hosts)
	if err != nil {
		return err
	}
	connectbanner.Write(r.Out)
	if host.Synced && host.SyncSource == "gcp" && host.SyncID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		engine := sync.New(r.Paths, r.store)
		if err := engine.EnsureGCPAccess(ctx, sshconfig.Host{
			Alias: host.Alias, Synced: host.Synced, SyncSource: host.SyncSource, SyncID: host.SyncID,
		}, connectbanner.Status(r.Out)); err != nil {
			return fail("gcp_access", err.Error())
		}
		fmt.Fprint(r.Out, "\r\n")
	} else if host.Synced && host.SyncSource == "aws" && host.SyncID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		engine := sync.New(r.Paths, r.store)
		if err := engine.EnsureAWSAccess(ctx, sshconfig.Host{
			Alias: host.Alias, Synced: host.Synced, SyncSource: host.SyncSource, SyncID: host.SyncID,
		}, connectbanner.Status(r.Out)); err != nil {
			return fail("aws_access", err.Error())
		}
		fmt.Fprint(r.Out, "\r\n")
	} else if host.Synced && host.SyncSource == "azure" && host.SyncID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		engine := sync.New(r.Paths, r.store)
		if err := engine.EnsureAzureAccess(ctx, sshconfig.Host{
			Alias: host.Alias, Synced: host.Synced, SyncSource: host.SyncSource, SyncID: host.SyncID,
		}, connectbanner.Status(r.Out)); err != nil {
			return fail("azure_access", err.Error())
		}
		fmt.Fprint(r.Out, "\r\n")
	}
	if err := r.store.RecordUse(host.Alias); err != nil {
		return err
	}
	cmd, err := r.OpenSSH.SSHCommand(host.Alias)
	if err != nil {
		return err
	}
	telemetry.Track("connect", r.Version)
	return r.runProcess(cmd, false)
}

func normalizeGroup(group string) (string, error) {
	normalized, err := metadata.NormalizeGroupPath(group)
	if err != nil {
		return "", fail("validation", err.Error())
	}
	return normalized, nil
}

// resolveLabelAndGroup peels path-in-label syntax into a leaf name.
// Explicit --group overrides only the group; a path in the label still peels.
// clear-group clears the group and still peels a path leaf when present.
func resolveLabelAndGroup(label string, groupSet bool, groupValue string, clearGroup bool) (leaf, group string, err error) {
	label = strings.TrimSpace(label)
	if clearGroup {
		if strings.Contains(label, "/") {
			_, leaf, err = metadata.SplitLabelPath(label)
			if err != nil {
				return "", "", fail("validation", err.Error())
			}
			return leaf, "", nil
		}
		return label, "", nil
	}
	if groupSet {
		group, err = normalizeGroup(groupValue)
		if err != nil {
			return "", "", err
		}
		if strings.Contains(label, "/") {
			_, leaf, err = metadata.SplitLabelPath(label)
			if err != nil {
				return "", "", fail("validation", err.Error())
			}
			return leaf, group, nil
		}
		return label, group, nil
	}
	if strings.Contains(label, "/") {
		group, leaf, err = metadata.SplitLabelPath(label)
		if err != nil {
			return "", "", fail("validation", err.Error())
		}
		return leaf, group, nil
	}
	return label, "", nil
}

func splitValues(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			if item != "" && !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	return out
}
