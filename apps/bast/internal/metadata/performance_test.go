package metadata

import (
	"fmt"
	"path/filepath"
	"testing"
)

func BenchmarkHostMetadataWrites100(b *testing.B) {
	b.Run("individual", func(b *testing.B) {
		store, err := Open(filepath.Join(b.TempDir(), "state.json"))
		if err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			for i := range 100 {
				alias := fmt.Sprintf("host-%03d", i)
				if err := store.SetHost(alias, Host{Group: "Production/Web"}); err != nil {
					b.Fatal(err)
				}
			}
		}
	})

	b.Run("batch", func(b *testing.B) {
		store, err := Open(filepath.Join(b.TempDir(), "state.json"))
		if err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			if err := store.UpdateHosts(func(hosts map[string]Host) {
				for i := range 100 {
					alias := fmt.Sprintf("host-%03d", i)
					hosts[alias] = Host{Group: "Production/Web"}
				}
			}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
