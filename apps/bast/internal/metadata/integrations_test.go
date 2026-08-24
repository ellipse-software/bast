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
	if len(gcp.ServiceAccounts) != 1 || gcp.ServiceAccounts[0] != "/tmp/sa.json" {
		t.Fatalf("service accounts = %+v", gcp.ServiceAccounts)
	}
}

func TestAWSIntegrationRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAWS(AWSIntegration{
		Enabled: true, AutoSync: true, DefaultSSHUser: "ec2-user",
		ProfileFilter: []string{"production"}, RegionFilter: []string{"eu-west-1"}, LastInstanceCount: 4,
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	aws := reopened.AWS()
	if !aws.Enabled || !aws.AutoSync || aws.DefaultSSHUser != "ec2-user" || aws.LastInstanceCount != 4 ||
		len(aws.ProfileFilter) != 1 || aws.ProfileFilter[0] != "production" || len(aws.RegionFilter) != 1 || aws.RegionFilter[0] != "eu-west-1" {
		t.Fatalf("aws = %+v", aws)
	}
}

func TestAzureIntegrationRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetAzure(AzureIntegration{
		Enabled: true, AutoSync: true, DefaultSSHUser: "azureuser",
		SubscriptionFilter: []string{"Production"}, ResourceGroupFilter: []string{"apps"}, LastInstanceCount: 5,
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	azure := reopened.Azure()
	if !azure.Enabled || !azure.AutoSync || azure.DefaultSSHUser != "azureuser" || azure.LastInstanceCount != 5 ||
		len(azure.SubscriptionFilter) != 1 || azure.SubscriptionFilter[0] != "Production" ||
		len(azure.ResourceGroupFilter) != 1 || azure.ResourceGroupFilter[0] != "apps" {
		t.Fatalf("azure = %+v", azure)
	}
}

func TestDigitalOceanIntegrationRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetDigitalOcean(DigitalOceanIntegration{
		Enabled: true, AutoSync: true, DefaultSSHUser: "root",
		ContextFilter: []string{"work"}, RegionFilter: []string{"nyc3"}, LastInstanceCount: 6,
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ocean := reopened.DigitalOcean()
	if !ocean.Enabled || !ocean.AutoSync || ocean.DefaultSSHUser != "root" || ocean.LastInstanceCount != 6 ||
		len(ocean.ContextFilter) != 1 || ocean.ContextFilter[0] != "work" ||
		len(ocean.RegionFilter) != 1 || ocean.RegionFilter[0] != "nyc3" {
		t.Fatalf("digitalocean = %+v", ocean)
	}
}

func TestBoxIntegrationRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBox(BoxIntegration{
		Enabled: true, AutoSync: true, Disabled: false, LastInstanceCount: 2,
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	box := reopened.Box()
	if !box.Enabled || !box.AutoSync || box.Disabled || box.LastInstanceCount != 2 {
		t.Fatalf("box = %+v", box)
	}
	if err := reopened.SetBox(BoxIntegration{Disabled: true}); err != nil {
		t.Fatal(err)
	}
	if !reopened.Box().Disabled || reopened.Box().Enabled {
		t.Fatalf("disabled box = %+v", reopened.Box())
	}
}

func TestUpstashIntegrationRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetUpstash(UpstashIntegration{
		Enabled: true, AutoSync: true, LastInstanceCount: 3,
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Upstash()
	if !got.Enabled || !got.AutoSync || got.LastInstanceCount != 3 {
		t.Fatalf("upstash = %+v", got)
	}
	if err := reopened.SetUpstash(UpstashIntegration{Disabled: true}); err != nil {
		t.Fatal(err)
	}
	if !reopened.Upstash().Disabled || reopened.Upstash().Enabled {
		t.Fatalf("disabled upstash = %+v", reopened.Upstash())
	}
}
