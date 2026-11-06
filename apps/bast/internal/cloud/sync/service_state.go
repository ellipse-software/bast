package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bast/internal/cloud"
	vercelcloud "bast/internal/cloud/vercel"
)

func (s engineService) Snapshot(status Status) cloud.Snapshot {
	if s.e == nil || s.e.Store == nil {
		return cloud.Snapshot{}
	}
	switch s.kind {
	case cloud.GCP:
		g := s.e.Store.GCP()
		snap := base(g.Enabled, g.AutoSync, g.LastSyncAt, g.LastInstanceCount)
		if len(status.GCP.Accounts) > 0 {
			snap.Account = strings.Join(status.GCP.Accounts, ", ")
		} else if len(g.ServiceAccounts) > 0 {
			snap.Account = "service account"
		}
		snap = finish(snap, status.GCP.GCloudError, g.LastSyncError)
		snap.Settings = []cloud.Setting{
			textSetting(cloud.KeySSHUser, "SSH user", g.DefaultSSHUser, "ubuntu"),
			csvSetting(cloud.KeyProjects, "Projects", g.ProjectFilter),
			{Key: cloud.KeySAAdd, Label: "Add service account", Kind: cloud.SettingAction},
		}
		if len(g.ServiceAccounts) > 0 {
			snap.Settings = append(snap.Settings, cloud.Setting{
				Key: cloud.KeySARemove, Label: "Remove service account", Kind: cloud.SettingChoice,
				Choices: append([]string(nil), g.ServiceAccounts...),
			})
		}
		return snap
	case cloud.AWS:
		a := s.e.Store.AWS()
		snap := base(a.Enabled, a.AutoSync, a.LastSyncAt, a.LastInstanceCount)
		if len(status.AWS.Profiles) > 0 {
			snap.Account = strings.Join(status.AWS.Profiles, ", ")
		}
		snap = finish(snap, status.AWS.AWSCLIError, a.LastSyncError)
		snap.Settings = []cloud.Setting{
			textSetting(cloud.KeySSHUser, "SSH user", a.DefaultSSHUser, "ec2-user"),
			csvSetting(cloud.KeyProfiles, "Profiles", a.ProfileFilter),
			csvSetting(cloud.KeyRegions, "Regions", a.RegionFilter),
		}
		return snap
	case cloud.Azure:
		a := s.e.Store.Azure()
		snap := base(a.Enabled, a.AutoSync, a.LastSyncAt, a.LastInstanceCount)
		if len(status.Azure.Subscriptions) > 0 {
			snap.Account = strings.Join(status.Azure.Subscriptions, ", ")
		}
		cli := status.Azure.AzureCLIError
		if cli == "" {
			cli = status.Azure.SSHExtensionError
		}
		if cli == "" {
			cli = status.Azure.BastionExtensionError
		}
		snap = finish(snap, cli, a.LastSyncError)
		snap.Settings = []cloud.Setting{
			textSetting(cloud.KeySSHUser, "SSH user", a.DefaultSSHUser, "azureuser"),
			csvSetting(cloud.KeySubscriptions, "Subscriptions", a.SubscriptionFilter),
			csvSetting(cloud.KeyResourceGroups, "Resource groups", a.ResourceGroupFilter),
		}
		return snap
	case cloud.Boat:
		b := s.e.Store.Boat()
		snap := base(b.Enabled, b.AutoSync, b.LastSyncAt, b.LastInstanceCount)
		snap.Account = accountName(status.Boat.Authenticated, status.Boat.Login, "")
		if status.Boat.Plan != "" {
			if snap.Account == "" {
				snap.Account = status.Boat.Plan
			} else {
				snap.Account += " · " + status.Boat.Plan
			}
		}
		cli := status.Boat.BoatCLIError
		if cli == "" && !status.Boat.Authenticated && status.Boat.Login == "" {
			cli = "not logged in"
		}
		return finish(snap, cli, b.LastSyncError)
	case cloud.Upstash:
		u := s.e.Store.Upstash()
		snap := base(u.Enabled, u.AutoSync, u.LastSyncAt, u.LastInstanceCount)
		snap.Account, snap.Error = tokenAccount(status.Upstash.Error, status.Upstash.Authenticated, status.Upstash.HasKey || (s.e.Upstash != nil && s.e.Upstash.HasKey()), u.LastSyncError)
		return snap
	case cloud.Vercel:
		v := s.e.Store.Vercel()
		snap := base(v.Enabled, v.AutoSync, v.LastSyncAt, v.LastInstanceCount)
		has := status.Vercel.HasToken || (s.e.Vercel != nil && s.e.Vercel.HasToken())
		snap.Account, snap.Error = tokenAccount(status.Vercel.Error, status.Vercel.Authenticated, has, v.LastSyncError)
		if v.TeamID != "" && snap.Account != "" && snap.Account != "token stored" {
			snap.Account = v.TeamID
		} else if v.TeamID != "" && snap.Account == "" {
			snap.Account = v.TeamID
		}
		snap.Settings = []cloud.Setting{
			textSetting(cloud.KeyTeam, "Team", v.TeamID, "team_…"),
			csvSetting(cloud.KeyProjects, "Projects", v.Projects()),
		}
		if len(v.Unrestorable) > 0 {
			snap.Settings = append(snap.Settings, cloud.Setting{Key: cloud.KeyCleanup, Label: "Cleanup", Kind: cloud.SettingAction, Value: fmt.Sprintf("%d unrestorable", len(v.Unrestorable))})
		}
		return snap
	case cloud.Hetzner:
		h := s.e.Store.Hetzner()
		snap := base(h.Enabled, h.AutoSync, h.LastSyncAt, h.LastInstanceCount)
		has := status.Hetzner.HasToken || (s.e.Hetzner != nil && s.e.Hetzner.HasToken())
		snap.Account, snap.Error = tokenAccount(status.Hetzner.Error, status.Hetzner.Authenticated, has, h.LastSyncError)
		if len(status.Hetzner.Contexts) > 0 {
			snap.Account = strings.Join(status.Hetzner.Contexts, ", ")
		}
		snap.Settings = []cloud.Setting{
			boolSetting(cloud.KeyPreferPrivate, "Private IP", h.PreferPrivateIP),
			textSetting(cloud.KeySSHUser, "SSH user", h.DefaultSSHUser, "root"),
			textSetting(cloud.KeySSHPort, "SSH port", h.DefaultSSHPort, "22"),
			csvSetting(cloud.KeyContexts, "Contexts", h.ContextFilter),
			csvSetting(cloud.KeyLocations, "Locations", h.LocationFilter),
		}
		if s.e.Hetzner != nil && len(s.e.Hetzner.StoredTokenNames()) > 0 {
			names := s.e.Hetzner.StoredTokenNames()
			snap.Settings = append(snap.Settings, cloud.Setting{Key: cloud.KeyRemoveToken, Label: "Remove token", Kind: cloud.SettingChoice, Choices: append([]string(nil), names...)})
		}
		return snap
	case cloud.Bitqate:
		b := s.e.Store.Bitqate()
		snap := base(b.Enabled, b.AutoSync, b.LastSyncAt, b.LastInstanceCount)
		has := status.Bitqate.HasToken || (s.e.Bitqate != nil && s.e.Bitqate.HasToken())
		snap.Account, snap.Error = tokenAccount(status.Bitqate.Error, status.Bitqate.Authenticated, has, b.LastSyncError)
		snap.Settings = []cloud.Setting{
			textSetting(cloud.KeySSHUser, "SSH user", b.DefaultSSHUser, "root"),
			textSetting(cloud.KeySSHPort, "SSH port", b.DefaultSSHPort, "22"),
			csvSetting(cloud.KeyRegions, "Regions", b.RegionFilter),
		}
		return snap
	case cloud.Railway:
		r := s.e.Store.Railway()
		snap := base(r.Enabled, r.AutoSync, r.LastSyncAt, r.LastInstanceCount)
		has := status.Railway.HasToken || (s.e.Railway != nil && s.e.Railway.HasToken())
		name := status.Railway.Name
		if name == "" {
			name = status.Railway.Email
		}
		snap.Account, snap.Error = tokenAccount(status.Railway.Error, status.Railway.Authenticated, has, r.LastSyncError)
		if name != "" && (snap.Account == "authenticated" || snap.Account == "") {
			snap.Account = name
		}
		return snap
	case cloud.Fly:
		f := s.e.Store.Fly()
		snap := base(f.Enabled, f.AutoSync, f.LastSyncAt, f.LastInstanceCount)
		snap.Account = accountName(status.Fly.Authenticated, status.Fly.Login, "")
		cli := status.Fly.FlyCLIError
		if cli == "" && !status.Fly.Authenticated && status.Fly.Login == "" {
			cli = "not logged in"
		}
		snap = finish(snap, cli, f.LastSyncError)
		snap.Settings = []cloud.Setting{
			textSetting(cloud.KeySSHUser, "SSH user", f.DefaultSSHUser, "root"),
			csvSetting(cloud.KeyOrgs, "Orgs", f.OrgFilter),
			csvSetting(cloud.KeyApps, "Apps", f.AppFilter),
		}
		return snap
	case cloud.DigitalOcean:
		d := s.e.Store.DigitalOcean()
		snap := base(d.Enabled, d.AutoSync, d.LastSyncAt, d.LastInstanceCount)
		if len(status.DigitalOcean.Contexts) > 0 {
			snap.Account = strings.Join(status.DigitalOcean.Contexts, ", ")
		}
		snap = finish(snap, status.DigitalOcean.DOCTLError, d.LastSyncError)
		snap.Settings = []cloud.Setting{
			textSetting(cloud.KeySSHUser, "SSH user", d.DefaultSSHUser, "root"),
			csvSetting(cloud.KeyContexts, "Contexts", d.ContextFilter),
			csvSetting(cloud.KeyRegions, "Regions", d.RegionFilter),
		}
		return snap
	case cloud.Boxd:
		b := s.e.Store.Boxd()
		snap := base(b.Enabled, b.AutoSync, b.LastSyncAt, b.LastInstanceCount)
		snap.Account = accountName(status.Boxd.Authenticated, status.Boxd.Login, status.Boxd.ActiveOrg)
		cli := status.Boxd.BoxdCLIError
		if cli == "" && !status.Boxd.Authenticated && status.Boxd.Login == "" {
			cli = "not logged in"
		}
		snap = finish(snap, cli, b.LastSyncError)
		snap.Settings = []cloud.Setting{csvSetting(cloud.KeyOrgs, "Orgs", b.OrgFilter)}
		return snap
	default:
		return cloud.Snapshot{}
	}
}

