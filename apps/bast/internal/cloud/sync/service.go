package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"bast/internal/cloud"
)

// ErrCredentialRequired means connect needs a token before sync can start.
var ErrCredentialRequired = errors.New("credential required")

// Service is the uniform provider Bast's Sync tab calls.
// Implementations live here, beside Engine, so they can call it without an import cycle.
type Service interface {
	Kind() cloud.Kind
	Auth() cloud.AuthSpec
	Snapshot(status Status) cloud.Snapshot
	Sync(ctx context.Context) (Result, error)
	SyncTimeout() time.Duration
	SetEnabled(ctx context.Context, on bool) error
	SetAutoSync(ctx context.Context, on bool) error
	Apply(ctx context.Context, key, value string) error
	SaveCredential(ctx context.Context, values map[string]string) (Result, error)
	CreateLabel() string
	CanCreate(status Status) bool
}

type engineService struct {
	kind cloud.Kind
	e    *Engine
}

func newServices(e *Engine) map[cloud.Kind]Service {
	out := make(map[cloud.Kind]Service, len(cloud.Descriptors()))
	for _, d := range cloud.Descriptors() {
		out[d.Kind] = engineService{kind: d.Kind, e: e}
	}
	return out
}

func (e *Engine) Service(kind cloud.Kind) (Service, bool) {
	if e == nil || e.services == nil {
		return nil, false
	}
	svc, ok := e.services[kind]
	return svc, ok
}

func (s engineService) Kind() cloud.Kind { return s.kind }

func (s engineService) Auth() cloud.AuthSpec {
	if spec, ok := authSpecs[s.kind]; ok {
		return spec
	}
	return cloud.AuthSpec{}
}

func (s engineService) CreateLabel() string { return createLabels[s.kind] }

func (s engineService) SyncTimeout() time.Duration {
	if d, ok := syncTimeouts[s.kind]; ok {
		return d
	}
	return 2 * time.Minute
}

func (s engineService) Sync(ctx context.Context) (Result, error) {
	fn, ok := syncByKind[s.kind]
	if !ok || s.e == nil {
		return Result{}, fmt.Errorf("unknown provider %s", s.kind)
	}
	return fn(s.e, ctx)
}

func (s engineService) SetEnabled(ctx context.Context, on bool) error {
	if !on {
		fn, ok := disableByKind[s.kind]
		if !ok || s.e == nil {
			return fmt.Errorf("unknown provider %s", s.kind)
		}
		return fn(s.e, ctx)
	}
	if s.credentialMissing() {
		return ErrCredentialRequired
	}
	return s.setConnect()
}

func (s engineService) SetAutoSync(ctx context.Context, on bool) error {
	_ = ctx
	return s.setAuto(on)
}

func (s engineService) CanCreate(status Status) bool {
	if !cloud.CapabilitiesFor(s.kind).Create || s.e == nil {
		return false
	}
	switch s.kind {
	case cloud.Boat:
		return status.Boat.Authenticated
	case cloud.Upstash:
		return status.Upstash.HasKey || (s.e.Upstash != nil && s.e.Upstash.HasKey())
	case cloud.Vercel:
		has := status.Vercel.HasToken || (s.e.Vercel != nil && s.e.Vercel.HasToken())
		v := s.e.Store.Vercel()
		return has && strings.TrimSpace(v.TeamID) != "" && len(v.Projects()) > 0
	case cloud.Railway:
		return status.Railway.HasToken || (s.e.Railway != nil && s.e.Railway.HasToken())
	case cloud.Fly:
		return status.Fly.Authenticated || s.e.Store.Fly().Enabled
	case cloud.Boxd:
		return status.Boxd.Authenticated || s.e.Store.Boxd().Enabled
	case cloud.DigitalOcean:
		return true
	default:
		return false
	}
}

func (s engineService) credentialMissing() bool {
	if s.e == nil {
		return true
	}
	switch s.kind {
	case cloud.Upstash:
		return s.e.Upstash == nil || !s.e.Upstash.HasKey()
	case cloud.Vercel:
		return s.e.Vercel == nil || !s.e.Vercel.HasToken() || strings.TrimSpace(s.e.Store.Vercel().TeamID) == ""
	case cloud.Bitqate:
		return s.e.Bitqate == nil || !s.e.Bitqate.HasToken()
	case cloud.Hetzner:
		return s.e.Hetzner == nil || !s.e.Hetzner.HasToken()
	case cloud.Railway:
		return s.e.Railway == nil || !s.e.Railway.HasToken()
	default:
		return false
	}
}

