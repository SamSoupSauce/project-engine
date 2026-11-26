package engine

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	glbMagic     uint32 = 0x46546C67 // "glTF"
	glbChunkJSON uint32 = 0x4E4F534A // "JSON"
	glbChunkBIN  uint32 = 0x004E4942 // "BIN\0"

	gltfTypeFloat  = 5126
	gltfTypeUByte  = 5121
	gltfTypeUShort = 5123
	gltfTypeUInt   = 5125
)

// GLTFMaterial holds PBR parameters parsed from a glTF 2.0 material definition.
type GLTFMaterial struct {
	Name            string
	BaseColorFactor [4]float32
	MetallicFactor  float32
	RoughnessFactor float32
	BaseColorTexURI string
}

// GLTFPrimitive stores decoded triangle geometry and material reference for a sub-mesh.
type GLTFPrimitive struct {
	Positions     []Vector3
	Normals       []Vector3
	TexCoords     []Vector2
	Indices       []uint32
	MaterialIndex int
}

// GLTFMesh represents a named mesh composed of one or more primitives.
type GLTFMesh struct {
	Name       string
	Primitives []GLTFPrimitive
}

// GLTFNode represents a node in the glTF scene graph usable as a socket or bone attachment point.
type GLTFNode struct {
	Name      string
	MeshIndex int
	Transform Transform
}

// GLTFAsset is a parsed glTF 2.0 / GLB model with bounding box and Jolt convex hull vertices.
type GLTFAsset struct {
	FilePath   string
	Version    string
	Materials  []GLTFMaterial
	Meshes     []GLTFMesh
	Nodes      []GLTFNode
	Bounds     BoundingBox
	HullPoints []Vector3
}

// TriangleCount returns the total triangle count across all meshes/primitives.
func (a *GLTFAsset) TriangleCount() int {
	if a == nil {
		return 0
	}
	total := 0
	for _, m := range a.Meshes {
		for _, p := range m.Primitives {
			if len(p.Indices) > 0 {
				total += len(p.Indices) / 3
			} else {
				total += len(p.Positions) / 3
			}
		}
	}
	return total
}

// LoadGLTFAsset reads and parses a .gltf or .glb file from disk.
func LoadGLTFAsset(filepathStr string) (*GLTFAsset, error) {
	data, err := os.ReadFile(filepathStr)
	if err != nil {
		return nil, fmt.Errorf("gltf: read %q: %w", filepathStr, err)
	}
	asset, err := ParseGLTFBytes(data, filepath.Dir(filepathStr))
	if err != nil {
		return nil, fmt.Errorf("gltf: parse %q: %w", filepathStr, err)
	}
	asset.FilePath = filepathStr
	return asset, nil
}