func (s engineService) Apply(ctx context.Context, key, value string) error {
	_ = ctx
	if s.e == nil || s.e.Store == nil {
		return fmt.Errorf("unknown provider %s", s.kind)
	}
	value = strings.TrimSpace(value)
	switch s.kind {
	case cloud.GCP:
		g := s.e.Store.GCP()
		switch key {
		case cloud.KeySSHUser:
			g.DefaultSSHUser = value
		case cloud.KeyProjects:
			g.ProjectFilter = splitCSV(value)
		case cloud.KeySAAdd:
			if err := ValidateServiceAccountPath(value); err != nil {
				return err
			}
			for _, existing := range g.ServiceAccounts {
				if existing == value {
					return nil
				}
			}
			g.ServiceAccounts = append(g.ServiceAccounts, value)
		case cloud.KeySARemove:
			kept := make([]string, 0, len(g.ServiceAccounts))
			found := false
			for _, existing := range g.ServiceAccounts {
				if existing == value {
					found = true
					continue
				}
				kept = append(kept, existing)
			}
			if !found {
				return fmt.Errorf("service account key %q not found", value)
			}
			g.ServiceAccounts = kept
		default:
			return fmt.Errorf("unknown setting %s", key)
		}
		return s.e.Store.SetGCP(g)
	case cloud.AWS:
		a := s.e.Store.AWS()
		switch key {
		case cloud.KeySSHUser:
			a.DefaultSSHUser = value
		case cloud.KeyProfiles:
			a.ProfileFilter = splitCSV(value)
		case cloud.KeyRegions:
			a.RegionFilter = splitCSV(value)
		default:
			return fmt.Errorf("unknown setting %s", key)
		}
		return s.e.Store.SetAWS(a)
	case cloud.Azure:
		a := s.e.Store.Azure()
		switch key {
		case cloud.KeySSHUser:
			a.DefaultSSHUser = value
		case cloud.KeySubscriptions:
			a.SubscriptionFilter = splitCSV(value)
		case cloud.KeyResourceGroups:
			a.ResourceGroupFilter = splitCSV(value)
		default:
			return fmt.Errorf("unknown setting %s", key)
		}
		return s.e.Store.SetAzure(a)
	case cloud.Hetzner:
		h := s.e.Store.Hetzner()
		switch key {
		case cloud.KeyPreferPrivate:
			h.PreferPrivateIP = value == "true" || value == "on"
		case cloud.KeySSHUser:
			h.DefaultSSHUser = value
		case cloud.KeySSHPort:
			h.DefaultSSHPort = value
		case cloud.KeyContexts:
			h.ContextFilter = splitCSV(value)
		case cloud.KeyLocations:
			h.LocationFilter = splitCSV(value)
		default:
			return fmt.Errorf("unknown setting %s", key)
		}
		return s.e.Store.SetHetzner(h)
	case cloud.Bitqate:
		b := s.e.Store.Bitqate()
		switch key {
		case cloud.KeySSHUser:
			b.DefaultSSHUser = value
		case cloud.KeySSHPort:
			b.DefaultSSHPort = value
		case cloud.KeyRegions:
			b.RegionFilter = splitCSV(value)
		default:
			return fmt.Errorf("unknown setting %s", key)
		}
		return s.e.Store.SetBitqate(b)
	case cloud.Fly:
		f := s.e.Store.Fly()
		switch key {
		case cloud.KeySSHUser:
			f.DefaultSSHUser = value
		case cloud.KeyOrgs:
			f.OrgFilter = splitCSV(value)
		case cloud.KeyApps:
			f.AppFilter = splitCSV(value)
		default:
			return fmt.Errorf("unknown setting %s", key)
		}
		return s.e.Store.SetFly(f)
	case cloud.DigitalOcean:
		d := s.e.Store.DigitalOcean()
		switch key {
		case cloud.KeySSHUser:
			d.DefaultSSHUser = value
		case cloud.KeyContexts:
			d.ContextFilter = splitCSV(value)
		case cloud.KeyRegions:
			d.RegionFilter = splitCSV(value)
		default:
			return fmt.Errorf("unknown setting %s", key)
		}
		return s.e.Store.SetDigitalOcean(d)
	case cloud.Boxd:
		b := s.e.Store.Boxd()
		if key != cloud.KeyOrgs {
			return fmt.Errorf("unknown setting %s", key)
		}
		b.OrgFilter = splitCSV(value)
		return s.e.Store.SetBoxd(b)
	case cloud.Vercel:
		v := s.e.Store.Vercel()
		switch key {
		case cloud.KeyTeam:
			v.TeamID = value
		case cloud.KeyProjects:
			setVercelProjects(&v, vercelcloud.ParseProjectList(value))
		default:
			return fmt.Errorf("unknown setting %s", key)
		}
		return s.e.Store.SetVercel(v)
	default:
		return fmt.Errorf("unknown setting %s", key)
	}
}

