package azure

import (
	"fmt"
	"strings"

	"bast/internal/cloud/sshutil"
	"bast/internal/sshconfig"
)

func ToSyncHost(inst Instance, alias, bastExecutable string) sshconfig.SyncHostInput {
	if alias == "" {
		alias = AliasFor(inst)
	}
	input := sshconfig.SyncHostInput{
		Alias: alias, SyncSource: ProviderName, SyncID: inst.SyncID,
		HostName: inst.HostName, User: inst.User, IdentityFile: inst.IdentityFile,
		IdentitiesOnly: inst.IdentitiesOnly,
	}
	if inst.UseBastion {
		input.ProxyCommand = BastionProxyCommand(inst, bastExecutable)
	}
	return input
}

func BastionProxyCommand(inst Instance, bastExecutable string) string {
	if strings.TrimSpace(bastExecutable) == "" {
		bastExecutable = "bast"
	}
	args := []string{
		bastExecutable, "__azure-bastion-proxy",
		"--subscription", inst.SubscriptionID,
		"--bastion-group", inst.BastionResourceGroup,
		"--bastion", inst.BastionName,
		"--target", inst.SyncID,
		"--resource-port", "%p",
	}
	for i := range args {
		if args[i] != "%p" {
			args[i] = sshutil.ProxyLiteral(args[i])
		}
		args[i] = sshutil.ShellQuote(args[i])
	}
	return strings.Join(args, " ")
}

func GroupPath(inst Instance) string {
	subscription := safeGroupPart(inst.SubscriptionName)
	group := safeGroupPart(inst.ResourceGroup)
	if subscription == "" {
		return "Microsoft Azure"
	}
	if group == "" {
		return "Microsoft Azure/" + subscription
	}
	return "Microsoft Azure/" + subscription + "/" + group
}

func AliasFor(inst Instance) string {
	subscription := sshutil.SanitizeAliasPart(inst.SubscriptionName)
	group := sshutil.SanitizeAliasPart(inst.ResourceGroup)
	name := sshutil.SanitizeAliasPart(inst.Name)
	if subscription == "" {
		subscription = sshutil.SanitizeAliasPart(inst.SubscriptionID)
	}
	if subscription == "" {
		subscription = "subscription"
	}
	if group == "" {
		group = "group"
	}
	if name == "" {
		name = "vm"
	}
	return "azure_" + subscription + "_" + group + "_" + name
}

func UniqueAlias(base string, used map[string]bool) string {
	return sshutil.UniqueAlias(base, used)
}

func ParseSyncID(syncID string) (subscriptionID, resourceGroup, name string, err error) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(syncID), "/"), "/")
	if len(parts) != 8 || !strings.EqualFold(parts[0], "subscriptions") ||
		!strings.EqualFold(parts[2], "resourceGroups") || !strings.EqualFold(parts[4], "providers") ||
		!strings.EqualFold(parts[5], "Microsoft.Compute") || !strings.EqualFold(parts[6], "virtualMachines") {
		return "", "", "", fmt.Errorf("invalid Azure sync id %q", syncID)
	}
	subscriptionID, resourceGroup, name = parts[1], parts[3], parts[7]
	if subscriptionID == "" || resourceGroup == "" || name == "" {
		return "", "", "", fmt.Errorf("invalid Azure sync id %q", syncID)
	}
	return subscriptionID, resourceGroup, name, nil
}

func safeGroupPart(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "/", "-")
}
