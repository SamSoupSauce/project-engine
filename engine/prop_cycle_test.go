package engine

import "testing"

// TestValidateProp_RejectsCycles verifies that validation detects a child node
// that points back to an ancestor instead of recursing forever.
func TestValidateProp_RejectsCycles(t *testing.T) {
	root := &Node{
		ID:       "root",
		TypePool: &Pool{Name: "type", Options: []PoolOption{{ID: "mug"}}},
	}
	child := &Node{
		ID:       "child",
		TypePool: &Pool{Name: "style", Options: []PoolOption{{ID: "ceramic"}}},
	}
	root.Children = []*Node{child}
	child.Children = []*Node{root} // cycle

	errs := ValidateProp(&Prop{ID: "mug", Root: root})
	foundCycle := false
	for _, e := range errs {
		if len(e) >= 5 && e[:5] == "cycle" {
			foundCycle = true
		}
	}
	if !foundCycle {
		t.Fatalf("expected cycle error, got %v", errs)
	}
}