func (s engineService) setConnect() error {
	switch s.kind {
	case cloud.GCP:
		g := s.e.Store.GCP()
		g.Enabled = true
		g.AutoSync = true
		return s.e.Store.SetGCP(g)
	case cloud.AWS:
		a := s.e.Store.AWS()
		a.Enabled = true
		a.AutoSync = true
		return s.e.Store.SetAWS(a)
	case cloud.Azure:
		a := s.e.Store.Azure()
		a.Enabled = true
		a.AutoSync = true
		return s.e.Store.SetAzure(a)
	case cloud.Boat:
		b := s.e.Store.Boat()
		b.Enabled, b.AutoSync, b.Disabled = true, true, false
		return s.e.Store.SetBoat(b)
	case cloud.Upstash:
		u := s.e.Store.Upstash()
		u.Enabled, u.AutoSync, u.Disabled = true, true, false
		return s.e.Store.SetUpstash(u)
	case cloud.Vercel:
		v := s.e.Store.Vercel()
		v.Enabled, v.AutoSync, v.Disabled = true, true, false
		return s.e.Store.SetVercel(v)
	case cloud.Hetzner:
		h := s.e.Store.Hetzner()
		h.Enabled, h.AutoSync = true, true
		return s.e.Store.SetHetzner(h)
	case cloud.Bitqate:
		b := s.e.Store.Bitqate()
		b.Enabled, b.AutoSync = true, true
		return s.e.Store.SetBitqate(b)
	case cloud.Railway:
		r := s.e.Store.Railway()
		r.Enabled, r.AutoSync, r.Disabled = true, true, false
		return s.e.Store.SetRailway(r)
	case cloud.Fly:
		f := s.e.Store.Fly()
		f.Enabled, f.AutoSync, f.Disabled = true, true, false
		return s.e.Store.SetFly(f)
	case cloud.DigitalOcean:
		d := s.e.Store.DigitalOcean()
		d.Enabled, d.AutoSync = true, true
		return s.e.Store.SetDigitalOcean(d)
	case cloud.Boxd:
		b := s.e.Store.Boxd()
		b.Enabled, b.AutoSync, b.Disabled = true, true, false
		return s.e.Store.SetBoxd(b)
	default:
		return fmt.Errorf("unknown provider %s", s.kind)
	}
}

