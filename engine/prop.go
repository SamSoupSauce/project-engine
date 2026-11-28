package engine

import (
	"encoding/json"
	"fmt"
	"os"
)

// PoolMode controls how an option is selected from a Pool.
type PoolMode int

const (
	PoolModeFirst PoolMode = iota
	PoolModeExplicit
	PoolModeRandom
)

// OptionPayload carries the concrete data a selected option contributes.
type OptionPayload struct {
	Model    string
	Material string
	SubPools []*Pool
}

// PoolOption is one selectable entry in a Pool.
type PoolOption struct {
	ID      string
	Label   string
	Payload OptionPayload
}

// Pool is a named collection of selectable options.
type Pool struct {
	Name    string
	Mode    PoolMode
	Options []PoolOption
}

// Node is a node in a Prop's composition tree.
type Node struct {
	ID       string
	TypePool *Pool
	Children []*Node
}

// Prop is a compositional GameObject whose identity is a tree of nodes.
type Prop struct {
	ID   string
	Root *Node
}

// ResolvedLeaf is a concrete resolved node.
type ResolvedLeaf struct {
	NodeID   string
	Model    string
	Material string
}

// ResolvedProp is the fully resolved output of a Prop.
type ResolvedProp struct {
	PropID string
	Leaves []ResolvedLeaf
}

// ResolveOptions configures resolution.
type ResolveOptions struct {
	Selections map[string]string
	Seed       int64
}

// selectOptionFromPool picks an option for a node from its type pool.
func selectOptionFromPool(node *Node, opts ResolveOptions) *PoolOption {
	if node == nil || node.TypePool == nil || len(node.TypePool.Options) == 0 {
		return nil
	}
	selected := opts.Selections[node.ID]
	if selected != "" {
		for i := range node.TypePool.Options {
			if node.TypePool.Options[i].ID == selected {
				return &node.TypePool.Options[i]
			}
		}
	}
	if node.TypePool.Mode == PoolModeRandom && len(node.TypePool.Options) > 1 {
		idx := int(uint64(opts.Seed) % uint64(len(node.TypePool.Options)))
		return &node.TypePool.Options[idx]
	}
	return &node.TypePool.Options[0]
}

// resolveNode resolves a single node and its descendants into leaves.
func resolveNode(node *Node, opts ResolveOptions, result *ResolvedProp, seen map[*Node]bool) {
	if node == nil || seen[node] {
		return
	}
	seen[node] = true
	opt := selectOptionFromPool(node, opts)
	if opt != nil {
		model := opt.Payload.Model
		material := opt.Payload.Material
		for _, sub := range opt.Payload.SubPools {
			if subOpt := selectOptionFromPool(&Node{ID: sub.Name, TypePool: sub}, opts); subOpt != nil {
				model = subOpt.Payload.Model
				material = subOpt.Payload.Material
			}
		}
		if model != "" || material != "" {
			result.Leaves = append(result.Leaves, ResolvedLeaf{
				NodeID:   node.ID,
				Model:    model,
				Material: material,
			})
		}
	}
	for _, child := range node.Children {
		resolveNode(child, opts, result, seen)
	}
}

// Resolve walks a Prop's node tree and produces concrete leaves.
func Resolve(prop *Prop, opts ResolveOptions) (*ResolvedProp, error) {
	if prop == nil || prop.Root == nil {
		return nil, nil
	}
	result := &ResolvedProp{PropID: prop.ID}
	resolveNode(prop.Root, opts, result, map[*Node]bool{})
	return result, nil
}

// ValidateProp checks a Prop tree for structural errors and returns human-readable messages.
func ValidateProp(prop *Prop) []string {
	var errs []string
	if prop == nil {
		return []string{"prop is nil"}
	}
	seen := map[*Node]bool{}
	var visit func(*Node)
	visit = func(n *Node) {
		if n == nil {
			return
		}
		if seen[n] {
			errs = append(errs, fmt.Sprintf("cycle detected at node '%s'", n.ID))
			return
		}
		seen[n] = true
		if n.TypePool == nil || len(n.TypePool.Options) == 0 {
			name := ""
			if n.TypePool != nil {
				name = n.TypePool.Name
			}
			errs = append(errs, fmt.Sprintf("node '%s': type pool '%s' has no options", n.ID, name))
		}
		for _, child := range n.Children {
			visit(child)
		}
	}
	visit(prop.Root)
	return errs
}

// SavePropManifest writes a Prop tree to a JSON manifest file atomically.
func SavePropManifest(path string, prop *Prop) error {
	if prop == nil {
		return fmt.Errorf("prop is nil")
	}
	data, err := json.MarshalIndent(prop, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal prop: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write manifest temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit manifest: %w", err)
	}
	return nil
}

// LoadPropManifest reads a Prop tree from a JSON manifest file.
func LoadPropManifest(path string) (*Prop, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var prop Prop
	if err := json.Unmarshal(data, &prop); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &prop, nil
}
