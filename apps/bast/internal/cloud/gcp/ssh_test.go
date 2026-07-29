package gcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bast/internal/cloud"
)

func TestAliasAndGroup(t *testing.T) {
	inst := cloud.Instance{
		Name: "web-01", ProjectID: "my-prod", ProjectName: "My Production",
		Zone: "us-central1-a", UseIAP: true,
	}
	if got := AliasFor(inst); got != "gcp_my-prod_web-01" {
		t.Fatalf("AliasFor = %q", got)
	}
	if got := GroupPath(inst); got != "Google Cloud/My Production" {
		t.Fatalf("GroupPath = %q", got)
	}
	used := map[string]bool{"gcp_my-prod_web-01": true}
	if got := UniqueAlias(AliasFor(inst), used); got != "gcp_my-prod_web-01_2" {
		t.Fatalf("UniqueAlias = %q", got)
	}
}

func TestToSyncHostPublicIP(t *testing.T) {
	inst := cloud.Instance{
		SyncID: "projects/p/zones/z/instances/web", Name: "web", ProjectID: "p",
		HostName: "1.2.3.4", User: "ubuntu", UseIAP: false,
		IdentityFile: "~/.ssh/bast/keys/IRIS", IdentitiesOnly: true,
	}
	block := ToSyncHost(inst, "gcp_p_web")
	if block.HostName != "1.2.3.4" || block.ProxyCommand != "" || block.User != "ubuntu" {
		t.Fatalf("unexpected block: %+v", block)
	}
	if !block.IdentitiesOnly || block.IdentityFile != "~/.ssh/bast/keys/IRIS" {
		t.Fatalf("identity = %+v", block)
	}
}

func TestToSyncHostSoftGcloudIdentity(t *testing.T) {
	inst := cloud.Instance{
		SyncID: "projects/p/zones/z/instances/web", Name: "web", ProjectID: "p",
		HostName: "1.2.3.4", User: "ubuntu",
		IdentityFile: gcloudIdentityFile, IdentitiesOnly: false,
	}
	block := ToSyncHost(inst, "gcp_p_web")
	if block.IdentitiesOnly || block.IdentityFile != gcloudIdentityFile {
		t.Fatalf("expected soft identity, got %+v", block)
	}
}

func TestToSyncHostIAP(t *testing.T) {
	inst := cloud.Instance{
		SyncID: "projects/p/zones/us-central1-a/instances/web", Name: "web",
		ProjectID: "p", Zone: "us-central1-a", UseIAP: true,
	}
	block := ToSyncHost(inst, "gcp_p_web")
	if block.HostName != "web" {
		t.Fatalf("HostName = %q", block.HostName)
	}
	if block.ProxyCommand == "" || block.SyncSource != "gcp" {
		t.Fatalf("unexpected block: %+v", block)
	}
	if !strings.Contains(block.ProxyCommand, "web %p") || strings.Contains(block.ProxyCommand, "%%p") {
		t.Fatalf("proxy port token = %q", block.ProxyCommand)
	}
}

func TestIAPProxyCommandUsesDiscoveryCredential(t *testing.T) {
	account := IAPProxyCommand(cloud.Instance{
		Name: "web", ProjectID: "p", Zone: "us-central1-a",
		CredentialAccount: "secondary@example.com",
	})
	if !strings.Contains(account, "--account=secondary@example.com") {
		t.Fatalf("account credential missing: %s", account)
	}

	serviceAccount := IAPProxyCommand(cloud.Instance{
		Name: "web", ProjectID: "p", Zone: "us-central1-a",
		CredentialFile: "/tmp/service account%prod.json",
	})
	if !strings.Contains(serviceAccount, "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE='/tmp/service account%%prod.json'") {
		t.Fatalf("service-account credential missing or unsafe: %s", serviceAccount)
	}
}

