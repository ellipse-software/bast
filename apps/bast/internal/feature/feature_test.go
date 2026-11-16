package feature

import "testing"

func TestFlagsAreOffUnlessSet(t *testing.T) {
	t.Setenv(EnvNoSync, "")
	t.Setenv(EnvNoVault, "")
	if SyncDisabled() || VaultDisabled() {
		t.Fatal("empty variables should leave the features on")
	}
	t.Setenv(EnvNoSync, "1")
	if !SyncDisabled() || VaultDisabled() {
		t.Fatal("BAST_NO_SYNC should disable only sync")
	}
	env, off := CommandDisabled("boat")
	if !off || env != EnvNoSync {
		t.Fatalf("boat: env=%q off=%v", env, off)
	}
	if _, off := CommandDisabled("hosts"); off {
		t.Fatal("hosts is not a sync command")
	}
	t.Setenv(EnvNoSync, "")
	t.Setenv(EnvNoVault, "1")
	env, off = CommandDisabled("vault")
	if !off || env != EnvNoVault {
		t.Fatalf("vault: env=%q off=%v", env, off)
	}
	if _, off := CommandDisabled("sync"); off {
		t.Fatal("sync should stay available when only vault is off")
	}
}
