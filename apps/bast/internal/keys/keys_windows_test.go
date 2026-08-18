//go:build windows

package keys

import "testing"

func TestWithinUsesWindowsPathIdentity(t *testing.T) {
	if !within(`c:\users\ted\.ssh\bast\keys\work`, `C:\Users\Ted\.ssh\bast\keys`) {
		t.Fatal("managed key should remain within its directory across path casing")
	}
	if within(`C:\Users\Ted\.ssh\other`, `C:\Users\Ted\.ssh\bast\keys`) {
		t.Fatal("external key was treated as managed")
	}
}