// ParseGLTFBytes parses either binary .glb (version 2) or UTF-8 JSON .gltf payloads.
func ParseGLTFBytes(data []byte, baseDir string) (*GLTFAsset, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("gltf payload too small")
	}
	var jsonBytes []byte
	var binChunk []byte

	if len(data) >= 12 && binary.LittleEndian.Uint32(data[0:4]) == glbMagic {
		version := binary.LittleEndian.Uint32(data[4:8])
		if version != 2 {
			return nil, fmt.Errorf("unsupported GLB version %d", version)
		}
		totalLen := binary.LittleEndian.Uint32(data[8:12])
		if int(totalLen) > len(data) {
			totalLen = uint32(len(data))
		}
		offset := uint32(12)
		for offset+8 <= totalLen {
			chunkLen := binary.LittleEndian.Uint32(data[offset : offset+4])
			chunkType := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
			offset += 8
			if offset+chunkLen > totalLen {
				return nil, fmt.Errorf("truncated GLB chunk")
			}
			payload := data[offset : offset+chunkLen]
			switch chunkType {
			case glbChunkJSON:
				jsonBytes = payload
			case glbChunkBIN:
				binChunk = payload
			}
			offset += chunkLen
		}
		if len(jsonBytes) == 0 {
			return nil, fmt.Errorf("GLB missing JSON chunk")
		}
	} else {
		jsonBytes = data
	}

	var doc gltfDoc
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, fmt.Errorf("invalid glTF JSON: %w", err)
	}
	if doc.Asset.Version != "" && !strings.HasPrefix(doc.Asset.Version, "2.") {
		return nil, fmt.Errorf("unsupported glTF version %q", doc.Asset.Version)
	}

	buffers, err := resolveGLTFBuffers(doc.Buffers, binChunk, baseDir)
	if err != nil {
		return nil, err
	}

	out := &GLTFAsset{
		Version:   doc.Asset.Version,
		Materials: make([]GLTFMaterial, len(doc.Materials)),
		Meshes:    make([]GLTFMesh, 0, len(doc.Meshes)),
		Nodes:     make([]GLTFNode, 0, len(doc.Nodes)),
	}
	if out.Version == "" {
		out.Version = "2.0"
	}

	for i, m := range doc.Materials {
		baseCol := [4]float32{1, 1, 1, 1}
		if len(m.PBRMetallicRoughness.BaseColorFactor) == 4 {
			for k := 0; k < 4; k++ {
				baseCol[k] = m.PBRMetallicRoughness.BaseColorFactor[k]
			}
		}
		texURI := ""
		if m.PBRMetallicRoughness.BaseColorTexture != nil {
			texIdx := m.PBRMetallicRoughness.BaseColorTexture.Index
			if texIdx >= 0 && texIdx < len(doc.Textures) {
				imgIdx := doc.Textures[texIdx].Source
				if imgIdx >= 0 && imgIdx < len(doc.Images) {
					texURI = doc.Images[imgIdx].URI
				}
			}
		}
		out.Materials[i] = GLTFMaterial{
			Name:            m.Name,
			BaseColorFactor: baseCol,
			MetallicFactor:  m.PBRMetallicRoughness.MetallicFactor,
			RoughnessFactor: m.PBRMetallicRoughness.RoughnessFactor,
			BaseColorTexURI: texURI,
		}
	}

	for _, n := range doc.Nodes {
		tr := NewTransform()
		if len(n.Translation) == 3 {
			tr.Position = NewVector3(n.Translation[0], n.Translation[1], n.Translation[2])
		}
		if len(n.Scale) == 3 {
			tr.Scale = NewVector3(n.Scale[0], n.Scale[1], n.Scale[2])
		}
		meshIdx := -1
		if n.Mesh != nil {
			meshIdx = *n.Mesh
		}
		out.Nodes = append(out.Nodes, GLTFNode{
			Name:      n.Name,
			MeshIndex: meshIdx,
			Transform: tr,
		})
	}

	minV := NewVector3(math.MaxFloat32, math.MaxFloat32, math.MaxFloat32)
	maxV := NewVector3(-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32)
	hasVerts := false
	hullSeen := make(map[[3]int32]struct{})

	for _, m := range doc.Meshes {
		gm := GLTFMesh{
			Name:       m.Name,
			Primitives: make([]GLTFPrimitive, 0, len(m.Primitives)),
		}
		for _, p := range m.Primitives {
			gp := GLTFPrimitive{MaterialIndex: -1}
			if p.Material != nil {
				gp.MaterialIndex = *p.Material
			}
			if posAcc, ok := p.Attributes["POSITION"]; ok {
				vecs, err := readVec3Accessor(&doc, buffers, posAcc)
				if err != nil {
					return nil, err
				}
				gp.Positions = vecs
				for _, v := range vecs {
					hasVerts = true
					if v.X < minV.X {
						minV.X = v.X
					}
					if v.Y < minV.Y {
						minV.Y = v.Y
					}
					if v.Z < minV.Z {
						minV.Z = v.Z
					}
					if v.X > maxV.X {
						maxV.X = v.X
					}
					if v.Y > maxV.Y {
						maxV.Y = v.Y
					}
					if v.Z > maxV.Z {
						maxV.Z = v.Z
					}
					key := [3]int32{int32(math.Round(float64(v.X * 1000))), int32(math.Round(float64(v.Y * 1000))), int32(math.Round(float64(v.Z * 1000)))}
					if _, exists := hullSeen[key]; !exists && len(out.HullPoints) < 128 {
						hullSeen[key] = struct{}{}
						out.HullPoints = append(out.HullPoints, v)
					}
				}
			}
			if normAcc, ok := p.Attributes["NORMAL"]; ok {
				norms, err := readVec3Accessor(&doc, buffers, normAcc)
				if err != nil {
					return nil, err
				}
				gp.Normals = norms
			}
			if uvAcc, ok := p.Attributes["TEXCOORD_0"]; ok {
				uvs, err := readVec2Accessor(&doc, buffers, uvAcc)
				if err != nil {
					return nil, err
				}
				gp.TexCoords = uvs
			}
			if p.Indices != nil {
				idxs, err := readScalarIndices(&doc, buffers, *p.Indices)
				if err != nil {
					return nil, err
				}
				gp.Indices = idxs
			}
			gm.Primitives = append(gm.Primitives, gp)
		}
		out.Meshes = append(out.Meshes, gm)
	}

	if hasVerts {
		out.Bounds = BoundingBox{Min: minV, Max: maxV}
	} else {
		out.Bounds = BoundingBox{Min: NewVector3(-0.5, -0.5, -0.5), Max: NewVector3(0.5, 0.5, 0.5)}
	}
	return out, nil
}

