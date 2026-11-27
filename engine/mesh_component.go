package engine

import "reflect"

// MeshComponent attaches a loaded .gltf / .glb 3D mesh asset to a GameObject.
type MeshComponent struct {
	Owner    *GameObject
	FilePath string
	Asset    *GLTFAsset
	LoadErr  error
}

// NewMeshComponent creates a MeshComponent referencing filepath and loads the .gltf/.glb asset immediately if non-empty.
func NewMeshComponent(filepath string) *MeshComponent {
	mc := &MeshComponent{FilePath: filepath}
	if filepath != "" {
		mc.LoadErr = mc.Load(filepath)
	}
	return mc
}

// Init implements Component and ensures the referenced .gltf/.glb file is loaded if configured.
func (m *MeshComponent) Init(owner *GameObject) {
	m.Owner = owner
	if m.Asset == nil && m.FilePath != "" {
		m.LoadErr = m.Load(m.FilePath)
	}
}

// Update implements Component.
func (m *MeshComponent) Update(_ float32) {}

// Destroy implements Component.
func (m *MeshComponent) Destroy() {
	m.Asset = nil
	m.Owner = nil
}

// Load parses a .gltf or .glb file from disk and attaches the resulting GLTFAsset.
func (m *MeshComponent) Load(filepath string) error {
	m.FilePath = filepath
	if filepath == "" {
		m.Asset = nil
		m.LoadErr = nil
		return nil
	}
	asset, err := LoadGLTFAsset(filepath)
	m.LoadErr = err
	if err != nil {
		return err
	}
	m.Asset = asset
	return nil
}

// TriangleCount returns the total triangle count across all loaded glTF meshes.
func (m *MeshComponent) TriangleCount() int {
	if m == nil || m.Asset == nil {
		return 0
	}
	return m.Asset.TriangleCount()
}

// Bounds returns the local-space bounding box computed from the loaded glTF asset.
func (m *MeshComponent) Bounds() BoundingBox {
	if m == nil || m.Asset == nil {
		return BoundingBox{Min: NewVector3(-0.5, -0.5, -0.5), Max: NewVector3(0.5, 0.5, 0.5)}
	}
	return m.Asset.Bounds
}

var meshComponentType = reflect.TypeOf((*MeshComponent)(nil))

// GetMeshComponent returns the attached MeshComponent on this GameObject, or nil.
func (go_ *GameObject) GetMeshComponent() *MeshComponent {
	if go_ == nil {
		return nil
	}
	if comp := go_.GetComponent(meshComponentType); comp != nil {
		if mc, ok := comp.(*MeshComponent); ok {
			return mc
		}
	}
	return nil
}
