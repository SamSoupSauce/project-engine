package luabridge

import (
	"path/filepath"
	"testing"

	"github.com/SamSoupSauce/project-engine/engine"
)

var createTestProp = engine.Prop{
	ID: "mug",
	Root: &engine.Node{
		ID: "root",
		TypePool: &engine.Pool{
			Name: "type", Mode: engine.PoolModeExplicit,
			Options: []engine.PoolOption{{
				ID: "ceramic", Label: "Ceramic",
				Payload: engine.OptionPayload{
					SubPools: []*engine.Pool{{
						Name: "style", Mode: engine.PoolModeExplicit,
						Options: []engine.PoolOption{
							{ID: "classic", Payload: engine.OptionPayload{Model: "mug_classic.gltf"}},
							{ID: "tall", Payload: engine.OptionPayload{Model: "mug_tall.gltf"}},
						},
					}},
				},
			}},
		},
	},
}

var badTestProp = engine.Prop{
	ID: "bad",
	Root: &engine.Node{
		ID:       "root",
		TypePool: &engine.Pool{Name: "type", Options: []engine.PoolOption{}},
	},
}

func writeTestPropManifest(t *testing.T, path string, prop *engine.Prop) {
	t.Helper()
	if err := engine.SavePropManifest(path, prop); err != nil {
		t.Fatalf("write prop manifest %s: %v", filepath.Base(path), err)
	}
}
