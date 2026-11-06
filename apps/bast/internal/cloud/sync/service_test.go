package sync

import (
	"testing"

	"bast/internal/cloud"
	"bast/internal/metadata"
	"bast/internal/paths"
)

func TestEveryDescriptorHasAService(t *testing.T) {
	dir := t.TempDir()
	store, err := metadata.Open(paths.ForHome(dir).StateFile)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(paths.ForHome(dir), store)
	for _, d := range cloud.Descriptors() {
		svc, ok := engine.Service(d.Kind)
		if !ok || svc == nil {
			t.Fatalf("missing service for %s", d.Kind)
		}
		if svc.Kind() != d.Kind {
			t.Fatalf("service kind %s, descriptor %s", svc.Kind(), d.Kind)
		}
	}
	aws, _ := engine.Service(cloud.AWS)
	if len(aws.Auth().Login) != 0 {
		t.Fatalf("AWS login = %v, want none", aws.Auth().Login)
	}
	upstash, _ := engine.Service(cloud.Upstash)
	if upstash.Auth().TokenURL != "https://console.upstash.com" {
		t.Fatalf("Upstash token URL = %q", upstash.Auth().TokenURL)
	}
}
