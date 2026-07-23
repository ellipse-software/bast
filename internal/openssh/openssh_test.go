package openssh

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAndCommandValidation(t *testing.T) {
	dir := t.TempDir()
	ssh := filepath.Join(dir, "ssh")
	script := "#!/bin/sh\nif [ \"$1\" = \"-G\" ]; then printf 'hostname prod.example\\nuser deploy\\nport 2222\\nidentityfile ~/.ssh/id_test\\nidentitiesonly yes\\nproxyjump bastion\\n'; fi\n"
	if err := os.WriteFile(ssh, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	c := Client{SSH: ssh, SSHKeygen: ssh, SSHAdd: ssh}
	resolved, err := c.Resolve(context.Background(), "prod")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.HostName != "prod.example" || resolved.User != "deploy" || resolved.Port != "2222" || len(resolved.IdentityFiles) != 1 || resolved.ProxyJump != "bastion" {
		t.Fatalf("resolved = %+v", resolved)
	}
	if _, err := c.SSHCommand("-oProxyCommand=evil"); err == nil {
		t.Fatal("expected option-like alias rejection")
	}
}
