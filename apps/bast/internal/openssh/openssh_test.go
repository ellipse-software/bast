package openssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveAndCommandValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell fixture")
	}
	dir := t.TempDir()
	ssh := filepath.Join(dir, "ssh")
	script := "#!/bin/sh\nif [ \"$1\" = \"-G\" ]; then printf 'hostname prod.example\\nuser deploy\\nport 2222\\nidentityfile ~/.ssh/id_test\\nidentitiesonly yes\\npubkeyauthentication no\\npasswordauthentication yes\\npreferredauthentications keyboard-interactive,password\\nproxyjump bastion\\n'; fi\n"
	if err := os.WriteFile(ssh, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	c := Client{SSH: ssh, SSHKeygen: ssh, SSHAdd: ssh}
	resolved, err := c.Resolve(context.Background(), "prod")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.HostName != "prod.example" || resolved.User != "deploy" || resolved.Port != "2222" || len(resolved.IdentityFiles) != 1 || resolved.PubkeyAuthentication != "no" || resolved.PasswordAuthentication != "yes" || resolved.ProxyJump != "bastion" {
		t.Fatalf("resolved = %+v", resolved)
	}
	if _, err := c.SSHCommand("-oProxyCommand=evil"); err == nil {
		t.Fatal("expected option-like alias rejection")
	}
	cmd, err := c.SFTPCommand("prod")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cmd.Args[1:], " "); got != "-o BatchMode=yes -s -- prod sftp" {
		t.Fatalf("SFTPCommand args = %q", got)
	}
	inDir, err := c.SSHCommandInDir("prod", "/var/app")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(inDir.Args[1:], " ")
	if !strings.Contains(args, "-t") || !strings.Contains(args, "RemoteCommand=none") || !strings.Contains(args, "cd /var/app") {
		t.Fatalf("SSHCommandInDir args = %q", args)
	}
	if _, err := c.SSHCommandInDir("prod", "bad\npath"); err == nil {
		t.Fatal("expected newline path rejection")
	}
	if _, err := c.Resolve(context.Background(), "prod\nHost evil"); err == nil {
		t.Fatal("expected newline alias rejection")
	}
	if err := c.RemoveKnownHost(context.Background(), "", "22"); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected empty-host rejection, got %v", err)
	}
}

func TestInstallPublicKeyCommandUsesStdinAndAuthorizedKeys(t *testing.T) {
	c := Default()
	public := "ssh-ed25519 AAAA-test workstation"
	cmd, err := c.InstallPublicKeyCommand("prod", public)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.Args) != 4 || cmd.Args[1] != "--" || cmd.Args[2] != "prod" {
		t.Fatalf("command args = %q", cmd.Args)
	}
	if !strings.Contains(cmd.Args[3], ".ssh/authorized_keys") || strings.Contains(cmd.Args[3], public) {
		t.Fatalf("remote command is unsafe or incomplete: %q", cmd.Args[3])
	}
	stdin, err := io.ReadAll(cmd.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	if string(stdin) != public+"\n" {
		t.Fatalf("stdin = %q", stdin)
	}
	if _, err := c.InstallPublicKeyCommand("prod", public+"\nsecond line"); err == nil {
		t.Fatal("multiline public key was accepted")
	}
}

func TestInstallPublicKeyCommandAppendsOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell fixture")
	}
	dir := t.TempDir()
	ssh := filepath.Join(dir, "ssh")
	script := "#!/bin/sh\nshift 2\nexec /bin/sh -c \"$1\"\n"
	if err := os.WriteFile(ssh, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	remoteHome := filepath.Join(dir, "remote-home")
	if err := os.MkdirAll(remoteHome, 0700); err != nil {
		t.Fatal(err)
	}
	c := Client{SSH: ssh}
	public := "ssh-ed25519 AAAA-test workstation"
	for range 2 {
		cmd, err := c.InstallPublicKeyCommand("prod", public)
		if err != nil {
			t.Fatal(err)
		}
		cmd.Env = append(os.Environ(), "HOME="+remoteHome)
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	}
	authorized, err := os.ReadFile(filepath.Join(remoteHome, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(authorized), public) != 1 {
		t.Fatalf("authorized_keys = %q", authorized)
	}
}

func TestFormatError(t *testing.T) {
	if got := FormatError(nil); got != "" {
		t.Fatalf("nil = %q", got)
	}
	if got := FormatError(errors.New("connection lost")); got != "connection lost" {
		t.Fatalf("plain error = %q", got)
	}

	cases := []struct {
		code int
		want string
	}{
		{1, "command failed"},
		{2, "invalid arguments or misuse"},
		{7, "exited with status 7"},
		{126, "command not executable"},
		{127, "command not found"},
		{130, "interrupted (Ctrl-C)"},
		{137, "killed"},
		{143, "terminated"},
		{140, "terminated by signal 12"},
		{255, "connection failed, refused, or interrupted"},
	}
	for _, tc := range cases {
		cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("exit %d", tc.code))
		err := cmd.Run()
		if got := FormatError(err); got != tc.want {
			t.Fatalf("exit %d = %q, want %q", tc.code, got, tc.want)
		}
	}
}