type gltfDoc struct {
	Asset struct {
		Version   string `json:"version"`
		Generator string `json:"generator,omitempty"`
	} `json:"asset"`
	Buffers []struct {
		URI        string `json:"uri,omitempty"`
		ByteLength int    `json:"byteLength"`
	} `json:"buffers"`
	BufferViews []struct {
		Buffer     int `json:"buffer"`
		ByteOffset int `json:"byteOffset"`
		ByteLength int `json:"byteLength"`
		ByteStride int `json:"byteStride,omitempty"`
	} `json:"bufferViews"`
	Accessors []struct {
		BufferView    *int      `json:"bufferView"`
		ByteOffset    int       `json:"byteOffset"`
		ComponentType int       `json:"componentType"`
		Count         int       `json:"count"`
		Type          string    `json:"type"`
		Min           []float32 `json:"min,omitempty"`
		Max           []float32 `json:"max,omitempty"`
	} `json:"accessors"`
	Materials []struct {
		Name                 string `json:"name,omitempty"`
		PBRMetallicRoughness struct {
			BaseColorFactor  []float32 `json:"baseColorFactor,omitempty"`
			MetallicFactor   float32   `json:"metallicFactor,omitempty"`
			RoughnessFactor  float32   `json:"roughnessFactor,omitempty"`
			BaseColorTexture *struct {
				Index int `json:"index"`
			} `json:"baseColorTexture,omitempty"`
		} `json:"pbrMetallicRoughness"`
	} `json:"materials"`
	Textures []struct {
		Source int `json:"source"`
	} `json:"textures"`
	Images []struct {
		URI string `json:"uri,omitempty"`
	} `json:"images"`
	Meshes []struct {
		Name       string `json:"name,omitempty"`
		Primitives []struct {
			Attributes map[string]int `json:"attributes"`
			Indices    *int           `json:"indices,omitempty"`
			Material   *int           `json:"material,omitempty"`
		} `json:"primitives"`
	} `json:"meshes"`
	Nodes []struct {
		Name        string    `json:"name,omitempty"`
		Mesh        *int      `json:"mesh,omitempty"`
		Translation []float32 `json:"translation,omitempty"`
		Scale       []float32 `json:"scale,omitempty"`
	} `json:"nodes"`
}

func resolveGLTFBuffers(defs []struct {
	URI        string `json:"uri,omitempty"`
	ByteLength int    `json:"byteLength"`
}, binChunk []byte, baseDir string) ([][]byte, error) {
	out := make([][]byte, len(defs))
	for i, b := range defs {
		if b.URI == "" {
			if i == 0 && len(binChunk) > 0 {
				out[i] = binChunk
				continue
			}
			return nil, fmt.Errorf("gltf buffer %d missing URI and BIN chunk", i)
		}
		if strings.HasPrefix(b.URI, "data:") {
			idx := strings.Index(b.URI, ",")
			if idx < 0 {
				return nil, fmt.Errorf("invalid data URI in gltf buffer %d", i)
			}
			decoded, err := base64.StdEncoding.DecodeString(b.URI[idx+1:])
			if err != nil {
				return nil, fmt.Errorf("decode base64 buffer %d: %w", i, err)
			}
			out[i] = decoded
			continue
		}
		resolved := filepath.Join(baseDir, b.URI)
		raw, err := os.ReadFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("read external buffer %q: %w", resolved, err)
		}
		out[i] = raw
	}
	return out, nil
}

