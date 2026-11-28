package engine

import "testing"

func testPropTree() *Prop {
	return &Prop{
		ID: "mug",
		Root: &Node{
			ID: "root",
			TypePool: &Pool{Name: "type", Options: []PoolOption{{ID: "mug"}}},
			Children: []*Node{
				{
					ID: "body",
					TypePool: &Pool{Name: "style", Options: []PoolOption{{ID: "ceramic", Payload: OptionPayload{Model: "ceramic"}}}},
					Children: []*Node{
						{ID: "handle", TypePool: &Pool{Name: "style", Options: []PoolOption{{ID: "loop"}}}},
					},
				},
			},
		},
	}
}

func TestFindNode_AbsoluteAndRelativePaths(t *testing.T) {
	prop := testPropTree()
	for _, path := range []string{"root/body/handle", "body/handle", "handle"} {
		node, err := FindNode(prop.Root, path)
		if err != nil {
			t.Fatalf("FindNode(%q): %v", path, err)
		}
		if node.ID != "handle" {
			t.Fatalf("FindNode(%q) got %q, want handle", path, node.ID)
		}
	}
}

func TestFindNode_UnknownPathErrors(t *testing.T) {
	prop := testPropTree()
	if _, err := FindNode(prop.Root, "root/nope"); err == nil {
		t.Fatal("expected error for unknown path")
	}
}