func (s engineService) setAuto(on bool) error {
	clear := on
	switch s.kind {
	case cloud.GCP:
		g := s.e.Store.GCP()
		g.AutoSync = on
		return s.e.Store.SetGCP(g)
	case cloud.AWS:
		a := s.e.Store.AWS()
		a.AutoSync = on
		return s.e.Store.SetAWS(a)
	case cloud.Azure:
		a := s.e.Store.Azure()
		a.AutoSync = on
		return s.e.Store.SetAzure(a)
	case cloud.Boat:
		b := s.e.Store.Boat()
		b.AutoSync = on
		if clear {
			b.Disabled = false
		}
		return s.e.Store.SetBoat(b)
	case cloud.Upstash:
		u := s.e.Store.Upstash()
		u.AutoSync = on
		if clear {
			u.Disabled = false
		}
		return s.e.Store.SetUpstash(u)
	case cloud.Vercel:
		v := s.e.Store.Vercel()
		v.AutoSync = on
		if clear {
			v.Disabled = false
		}
		return s.e.Store.SetVercel(v)
	case cloud.Hetzner:
		h := s.e.Store.Hetzner()
		h.AutoSync = on
		return s.e.Store.SetHetzner(h)
	case cloud.Bitqate:
		b := s.e.Store.Bitqate()
		b.AutoSync = on
		return s.e.Store.SetBitqate(b)
	case cloud.Railway:
		r := s.e.Store.Railway()
		r.AutoSync = on
		if clear {
			r.Disabled = false
		}
		return s.e.Store.SetRailway(r)
	case cloud.Fly:
		f := s.e.Store.Fly()
		f.AutoSync = on
		if clear {
			f.Disabled = false
		}
		return s.e.Store.SetFly(f)
	case cloud.DigitalOcean:
		d := s.e.Store.DigitalOcean()
		d.AutoSync = on
		return s.e.Store.SetDigitalOcean(d)
	case cloud.Boxd:
		b := s.e.Store.Boxd()
		b.AutoSync = on
		if clear {
			b.Disabled = false
		}
		return s.e.Store.SetBoxd(b)
	default:
		return fmt.Errorf("unknown provider %s", s.kind)
	}
}

