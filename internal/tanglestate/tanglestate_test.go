package tanglestate

import "testing"

func TestKeyIsWordingIndependent(t *testing.T) {
	a := Key("collision", []string{"Bob", "Alice"})
	b := Key("collision", []string{" alice ", "bob", "Alice"}) // reorder, case, dupes, spaces
	if a != b {
		t.Errorf("same tangle should key the same: %q vs %q", a, b)
	}
	if a != "collision|alice+bob" {
		t.Errorf("unexpected key %q", a)
	}
}

func TestLoadSaveAddRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if set, err := Load(Muted, "room"); err != nil || len(set) != 0 {
		t.Fatalf("cold store should be empty: %v %v", set, err)
	}
	if err := Add(Muted, "room", "collision|alice+bob"); err != nil {
		t.Fatal(err)
	}
	if err := Add(Muted, "room", "collision|alice+bob"); err != nil { // idempotent
		t.Fatal(err)
	}
	set, err := Load(Muted, "room")
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != 1 || !set["collision|alice+bob"] {
		t.Errorf("round-trip mismatch: %v", set)
	}
	// Stores are separate by name and by room.
	if other, _ := Load(Escalated, "room"); len(other) != 0 {
		t.Errorf("escalated store should be independent of muted: %v", other)
	}
	if other, _ := Load(Muted, "other-room"); len(other) != 0 {
		t.Errorf("a different room should be independent: %v", other)
	}
}
