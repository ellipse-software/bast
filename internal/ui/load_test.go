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

func TestLoadEnrichesEveryHost(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	if err := os.MkdirAll(p.SSHDir, 0700); err != nil {
		t.Fatal(err)
	}
	var config strings.Builder
	for i := range 24 {
		fmt.Fprintf(&config, "Host host-%02d\n  HostName host-%02d.example\n", i, i)
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

	app, err := New(p, openssh.Client{SSH: ssh, SSHKeygen: keygen, SSHAdd: sshAdd})
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := app.loadCmd()().(loadedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("load result = %#v", msg)
	}
	if len(msg.hosts) != 24 {
		t.Fatalf("loaded %d hosts", len(msg.hosts))
	}
	for _, host := range msg.hosts {
		if host.Resolved.HostName != host.Alias+".example" || !host.KnownHost {
			t.Fatalf("host was not enriched: %+v", host)
		}
	}
}
