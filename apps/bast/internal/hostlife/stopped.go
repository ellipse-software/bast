// Package hostlife reports whether a synced host is stopped.
// The TUI and the CLI share this so a jump is refused in both places.
package hostlife

import (
	"bast/internal/cloud"
	bitqatecloud "bast/internal/cloud/bitqate"
	boatcloud "bast/internal/cloud/boat"
	boxdcloud "bast/internal/cloud/boxd"
	docloud "bast/internal/cloud/digitalocean"
	flycloud "bast/internal/cloud/fly"
	hetznercloud "bast/internal/cloud/hetzner"
	railwaycloud "bast/internal/cloud/railway"
	upstashcloud "bast/internal/cloud/upstash"
	vercelcloud "bast/internal/cloud/vercel"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func LooksStopped(host sshconfig.Host, meta metadata.Host) bool {
	if !host.Synced {
		return false
	}
	kind, ok := cloud.KindForSource(host.SyncSource)
	if !ok || !cloud.CapabilitiesFor(kind).Stop {
		return false
	}
	switch kind {
	case cloud.Boat:
		return boatcloud.HostLooksStopped(host.Resolved.HostName, meta.Tags)
	case cloud.Upstash:
		return upstashcloud.HostLooksStopped(meta.Tags)
	case cloud.Vercel:
		return vercelcloud.HostLooksStopped(meta.Tags)
	case cloud.Hetzner:
		return hetznercloud.HostLooksStopped(meta.Tags)
	case cloud.Bitqate:
		return bitqatecloud.HostLooksStopped(meta.Tags)
	case cloud.Railway:
		return railwaycloud.HostLooksStopped(meta.Tags)
	case cloud.Fly:
		return flycloud.HostLooksStopped(host.Resolved.HostName, meta.Tags)
	case cloud.DigitalOcean:
		return docloud.HostLooksStopped(meta.Tags, "")
	case cloud.Boxd:
		return boxdcloud.HostLooksStopped(meta.Tags)
	default:
		return false
	}
}