func TestResolveAuthMatchesLocalKeyUser(t *testing.T) {
	home := t.TempDir()
	keys := filepath.Join(home, ".ssh", "bast", "keys")
	if err := os.MkdirAll(keys, 0700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG9M+so2dj/OeGlcJcRbIvVQ76hrcdJjU2WC3x6wEjos ted@gcp\n"
	if err := os.WriteFile(filepath.Join(keys, "IRIS.pub"), []byte(pub), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keys, "IRIS"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	inst := cloud.Instance{
		User: "settings-user",
		SSHKeys: []cloud.SSHKey{
			{User: "ubuntu", PublicKey: strings.TrimSpace(pub)},
			{User: "other", PublicKey: "ssh-ed25519 AAAAother comment"},
		},
	}
	ResolveAuth(&inst, home, keys, "oslogin")
	if inst.User != "ubuntu" || !inst.IdentitiesOnly || inst.IdentityFile != "~/.ssh/bast/keys/IRIS" {
		t.Fatalf("auth = %+v", inst)
	}
}

func TestResolveAuthFallsBackToImageAndSoftKey(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "google_compute_engine"), []byte("k"), 0600); err != nil {
		t.Fatal(err)
	}
	inst := cloud.Instance{Image: "projects/ubuntu-os-cloud/global/images/ubuntu-2204"}
	ResolveAuth(&inst, home, "", "")
	if inst.User != "ubuntu" || inst.IdentitiesOnly || inst.IdentityFile != gcloudIdentityFile {
		t.Fatalf("auth = %+v", inst)
	}
}

func TestMergeSSHKeysInstanceWins(t *testing.T) {
	merged := mergeSSHKeys(
		"ubuntu:ssh-ed25519 AAAAinst comment\n",
		"ubuntu:ssh-ed25519 AAAAproj comment\ndeploy:ssh-ed25519 AAAAdep comment\n",
	)
	if len(merged) != 2 {
		t.Fatalf("merged = %+v", merged)
	}
	if merged[0].User != "ubuntu" || !strings.Contains(merged[0].PublicKey, "AAAAinst") {
		t.Fatalf("instance key should win: %+v", merged[0])
	}
	if merged[1].User != "deploy" {
		t.Fatalf("project-only user missing: %+v", merged)
	}
}

func TestImageSSHUser(t *testing.T) {
	cases := map[string]string{
		"ubuntu-2204-lts":            "ubuntu",
		"debian-12-bookworm":         "debian",
		"centos-7":                   "centos",
		"rhel-9":                     "cloud-user",
		"rocky-linux-9":              "cloud-user",
		"cos-105-lts":                "cloud-user",
		"fedora-cloud-39":            "fedora",
		"projects/x/global/images/y": "",
	}
	for image, want := range cases {
		if got := imageSSHUser(image); got != want {
			t.Fatalf("imageSSHUser(%q)=%q want %q", image, got, want)
		}
	}
}

func TestMapInstancePrefersPublicIP(t *testing.T) {
	inst := decodeInstance(t, `{
		"name":"web","status":"RUNNING","zone":"zones/us-central1-a",
		"selfLink":"https://www.googleapis.com/compute/v1/projects/p/zones/us-central1-a/instances/web",
		"networkInterfaces":[{"networkIP":"10.0.0.2","accessConfigs":[{"natIP":"8.8.8.8"}]}]
	}`)
	mapped := mapInstance(inst, project{ID: "p", Name: "Prod"}, "")
	if mapped.UseIAP || mapped.HostName != "8.8.8.8" {
		t.Fatalf("unexpected instance: %+v", mapped)
	}
	if mapped.SyncID != "projects/p/zones/us-central1-a/instances/web" {
		t.Fatalf("SyncID = %q", mapped.SyncID)
	}
}

func TestMapInstanceUsesIAPWithoutExternalIP(t *testing.T) {
	inst := decodeInstance(t, `{
		"name":"db","status":"RUNNING","zone":"zones/us-central1-a",
		"networkInterfaces":[{"networkIP":"10.0.0.9"}]
	}`)
	mapped := mapInstance(inst, project{ID: "p", Name: "Prod"}, "deploy")
	if !mapped.UseIAP || mapped.HostName != "db" || mapped.User != "deploy" {
		t.Fatalf("unexpected instance: %+v", mapped)
	}
}