func base(enabled, auto bool, at *time.Time, count int) cloud.Snapshot {
	return cloud.Snapshot{Enabled: enabled, AutoSync: auto, LastSyncAt: at, InstanceCount: count}
}

func finish(snap cloud.Snapshot, cliErr, lastSync string) cloud.Snapshot {
	if cliErr = strings.TrimSpace(cliErr); cliErr != "" {
		snap.Error = cliErr
		return snap
	}
	snap.Error = strings.TrimSpace(lastSync)
	return snap
}

func accountName(authenticated bool, login, extra string) string {
	name := strings.TrimSpace(login)
	if name == "" && authenticated {
		name = "authenticated"
	}
	extra = strings.TrimSpace(extra)
	if extra != "" && name != "" {
		return name + " (" + extra + ")"
	}
	if name == "" {
		return extra
	}
	return name
}

func tokenAccount(cliErr string, authenticated, has bool, lastSync string) (account, errText string) {
	if cliErr = strings.TrimSpace(cliErr); cliErr != "" {
		return "", cliErr
	}
	if authenticated {
		account = "authenticated"
	} else if has {
		account = "token stored"
	}
	if account == "" {
		return "", strings.TrimSpace(lastSync)
	}
	if strings.TrimSpace(lastSync) != "" && account != "" {
		return account, strings.TrimSpace(lastSync)
	}
	return account, ""
}

func textSetting(key, label, value, placeholder string) cloud.Setting {
	return cloud.Setting{Key: key, Label: label, Value: value, Placeholder: placeholder, Kind: cloud.SettingText, Optional: true}
}

func csvSetting(key, label string, values []string) cloud.Setting {
	return cloud.Setting{Key: key, Label: label, Value: strings.Join(values, ", "), Kind: cloud.SettingCSV, Optional: true}
}

func boolSetting(key, label string, on bool) cloud.Setting {
	value := "false"
	if on {
		value = "true"
	}
	return cloud.Setting{Key: key, Label: label, Value: value, Kind: cloud.SettingBool}
}

func splitCSV(value string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
