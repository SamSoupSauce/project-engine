package engine

import "testing"

func TestSetAndRemoveOption(t *testing.T) {
	pool := &Pool{Name: "style", Options: []PoolOption{{ID: "classic", Payload: OptionPayload{Model: "classic.gltf"}}}}
	if err := SetOption(pool, "tall", "Tall", OptionPayload{Model: "tall.gltf"}); err != nil {
		t.Fatalf("SetOption: %v", err)
	}
	if len(pool.Options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(pool.Options))
	}
	if err := SetOption(pool, "classic", "Classic Pro", OptionPayload{Model: "classic2.gltf"}); err != nil {
		t.Fatalf("SetOption replace: %v", err)
	}
	if len(pool.Options) != 2 || pool.Options[0].Label != "Classic Pro" {
		t.Fatalf("expected replaced option, got %+v", pool.Options)
	}
	if err := RemoveOption(pool, "tall"); err != nil {
		t.Fatalf("RemoveOption: %v", err)
	}
	if len(pool.Options) != 1 || pool.Options[0].ID != "classic" {
		t.Fatalf("expected only classic, got %+v", pool.Options)
	}
	if err := RemoveOption(pool, "missing"); err == nil {
		t.Fatal("expected error removing unknown option")
	}
}
