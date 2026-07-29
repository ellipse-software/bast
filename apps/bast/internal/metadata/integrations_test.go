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
