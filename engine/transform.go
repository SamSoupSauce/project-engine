package engine

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Transform represents spatial identity in 3D world/local space:
// position (meters), rotation (Euler angles in degrees), and scale.
type Transform struct {
	Position rl.Vector3
	Rotation rl.Vector3 // Pitch (X), Yaw (Y), Roll (Z) in degrees
	Scale    rl.Vector3
}

// NewTransform returns an identity transform at origin with unit scale.
func NewTransform() Transform {
	return Transform{
		Position: rl.NewVector3(0, 0, 0),
		Rotation: rl.NewVector3(0, 0, 0),
		Scale:    rl.NewVector3(1, 1, 1),
	}
}

// NewTransformAt creates a transform at a specific 3D coordinate with unit scale.
func NewTransformAt(pos rl.Vector3) Transform {
	return Transform{
		Position: pos,
		Rotation: rl.NewVector3(0, 0, 0),
		Scale:    rl.NewVector3(1, 1, 1),
	}
}

// CombineTransforms combines a parent transform with a local child transform.
func CombineTransforms(parent, local Transform) Transform {
	// Scaled local offset
	scaledOffset := rl.NewVector3(
		local.Position.X*parent.Scale.X,
		local.Position.Y*parent.Scale.Y,
		local.Position.Z*parent.Scale.Z,
	)

	// In a full quaternion pipeline this rotates by parent orientation;
	// for standard 3D hierarchy we compose the translation and sum the Euler rotations:
	worldPos := rl.Vector3Add(parent.Position, scaledOffset)
	worldRot := rl.Vector3Add(parent.Rotation, local.Rotation)
	worldScale := rl.NewVector3(
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