func accessorSlice(doc *gltfDoc, buffers [][]byte, accIdx int, elemBytes int) ([]byte, int, int, error) {
	if accIdx < 0 || accIdx >= len(doc.Accessors) {
		return nil, 0, 0, fmt.Errorf("accessor index %d out of range", accIdx)
	}
	acc := doc.Accessors[accIdx]
	if acc.BufferView == nil {
		return nil, 0, 0, fmt.Errorf("accessor %d missing bufferView", accIdx)
	}
	bvIdx := *acc.BufferView
	if bvIdx < 0 || bvIdx >= len(doc.BufferViews) {
		return nil, 0, 0, fmt.Errorf("bufferView %d out of range", bvIdx)
	}
	bv := doc.BufferViews[bvIdx]
	if bv.Buffer < 0 || bv.Buffer >= len(buffers) {
		return nil, 0, 0, fmt.Errorf("buffer %d out of range", bv.Buffer)
	}
	stride := bv.ByteStride
	if stride == 0 {
		stride = elemBytes
	}
	start := bv.ByteOffset + acc.ByteOffset
	if start < 0 || start+stride*(acc.Count-1)+elemBytes > len(buffers[bv.Buffer]) && acc.Count > 0 {
		return nil, 0, 0, fmt.Errorf("accessor %d overflows buffer %d", accIdx, bv.Buffer)
	}
	return buffers[bv.Buffer][start:], stride, acc.Count, nil
}

func readVec3Accessor(doc *gltfDoc, buffers [][]byte, accIdx int) ([]Vector3, error) {
	if doc.Accessors[accIdx].ComponentType != gltfTypeFloat || doc.Accessors[accIdx].Type != "VEC3" {
		return nil, fmt.Errorf("accessor %d expected float VEC3", accIdx)
	}
	raw, stride, count, err := accessorSlice(doc, buffers, accIdx, 12)
	if err != nil {
		return nil, err
	}
	res := make([]Vector3, count)
	for i := 0; i < count; i++ {
		off := i * stride
		res[i] = Vector3{
			X: math.Float32frombits(binary.LittleEndian.Uint32(raw[off : off+4])),
			Y: math.Float32frombits(binary.LittleEndian.Uint32(raw[off+4 : off+8])),
			Z: math.Float32frombits(binary.LittleEndian.Uint32(raw[off+8 : off+12])),
		}
	}
	return res, nil
}

func readVec2Accessor(doc *gltfDoc, buffers [][]byte, accIdx int) ([]Vector2, error) {
	if doc.Accessors[accIdx].ComponentType != gltfTypeFloat || doc.Accessors[accIdx].Type != "VEC2" {
		return nil, fmt.Errorf("accessor %d expected float VEC2", accIdx)
	}
	raw, stride, count, err := accessorSlice(doc, buffers, accIdx, 8)
	if err != nil {
		return nil, err
	}
	res := make([]Vector2, count)
	for i := 0; i < count; i++ {
		off := i * stride
		res[i] = Vector2{
			X: math.Float32frombits(binary.LittleEndian.Uint32(raw[off : off+4])),
			Y: math.Float32frombits(binary.LittleEndian.Uint32(raw[off+4 : off+8])),
		}
	}
	return res, nil
}

func readScalarIndices(doc *gltfDoc, buffers [][]byte, accIdx int) ([]uint32, error) {
	if accIdx < 0 || accIdx >= len(doc.Accessors) {
		return nil, fmt.Errorf("index accessor %d out of range", accIdx)
	}
	acc := doc.Accessors[accIdx]
	elemSize := 2
	switch acc.ComponentType {
	case gltfTypeUByte:
		elemSize = 1
	case gltfTypeUShort:
		elemSize = 2
	case gltfTypeUInt:
		elemSize = 4
	default:
		return nil, fmt.Errorf("unsupported index componentType %d", acc.ComponentType)
	}
	raw, stride, count, err := accessorSlice(doc, buffers, accIdx, elemSize)
	if err != nil {
		return nil, err
	}
	res := make([]uint32, count)
	for i := 0; i < count; i++ {
		off := i * stride
		switch acc.ComponentType {
		case gltfTypeUByte:
			res[i] = uint32(raw[off])
		case gltfTypeUShort:
			res[i] = uint32(binary.LittleEndian.Uint16(raw[off : off+2]))
		case gltfTypeUInt:
			res[i] = binary.LittleEndian.Uint32(raw[off : off+4])
		}
	}
	return res, nil
}
