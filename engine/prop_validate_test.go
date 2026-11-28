package engine

import "testing"

// TestValidateProp_RejectsEmptyPool verifies that validation reports an error
// for a node whose type pool has no options.
func TestValidateProp_RejectsEmptyPool(t *testing.T) {
	prop := &Prop{
		ID: "mug",
		Root: &Node{
			ID:       "root",
			TypePool: &Pool{Name: "type", Options: []PoolOption{}},
		},
	}
	errs := ValidateProp(prop)
	if len(errs) == 0 {
		t.Fatal("expected validation error for empty pool")
	}
	found := false
	for _, e := range errs {
		if e == "node 'root': type pool 'type' has no options" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected empty-pool error, got %v", errs)
	}
}
