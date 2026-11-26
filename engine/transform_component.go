package engine

import "reflect"

// TransformComponent is the sole ECS Component storing a GameObject's 3D location,
// Euler rotation (degrees), and scale.
type TransformComponent struct {
	Owner     *GameObject
	Transform Transform
}

// NewTransformComponent creates an identity TransformComponent at origin (0, 0, 0).
func NewTransformComponent() *TransformComponent {
	return &TransformComponent{
		Transform: NewTransform(),
	}
}

// NewTransformComponentAt creates a TransformComponent initialized at the given 3D location.
func NewTransformComponentAt(location Vector3) *TransformComponent {
	return &TransformComponent{
		Transform: NewTransformAt(location),
	}
}

// Init implements Component.
func (t *TransformComponent) Init(owner *GameObject) {
	t.Owner = owner
}

// Update implements Component.
func (t *TransformComponent) Update(_ float32) {}

// Destroy implements Component.
func (t *TransformComponent) Destroy() {
	t.Owner = nil
}

// Value returns the local Transform snapshot.
func (t *TransformComponent) Value() Transform {
	if t == nil {
		return NewTransform()
	}
	return t.Transform
}

// GetPosition returns the 3D location stored by this TransformComponent.
func (t *TransformComponent) GetPosition() Vector3 {
	return t.Value().Position
}

// SetPosition updates the 3D location stored on this TransformComponent.
func (t *TransformComponent) SetPosition(pos Vector3) {
	if t == nil {
		return
	}
	t.Transform.Position = pos
}

// Translate shifts the 3D location by delta.
func (t *TransformComponent) Translate(delta Vector3) {
	cur := t.GetPosition()
	t.SetPosition(NewVector3(cur.X+delta.X, cur.Y+delta.Y, cur.Z+delta.Z))
}

// GetRotation returns Euler angles in degrees.
func (t *TransformComponent) GetRotation() Vector3 {
	return t.Value().Rotation
}

// SetRotation updates Euler angles in degrees.
func (t *TransformComponent) SetRotation(rot Vector3) {
	if t == nil {
		return
	}
	t.Transform.Rotation = rot
}

// GetScale returns local scale factors.
func (t *TransformComponent) GetScale() Vector3 {
	return t.Value().Scale
}

// SetScale updates local scale factors.
func (t *TransformComponent) SetScale(scale Vector3) {
	if t == nil {
		return
	}
	t.Transform.Scale = scale
}

// WorldTransform evaluates the cascaded world-space transform through the parent hierarchy.
func (t *TransformComponent) WorldTransform() Transform {
	if t == nil {
		return NewTransform()
	}
	if t.Owner == nil || t.Owner.Parent == nil {
		return t.Transform
	}
	return CombineTransforms(t.Owner.Parent.GetWorldTransform(), t.Transform)
}

var transformComponentType = reflect.TypeOf((*TransformComponent)(nil))

// GetTransformComponent returns the attached TransformComponent on this GameObject, or nil.
func (go_ *GameObject) GetTransformComponent() *TransformComponent {
	if go_ == nil {
		return nil
	}
	if comp := go_.GetComponent(transformComponentType); comp != nil {
		if tc, ok := comp.(*TransformComponent); ok {
			return tc
		}
	}
	return nil
}

// EnsureTransform returns an existing TransformComponent or attaches a new identity TransformComponent.
func (go_ *GameObject) EnsureTransform() *TransformComponent {
	if go_ == nil {
		return nil
	}
	if tc := go_.GetTransformComponent(); tc != nil {
		return tc
	}
	tc := NewTransformComponent()
	go_.AddComponent(tc)
	return tc
}
