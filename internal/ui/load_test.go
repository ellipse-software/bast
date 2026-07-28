package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bast/internal/openssh"
	"bast/internal/paths"
)

func TestLoadDiscoversBeforeEnrichingEveryHost(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	if err := os.MkdirAll(p.SSHDir, 0700); err != nil {
		t.Fatal(err)
	}
	var config strings.Builder
	for i := range 24 {
		fmt.Fprintf(&config, "Host host-%02d\n  HostName host-%02d.example\n  User tester\n  Port 22\n", i, i)
	}
	if err := os.WriteFile(p.MainConfig, []byte(config.String()), 0600); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	ssh := filepath.Join(bin, "ssh")
	sshScript := "#!/bin/sh\nif [ \"$1\" = \"-G\" ]; then printf 'hostname %s.example\\nuser tester\\nport 22\\n' \"$3\"; fi\n"
	if err := os.WriteFile(ssh, []byte(sshScript), 0700); err != nil {
		t.Fatal(err)
	}
	keygen := filepath.Join(bin, "ssh-keygen")
	if err := os.WriteFile(keygen, []byte("#!/bin/sh\nif [ \"$1\" = \"-F\" ]; then echo known; fi\n"), 0700); err != nil {
		t.Fatal(err)
	}
	sshAdd := filepath.Join(bin, "ssh-add")
	if err := os.WriteFile(sshAdd, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}

	app, err := New(p, openssh.Client{SSH: ssh, SSHKeygen: keygen, SSHAdd: sshAdd}, "dev")
	if err != nil {
		t.Fatal(err)
	}
	discovered, ok := app.loadCmd()().(discoveredMsg)
	if !ok || discovered.err != nil {
		t.Fatalf("discovery result = %#v", discovered)
	}
	if len(discovered.hosts) != 24 {
		t.Fatalf("discovered %d hosts", len(discovered.hosts))
	}
	for _, host := range discovered.hosts {
		if host.Resolved.HostName != host.Alias+".example" {
			t.Fatalf("discovery should parse HostName from config: %+v", host)
		}
		if host.KnownHost {
			t.Fatalf("discovery unexpectedly waited for known-host enrichment: %+v", host)
		}
	}

	msg, ok := app.enrichCmd(discovered.hosts)().(loadedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("enrichment result = %#v", msg)
	}
	for _, host := range msg.hosts {
		if host.Resolved.HostName != host.Alias+".example" || !host.KnownHost {
			t.Fatalf("host was not enriched: %+v", host)
		}
	}
}

func TestEnrichmentChecksSharedKnownHostEndpointOnce(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	if err := os.MkdirAll(p.SSHDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.MainConfig, []byte("Host first second\n  HostName shared.example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	ssh := filepath.Join(bin, "ssh")
	if err := os.WriteFile(ssh, []byte("#!/bin/sh\nprintf 'hostname shared.example\\nport 22\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	countPath := filepath.Join(home, "known-host-lookups")
	keygen := filepath.Join(bin, "ssh-keygen")
	keygenScript := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"-F\" ]; then printf x >> %q; echo known; fi\n", countPath)
	if err := os.WriteFile(keygen, []byte(keygenScript), 0700); err != nil {
		t.Fatal(err)
	}
	sshAdd := filepath.Join(bin, "ssh-add")
	if err := os.WriteFile(sshAdd, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}

	app, err := New(p, openssh.Client{SSH: ssh, SSHKeygen: keygen, SSHAdd: sshAdd}, "dev")
	if err != nil {
		t.Fatal(err)
	}
	discovered := app.loadCmd()().(discoveredMsg)
	loaded := app.enrichCmd(discovered.hosts)().(loadedMsg)
	if loaded.err != nil || len(loaded.hosts) != 2 || !loaded.hosts[0].KnownHost || !loaded.hosts[1].KnownHost {
		t.Fatalf("enrichment result = %#v", loaded)
	}
	lookups, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(lookups) != "x" {
		t.Fatalf("known-host lookups = %q, want one", lookups)
	}
}