func (s engineService) SaveCredential(ctx context.Context, values map[string]string) (Result, error) {
	if s.e == nil {
		return Result{}, fmt.Errorf("unknown provider %s", s.kind)
	}
	switch s.kind {
	case cloud.Upstash:
		return s.e.SaveUpstashKey(ctx, strings.TrimSpace(values["token"]))
	case cloud.Vercel:
		return s.e.SaveVercelToken(ctx, strings.TrimSpace(values["token"]), strings.TrimSpace(values["team"]), strings.TrimSpace(values["project"]))
	case cloud.Bitqate:
		return s.e.SaveBitqateKey(ctx, strings.TrimSpace(values["token"]))
	case cloud.Railway:
		return s.e.SaveRailwayToken(ctx, strings.TrimSpace(values["token"]))
	case cloud.Hetzner:
		name := strings.TrimSpace(values["name"])
		if name == "" {
			if s.e.Hetzner != nil && len(s.e.Hetzner.StoredTokenNames()) > 0 {
				return Result{}, fmt.Errorf("name is required")
			}
			name = "default"
		}
		return s.e.SaveHetznerKey(ctx, name, strings.TrimSpace(values["token"]))
	default:
		return Result{}, fmt.Errorf("%s does not store a token", s.kind)
	}
}

var authSpecs = map[cloud.Kind]cloud.AuthSpec{
	cloud.GCP:          {Kind: cloud.AuthCLI, Login: []string{"gcloud", "auth", "login"}},
	cloud.AWS:          {Kind: cloud.AuthCLI},
	cloud.Azure:        {Kind: cloud.AuthCLI, Login: []string{"az", "login"}},
	cloud.Boat:         {Kind: cloud.AuthCLI, Login: []string{"boat", "login"}},
	cloud.Upstash:      {Kind: cloud.AuthToken, TokenURL: "https://console.upstash.com", TokenLabel: "API key"},
	cloud.Vercel:       {Kind: cloud.AuthToken, TokenURL: "https://vercel.com/account/settings/tokens", TokenLabel: "Access token", TokenExtra: []cloud.Setting{{Key: "team", Label: "Team", Placeholder: "team_…"}, {Key: "project", Label: "Project", Placeholder: "prj_…", Optional: true}}},
	cloud.Hetzner:      {Kind: cloud.AuthToken, TokenURL: "https://console.hetzner.cloud", TokenLabel: "API token", TokenExtra: []cloud.Setting{{Key: "name", Label: "Name", Placeholder: "default", Optional: true}}},
	cloud.Bitqate:      {Kind: cloud.AuthToken, TokenLabel: "API token", TokenHint: "vms:read, vms:power for power"},
	cloud.Railway:      {Kind: cloud.AuthToken, TokenURL: "https://railway.com/account/tokens", TokenLabel: "API token"},
	cloud.Fly:          {Kind: cloud.AuthCLI, Login: []string{"fly", "auth", "login"}},
	cloud.DigitalOcean: {Kind: cloud.AuthCLI, Login: []string{"doctl", "auth", "init"}},
	cloud.Boxd:         {Kind: cloud.AuthCLI, Login: []string{"boxd", "auth", "login"}},
}

var createLabels = map[cloud.Kind]string{
	cloud.Boat:         "New sandbox",
	cloud.Upstash:      "New box",
	cloud.Vercel:       "New sandbox",
	cloud.Railway:      "New service",
	cloud.Fly:          "New machine",
	cloud.DigitalOcean: "New droplet",
	cloud.Boxd:         "New computer",
}

var syncTimeouts = map[cloud.Kind]time.Duration{
	cloud.AWS:          5 * time.Minute,
	cloud.Azure:        5 * time.Minute,
	cloud.Railway:      3 * time.Minute,
	cloud.Fly:          3 * time.Minute,
	cloud.Boxd:         3 * time.Minute,
	cloud.DigitalOcean: 3 * time.Minute,
}

var syncByKind = map[cloud.Kind]func(*Engine, context.Context) (Result, error){
	cloud.GCP:          (*Engine).SyncGCP,
	cloud.AWS:          (*Engine).SyncAWS,
	cloud.Azure:        (*Engine).SyncAzure,
	cloud.Boat:         (*Engine).SyncBoat,
	cloud.Upstash:      (*Engine).SyncUpstash,
	cloud.Vercel:       (*Engine).SyncVercel,
	cloud.Hetzner:      (*Engine).SyncHetzner,
	cloud.Bitqate:      (*Engine).SyncBitqate,
	cloud.Railway:      (*Engine).SyncRailway,
	cloud.Fly:          (*Engine).SyncFly,
	cloud.DigitalOcean: (*Engine).SyncDigitalOcean,
	cloud.Boxd:         (*Engine).SyncBoxd,
}

var disableByKind = map[cloud.Kind]func(*Engine, context.Context) error{
	cloud.GCP:          (*Engine).DisableGCP,
	cloud.AWS:          (*Engine).DisableAWS,
	cloud.Azure:        (*Engine).DisableAzure,
	cloud.Boat:         (*Engine).DisableBoat,
	cloud.Upstash:      (*Engine).DisableUpstash,
	cloud.Vercel:       (*Engine).DisableVercel,
	cloud.Hetzner:      (*Engine).DisableHetzner,
	cloud.Bitqate:      (*Engine).DisableBitqate,
	cloud.Railway:      (*Engine).DisableRailway,
	cloud.Fly:          (*Engine).DisableFly,
	cloud.DigitalOcean: (*Engine).DisableDigitalOcean,
	cloud.Boxd:         (*Engine).DisableBoxd,
}
