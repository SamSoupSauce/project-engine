package engine

// Transform represents spatial identity in 3D world/local space:
// position (meters), rotation (Euler angles in degrees), and scale.
type Transform struct {
	Position Vector3
	Rotation Vector3 // Pitch (X), Yaw (Y), Roll (Z) in degrees
	Scale    Vector3
}

// NewTransform returns an identity transform at origin with unit scale.
func NewTransform() Transform {
	return Transform{
		Position: NewVector3(0, 0, 0),
		Rotation: NewVector3(0, 0, 0),
		Scale:    NewVector3(1, 1, 1),
	}
}

// NewTransformAt creates a transform at a specific 3D coordinate with unit scale.
func NewTransformAt(pos Vector3) Transform {
	return Transform{
		Position: pos,
		Rotation: NewVector3(0, 0, 0),
		Scale:    NewVector3(1, 1, 1),
	}
}

// CombineTransforms combines a parent transform with a local child transform.
func CombineTransforms(parent, local Transform) Transform {
	// Scaled local offset
	scaledOffset := NewVector3(
		local.Position.X*parent.Scale.X,
		local.Position.Y*parent.Scale.Y,
		local.Position.Z*parent.Scale.Z,
	)

	// In a full quaternion pipeline this rotates by parent orientation;
	// for standard 3D hierarchy we compose the translation and sum the Euler rotations:
	worldPos := Vector3Add(parent.Position, scaledOffset)
	worldRot := Vector3Add(parent.Rotation, local.Rotation)
	worldScale := NewVector3(
		parent.Scale.X*local.Scale.X,
		parent.Scale.Y*local.Scale.Y,
		parent.Scale.Z*local.Scale.Z,
	)

	return Transform{
		Position: worldPos,
		Rotation: worldRot,
		Scale:    worldScale,
	}
}
