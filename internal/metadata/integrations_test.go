package metadata

import (
	"path/filepath"
	"testing"
)

func TestGCPIntegrationRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetGCP(GCPIntegration{
		Enabled: true, AutoSync: true, DefaultSSHUser: "ubuntu",
		ProjectFilter: []string{"demo"}, ServiceAccounts: []string{"/tmp/sa.json"},
		LastInstanceCount: 3,
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	gcp := reopened.GCP()
	if !gcp.Enabled || !gcp.AutoSync || gcp.DefaultSSHUser != "ubuntu" || gcp.LastInstanceCount != 3 {
		t.Fatalf("gcp = %+v", gcp)
	}
	if len(gcp.ProjectFilter) != 1 || gcp.ProjectFilter[0] != "demo" {
		t.Fatalf("filter = %+v", gcp.ProjectFilter)
	}
}