func TestMapInstanceMergesProjectKeys(t *testing.T) {
	inst := decodeInstance(t, `{
		"name":"gpu","status":"RUNNING","zone":"zones/europe-west2-b",
		"networkInterfaces":[{"networkIP":"10.0.0.2","accessConfigs":[{"natIP":"1.2.3.4"}]}],
		"metadata":{"items":[{"key":"ssh-keys","value":"ubuntu:ssh-ed25519 AAAA comment"}]}
	}`)
	mapped := mapInstance(inst, project{
		ID: "p", Name: "P",
		SSHKeys: "deploy:ssh-ed25519 BBBB comment\n",
	}, "")
	if len(mapped.SSHKeys) != 2 {
		t.Fatalf("SSHKeys = %+v", mapped.SSHKeys)
	}
}

func TestMapInstanceHonorsEffectiveSSHMetadata(t *testing.T) {
	inst := decodeInstance(t, `{
		"name":"web","zone":"zones/us-central1-a",
		"metadata":{"items":[
			{"key":"enable-oslogin","value":"FALSE"},
			{"key":"block-project-ssh-keys","value":"TRUE"},
			{"key":"ssh-keys","value":"instance:ssh-ed25519 AAAAinstance"}
		]}
	}`)
	mapped := mapInstance(inst, project{
		ID: "p", SSHKeys: "project:ssh-ed25519 AAAAproject",
	}, "")
	if !mapped.BlockProjectSSHKeys || len(mapped.SSHKeys) != 1 || mapped.SSHKeys[0].User != "instance" {
		t.Fatalf("blocked project keys were merged: %+v", mapped)
	}

	osLogin := mapInstance(inst, project{ID: "p", EnableOSLogin: true}, "")
	if osLogin.OSLogin {
		t.Fatal("instance metadata should override project OS Login")
	}

	osLoginInst := decodeInstance(t, `{
		"name":"web","zone":"zones/us-central1-a",
		"metadata":{"items":[
			{"key":"enable-oslogin","value":"TRUE"},
			{"key":"ssh-keys","value":"instance:ssh-ed25519 AAAAinstance"}
		]}
	}`)
	osLogin = mapInstance(osLoginInst, project{ID: "p", SSHKeys: "project:ssh-ed25519 AAAAproject"}, "")
	if !osLogin.OSLogin || len(osLogin.SSHKeys) != 0 {
		t.Fatalf("OS Login should ignore metadata keys: %+v", osLogin)
	}
}

func TestIsWindowsInstance(t *testing.T) {
	inst := decodeInstance(t, `{
		"name":"win","status":"RUNNING","zone":"zones/z",
		"disks":[{"boot":true,"licenses":["https://www.googleapis.com/compute/v1/projects/windows-cloud/global/licenses/windows-server-2022-dc"]}]
	}`)
	if !isWindowsInstance(inst) {
		t.Fatal("expected windows")
	}
	linux := decodeInstance(t, `{
		"name":"web","status":"RUNNING","zone":"zones/z",
		"disks":[{"boot":true,"initializeParams":{"sourceImage":"projects/ubuntu-os-cloud/global/images/ubuntu-2204"}}]
	}`)
	if isWindowsInstance(linux) {
		t.Fatal("ubuntu should not be windows")
	}
}

func TestSkipInstanceStatus(t *testing.T) {
	if !skipInstanceStatus("TERMINATED") || !skipInstanceStatus("SUSPENDED") || skipInstanceStatus("RUNNING") {
		t.Fatal("status filter mismatch")
	}
}

func TestBootImage(t *testing.T) {
	inst := decodeInstance(t, `{
		"name":"web","disks":[
			{"boot":false,"source":"projects/p/zones/z/disks/data"},
			{"boot":true,"initializeParams":{"sourceImage":"projects/ubuntu-os-cloud/global/images/ubuntu-2204"}}
		]
	}`)
	if got := bootImage(inst); !strings.Contains(got, "ubuntu-2204") {
		t.Fatalf("bootImage = %q", got)
	}
}

func decodeInstance(t *testing.T, raw string) gceInstance {
	t.Helper()
	var inst gceInstance
	if err := json.Unmarshal([]byte(raw), &inst); err != nil {
		t.Fatal(err)
	}
	return inst
}
