package engine

// Vector3 represents a 3D coordinate or vector in world/local space.
type Vector3 struct {
	X float32
	Y float32
	Z float32
}

// NewVector3 creates a Vector3 with the given X, Y, Z components.
func NewVector3(x, y, z float32) Vector3 {
	return Vector3{X: x, Y: y, Z: z}
}

// Vector3Add returns the component-wise sum of two vectors.
func Vector3Add(v1, v2 Vector3) Vector3 {
	return Vector3{
		X: v1.X + v2.X,
		Y: v1.Y + v2.Y,
		Z: v1.Z + v2.Z,
	}
}

// Color represents an 8-bit per channel RGBA color.
type Color struct {
	R uint8
	G uint8
	B uint8
	A uint8
}

// NewColor constructs an RGBA Color.
func NewColor(r, g, b, a uint8) Color {
	return Color{R: r, G: g, B: b, A: a}
}

var (
	White = Color{R: 255, G: 255, B: 255, A: 255}
	Black = Color{R: 0, G: 0, B: 0, A: 255}
)

// BoundingBox represents an axis-aligned 3D bounding box.
type BoundingBox struct {
	Min Vector3
	Max Vector3
}

// NewBoundingBox constructs a BoundingBox from min and max corners.
func NewBoundingBox(min, max Vector3) BoundingBox {
	return BoundingBox{Min: min, Max: max}
}

// Ray represents a 3D ray with origin Position and direction vector.
type Ray struct {
	Position  Vector3
	Direction Vector3
}

// CameraProjection identifies the 3D camera projection mode.
type CameraProjection int32

const (
	CameraPerspective CameraProjection = iota
	CameraOrthographic
)

// Camera3D defines a 3D scene camera's orientation and projection parameters.
type Camera3D struct {
	Position   Vector3
	Target     Vector3
	Up         Vector3
	Fovy       float32
	Projection CameraProjection
}
