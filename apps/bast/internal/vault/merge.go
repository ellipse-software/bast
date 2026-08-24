package vault

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"bast/internal/metadata"
)

// Merge combines local and remote documents by ManagedID / key fingerprint.
func Merge(local, remote Document, mode MergeMode) MergeResult {
	summary := Summary{
		LocalHosts:  countLiveHosts(local.Hosts),
		RemoteHosts: countLiveHosts(remote.Hosts),
		LocalKeys:   countLiveKeys(local.Keys),
		RemoteKeys:  countLiveKeys(remote.Keys),
	}
	switch mode {
	case MergeModeReplaceLocal:
		out := remote
		out.Version = DocumentVersion
		if out.UpdatedAt == 0 {
			out.UpdatedAt = time.Now().UTC().Unix()
		}
		return MergeResult{Document: out, Summary: summary}
	case MergeModeReplaceRemote:
		out := local
		out.Version = DocumentVersion
		out.UpdatedAt = time.Now().UTC().Unix()
		return MergeResult{Document: out, Summary: summary}
	}

	now := time.Now().UTC().Unix()
	tombHosts := mergeTombstones(local.Tombstones.Hosts, remote.Tombstones.Hosts)
	tombKeys := mergeTombstones(local.Tombstones.Keys, remote.Tombstones.Keys)

	hosts, hostConflicts := mergeHosts(local.Hosts, remote.Hosts, tombHosts)
	keys, keyConflicts := mergeKeys(local.Keys, remote.Keys, tombKeys)
	conflicts := append(hostConflicts, keyConflicts...)

	meta := mergeMetadata(local.Metadata, remote.Metadata, hosts)
	prefs := local.Preferences
	if remote.Preferences.Sort != "" && (local.Preferences.Sort == "" || remote.UpdatedAt >= local.UpdatedAt) {
		prefs = remote.Preferences
	} else if local.Preferences.Sort == "" {
		prefs = remote.Preferences
	}
	integrations := mergeIntegrations(remote.Integrations, local.Integrations)
	if remote.UpdatedAt >= local.UpdatedAt {
		integrations = mergeIntegrations(local.Integrations, remote.Integrations)
	}

	doc := Document{
		Version:      DocumentVersion,
		Revision:     remote.Revision,
		UpdatedAt:    now,
		Hosts:        hosts,
		Keys:         keys,
		Metadata:     meta,
		Preferences:  prefs,
		Integrations: integrations,
		Tombstones: Tombstones{
			Hosts: tombHosts,
			Keys:  tombKeys,
		},
	}
	summary.Conflicts = len(conflicts)
	return MergeResult{Document: doc, Conflicts: conflicts, Summary: summary}
}

func countLiveHosts(hosts []HostEntry) int {
	n := 0
	for _, h := range hosts {
		if h.DeletedAt == 0 {
			n++
		}
	}
	return n
}

func countLiveKeys(keys []KeyEntry) int {
	n := 0
	for _, k := range keys {
		if k.DeletedAt == 0 {
			n++
		}
	}
	return n
}

