package cloud

import "testing"

func TestKindForGroup(t *testing.T) {
	tests := []struct {
		group string
		kind  Kind
		ok    bool
	}{
		{"Google Cloud", GCP, true},
		{"Google Cloud/demo", GCP, true},
		{"GCP", GCP, true},
		{"GCP/demo", GCP, true},
		{"Amazon EC2", AWS, true},
		{"Amazon EC2/default", AWS, true},
		{"AWS/default", AWS, true},
		{"Microsoft Azure", Azure, true},
		{"Microsoft Azure/Production/apps", Azure, true},
		{"Box", Box, true},
		{"Box/Running", Box, true},
		{"Upstash", Upstash, true},
		{"Upstash/dev", Upstash, true},
		{"Hetzner Cloud", Hetzner, true},
		{"Hetzner Cloud/prod/fsn1", Hetzner, true},
		{"Work", "", false},
		{"Boxing", "", false},
		{"", "", false},
	}
	for _, test := range tests {
		kind, ok := KindForGroup(test.group)
		if kind != test.kind || ok != test.ok {
			t.Fatalf("KindForGroup(%q) = %q, %t; want %q, %t", test.group, kind, ok, test.kind, test.ok)
		}
		if IsSyncedGroup(test.group) != test.ok {
			t.Fatalf("IsSyncedGroup(%q) = %t; want %t", test.group, !test.ok, test.ok)
		}
	}
}

func TestKindForSource(t *testing.T) {
	kind, ok := KindForSource("box")
	if !ok || kind != Box {
		t.Fatalf("KindForSource(box) = %q, %t", kind, ok)
	}
	kind, ok = KindForSource("upstash")
	if !ok || kind != Upstash {
		t.Fatalf("KindForSource(upstash) = %q, %t", kind, ok)
	}
	kind, ok = KindForSource("hetzner")
	if !ok || kind != Hetzner {
		t.Fatalf("KindForSource(hetzner) = %q, %t", kind, ok)
	}
	if _, ok := KindForSource("digitalocean"); ok {
		t.Fatal("unknown source should not match")
	}
}

func TestCapabilitiesForLifecycleProviders(t *testing.T) {
	box := CapabilitiesFor(Box)
	if !box.Create || !box.Stop || !box.Start || !box.Fork || box.Delete {
		t.Fatalf("box caps = %+v", box)
	}
	upstash := CapabilitiesFor(Upstash)
	if !upstash.Create || !upstash.Stop || !upstash.Start || !upstash.Fork || !upstash.Delete {
		t.Fatalf("upstash caps = %+v", upstash)
	}
	hetzner := CapabilitiesFor(Hetzner)
	if hetzner.Create || !hetzner.Stop || !hetzner.Start || !hetzner.Restart || hetzner.Fork || hetzner.Delete {
		t.Fatalf("hetzner caps = %+v", hetzner)
	}
	for _, kind := range []Kind{GCP, AWS, Azure} {
		if caps := CapabilitiesFor(kind); caps != (Capabilities{}) {
			t.Fatalf("%s caps = %+v; want empty", kind, caps)
		}
	}
}

func TestIsProviderRoot(t *testing.T) {
	if !IsProviderRoot("Box") || !IsProviderRoot("Google Cloud") || !IsProviderRoot("GCP") || !IsProviderRoot("Hetzner Cloud") {
		t.Fatal("expected roots to match")
	}
	if IsProviderRoot("Google Cloud/demo") || IsProviderRoot("Box/Running") || IsProviderRoot("Work") {
		t.Fatal("subgroups and user groups are not provider roots")
	}
}

func TestDescriptorsCoverEveryKind(t *testing.T) {
	seen := map[Kind]bool{}
	for _, d := range Descriptors() {
		if d.Kind == "" || d.Title == "" || d.GroupRoot == "" || d.BrandColor == "" || d.NerdIcon == "" {
			t.Fatalf("incomplete descriptor: %+v", d)
		}
		if _, ok := DescriptorForKind(d.Kind); !ok {
			t.Fatalf("DescriptorForKind missing %s", d.Kind)
		}
		seen[d.Kind] = true
	}
	for _, kind := range []Kind{GCP, AWS, Azure, Box, Upstash, Hetzner} {
		if !seen[kind] {
			t.Fatalf("missing descriptor for %s", kind)
		}
	}
}
