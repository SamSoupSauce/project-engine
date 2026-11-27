package engine

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestParseGLTFJSONAndBinaryGLB(t *testing.T) {
	// 1. Parse real disk glTF asset with embedded base64 buffer.
	asset, err := LoadGLTFAsset("../assets/models/props/kitchen_toaster.gltf")
	if err != nil {
		t.Fatalf("LoadGLTFAsset failed: %v", err)
	}
	if asset.Version != "2.0" || asset.TriangleCount() != 12 {
		t.Fatalf("unexpected asset metadata: ver=%s tris=%d", asset.Version, asset.TriangleCount())
	}
	if len(asset.Materials) != 1 || asset.Materials[0].Name != "metal_hollow" {
		t.Fatalf("unexpected materials: %+v", asset.Materials)
	}
	if len(asset.HullPoints) != 8 {
		t.Fatalf("expected 8 unique box hull vertices, got %d", len(asset.HullPoints))
	}
	if math.Abs(float64(asset.Bounds.Max.X-0.16)) > 1e-4 || math.Abs(float64(asset.Bounds.Min.Y+0.11)) > 1e-4 {
		t.Fatalf("unexpected glTF bounds: %+v", asset.Bounds)
	}

	// 2. Construct minimal valid binary .glb v2 container with 1 triangle and verify ParseGLTFBytes.
	jsonStr := `{"asset":{"version":"2.0"},"buffers":[{"byteLength":36}],"bufferViews":[{"buffer":0,"byteOffset":0,"byteLength":36}],"accessors":[{"bufferView":0,"byteOffset":0,"componentType":5126,"count":3,"type":"VEC3"}],"meshes":[{"name":"tri","primitives":[{"attributes":{"POSITION":0}}]}]}`
	for len(jsonStr)%4 != 0 {
		jsonStr += " "
	}
	var binBuf bytes.Buffer
	verts := [][3]float32{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}
	for _, v := range verts {
		_ = binary.Write(&binBuf, binary.LittleEndian, v[0])
		_ = binary.Write(&binBuf, binary.LittleEndian, v[1])
		_ = binary.Write(&binBuf, binary.LittleEndian, v[2])
	}
	binBytes := binBuf.Bytes()

	totalLen := uint32(12 + 8 + len(jsonStr) + 8 + len(binBytes))
	var glb bytes.Buffer
	_ = binary.Write(&glb, binary.LittleEndian, glbMagic)
	_ = binary.Write(&glb, binary.LittleEndian, uint32(2))
	_ = binary.Write(&glb, binary.LittleEndian, totalLen)
	_ = binary.Write(&glb, binary.LittleEndian, uint32(len(jsonStr)))
	_ = binary.Write(&glb, binary.LittleEndian, glbChunkJSON)
	glb.WriteString(jsonStr)
	_ = binary.Write(&glb, binary.LittleEndian, uint32(len(binBytes)))
	_ = binary.Write(&glb, binary.LittleEndian, glbChunkBIN)
	glb.Write(binBytes)

	parsedGLB, err := ParseGLTFBytes(glb.Bytes(), ".")
	if err != nil {
		t.Fatalf("ParseGLTFBytes(.glb) failed: %v", err)
	}
	if parsedGLB.TriangleCount() != 1 || len(parsedGLB.HullPoints) != 3 {
		t.Fatalf("expected 1 triangle and 3 verts from GLB, got tris=%d hull=%d",
			parsedGLB.TriangleCount(), len(parsedGLB.HullPoints))
	}
}