func mergeTombstones(a, b map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if cur, ok := out[k]; !ok || v > cur {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeHosts(local, remote []HostEntry, tombs map[string]int64) ([]HostEntry, []Conflict) {
	byID := map[string]HostEntry{}
	for _, h := range local {
		if strings.TrimSpace(h.ManagedID) == "" {
			continue
		}
		byID[h.ManagedID] = h
	}
	for _, h := range remote {
		if strings.TrimSpace(h.ManagedID) == "" {
			continue
		}
		cur, ok := byID[h.ManagedID]
		if !ok || h.UpdatedAt >= cur.UpdatedAt {
			byID[h.ManagedID] = h
		}
	}
	aliasOwner := map[string]string{}
	var conflicts []Conflict
	out := make([]HostEntry, 0, len(byID))
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		h := byID[id]
		if deletedAt, ok := tombs[id]; ok && deletedAt >= h.UpdatedAt {
			h.DeletedAt = deletedAt
			byID[id] = h
			continue
		}
		if h.DeletedAt != 0 {
			continue
		}
		if other, taken := aliasOwner[h.Alias]; taken && other != h.ManagedID {
			conflicts = append(conflicts, Conflict{
				Kind:    "host_alias",
				Key:     h.Alias,
				Message: fmt.Sprintf("alias %q claimed by managed ids %s and %s", h.Alias, other, h.ManagedID),
			})
			continue
		}
		aliasOwner[h.Alias] = h.ManagedID
		out = append(out, h)
	}
	return out, conflicts
}

func mergeKeys(local, remote []KeyEntry, tombs map[string]int64) ([]KeyEntry, []Conflict) {
	byFP := map[string]KeyEntry{}
	for _, k := range local {
		byFP[keyID(k)] = k
	}
	for _, k := range remote {
		id := keyID(k)
		cur, ok := byFP[id]
		if !ok || k.UpdatedAt >= cur.UpdatedAt {
			byFP[id] = k
		}
	}
	nameOwner := map[string]string{}
	var conflicts []Conflict
	out := make([]KeyEntry, 0, len(byFP))
	ids := make([]string, 0, len(byFP))
	for id := range byFP {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		k := byFP[id]
		if deletedAt, ok := tombs[id]; ok && deletedAt >= k.UpdatedAt {
			continue
		}
		if deletedAt, ok := tombs["name:"+k.Name]; ok && deletedAt >= k.UpdatedAt {
			continue
		}
		if k.DeletedAt != 0 {
			continue
		}
		if other, taken := nameOwner[k.Name]; taken && other != id {
			conflicts = append(conflicts, Conflict{
				Kind:    "key_name",
				Key:     k.Name,
				Message: fmt.Sprintf("key name %q claimed by different fingerprints", k.Name),
			})
			continue
		}
		nameOwner[k.Name] = id
		out = append(out, k)
	}
	return out, conflicts
}

func mergeMetadata(local, remote map[string]metadata.Host, hosts []HostEntry) map[string]metadata.Host {
	out := map[string]metadata.Host{}
	for alias, meta := range local {
		out[alias] = meta
	}
	for alias, meta := range remote {
		if _, ok := out[alias]; !ok {
			out[alias] = meta
		}
	}
	alive := map[string]bool{}
	for _, h := range hosts {
		alive[h.Alias] = true
	}
	for alias := range out {
		if !alive[alias] {
			delete(out, alias)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeIntegrations(base, overlay VaultIntegrations) VaultIntegrations {
	out := base
	if overlay.GCP != nil {
		out.GCP = overlay.GCP
	}
	if overlay.AWS != nil {
		out.AWS = overlay.AWS
	}
	if overlay.Azure != nil {
		out.Azure = overlay.Azure
	}
	if overlay.Upstash != nil {
		out.Upstash = overlay.Upstash
	}
	if overlay.Railway != nil {
		out.Railway = overlay.Railway
	}
	return out
}

func keyID(k KeyEntry) string {
	if k.Fingerprint != "" {
		return k.Fingerprint
	}
	return "name:" + k.Name
}

// MarkHostDeleted records a tombstone for a managed host id.
func (d *Document) MarkHostDeleted(managedID string, at int64) {
	if d.Tombstones.Hosts == nil {
		d.Tombstones.Hosts = map[string]int64{}
	}
	d.Tombstones.Hosts[managedID] = at
	for i := range d.Hosts {
		if d.Hosts[i].ManagedID == managedID {
			d.Hosts[i].DeletedAt = at
		}
	}
}

// MarkKeyDeleted records a tombstone for a key fingerprint or name.
func (d *Document) MarkKeyDeleted(fingerprint, name string, at int64) {
	if d.Tombstones.Keys == nil {
		d.Tombstones.Keys = map[string]int64{}
	}
	id := fingerprint
	if id == "" {
		id = "name:" + name
	}
	d.Tombstones.Keys[id] = at
	for i := range d.Keys {
		if (fingerprint != "" && d.Keys[i].Fingerprint == fingerprint) ||
			(fingerprint == "" && d.Keys[i].Name == name) {
			d.Keys[i].DeletedAt = at
		}
	}
}
