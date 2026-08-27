package hetzner

import (
	"context"
	"testing"
)

func TestDetectSSHPort(t *testing.T) {
	prev := probeSSHPort
	t.Cleanup(func() { probeSSHPort = prev })

	probeSSHPort = func(_ context.Context, _ string, port int) probeResult {
		switch port {
		case 22:
			return probeRefused
		case 2022:
			return probeSSHBanner
		default:
			return probeUnknown
		}
	}
	if got := detectSSHPort(context.Background(), "203.0.113.9", ""); got != "2022" {
		t.Fatalf("refused 22 = %q", got)
	}

	probeSSHPort = func(_ context.Context, _ string, port int) probeResult {
		if port == 22 {
			return probeSSHBanner
		}
		return probeRefused
	}
	if got := detectSSHPort(context.Background(), "203.0.113.9", ""); got != "" {
		t.Fatalf("open 22 = %q want empty", got)
	}

	probeSSHPort = func(_ context.Context, _ string, port int) probeResult {
		switch port {
		case 2022:
			return probeRefused
		case 22:
			return probeSSHBanner
		default:
			return probeUnknown
		}
	}
	if got := detectSSHPort(context.Background(), "203.0.113.9", "2022"); got != "" {
		t.Fatalf("default 2022 but 22 open = %q", got)
	}

	probeSSHPort = func(_ context.Context, _ string, port int) probeResult {
		switch port {
		case 22:
			return probeOther
		case 2022:
			return probeSSHBanner
		default:
			return probeUnknown
		}
	}
	if got := detectSSHPort(context.Background(), "203.0.113.9", ""); got != "2022" {
		t.Fatalf("non-SSH banner on 22 = %q", got)
	}
}

func TestLabeledAndConfiguredSSHPort(t *testing.T) {
	if got := labeledSSHPort(map[string]string{"ssh-port": "2022"}); got != "2022" {
		t.Fatalf("label = %q", got)
	}
	if got := labeledSSHPort(map[string]string{"ssh_port": "22"}); got != "" {
		t.Fatalf("label 22 = %q", got)
	}
	if got := configuredSSHPort("2022"); got != "2022" {
		t.Fatalf("configured = %q", got)
	}
	if got := configuredSSHPort("22"); got != "" {
		t.Fatalf("configured 22 = %q", got)
	}
}

func TestAliasForOmitsEmptyLocation(t *testing.T) {
	if got := AliasFor(Instance{Context: "ted.ac", Name: "vpn"}); got != "hetzner_ted_ac_vpn" {
		t.Fatalf("empty location = %s", got)
	}
	if got := AliasFor(Instance{Context: "ted.ac", Location: "nbg1", Name: "vpn"}); got != "hetzner_ted_ac_nbg1_vpn" {
		t.Fatalf("location = %s", got)
	}
	if got := GroupPath(Instance{Context: "ted.ac", Name: "vpn"}); got != "Hetzner Cloud/ted.ac" {
		t.Fatalf("group = %s", got)
	}
}
