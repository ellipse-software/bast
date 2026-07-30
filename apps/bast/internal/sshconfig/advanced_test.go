package sshconfig

import "testing"

func TestParseAdvancedRoundTrip(t *testing.T) {
	extras := []string{
		"ForwardAgent yes",
		"RemoteCommand tmux attach",
		"SetEnv FOO=bar",
		"LocalForward 8080 localhost:80",
		"RemoteForward 9090 localhost:9090",
		"DynamicForward 1080",
		"ServerAliveInterval 30",
		"Compression yes",
		"IdentitiesOnly yes",
	}
	settings := ParseAdvanced(extras, "bastion")
	if settings.ProxyJump != "bastion" || settings.ForwardAgent != "yes" || settings.RemoteCommand != "tmux attach" {
		t.Fatalf("settings = %+v", settings)
	}
	if len(settings.SetEnv) != 1 || len(settings.LocalForwards) != 1 || len(settings.RemoteForwards) != 1 {
		t.Fatalf("forwards/env = %+v", settings)
	}
	if len(settings.Custom) != 1 || settings.Custom[0] != "IdentitiesOnly yes" {
		t.Fatalf("custom = %+v", settings.Custom)
	}
	out := settings.ExtraOptions()
	if len(out) != len(extras) {
		t.Fatalf("ExtraOptions() = %#v", out)
	}
}

func TestValidateAdvancedAllowsRemoteCommand(t *testing.T) {
	if err := ValidateAdvanced(AdvancedSettings{RemoteCommand: "tmux attach -t main"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAdvanced(AdvancedSettings{RemoteCommand: "bad\ncommand"}); err == nil {
		t.Fatal("expected newline rejection")
	}
}
