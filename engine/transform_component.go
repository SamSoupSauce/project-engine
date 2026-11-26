package engine

import "reflect"

// TransformComponent is a first-class engine Component storing a 3D spatial location,
// Euler rotation, and scale on a GameObject. When attached to an Owner GameObject,
// mutations stay synchronized with Owner.Transform.
type TransformComponent struct {
	Owner        *GameObject
	local        Transform
	hasExplicit  bool
}

// NewTransformComponent creates an identity TransformComponent at origin (0, 0, 0).
func NewTransformComponent() *TransformComponent {
	return &TransformComponent{
		local: NewTransform(),
	}
}

// NewTransformComponentAt creates a TransformComponent initialized at the given 3D location.
func NewTransformComponentAt(location Vector3) *TransformComponent {
	return &TransformComponent{
		local:       NewTransformAt(location),
		hasExplicit: true,
	}
}

// Init implements Component and binds this TransformComponent to owner.Transform.
func (t *TransformComponent) Init(owner *GameObject) {
	t.Owner = owner
	if owner == nil {
		return
	}
	if t.hasExplicit {
		owner.Transform = t.local
		t.hasExplicit = false
	} else {
		t.local = owner.Transform
	}
}

// Update implements Component.
func (t *TransformComponent) Update(_ float32) {}

// Destroy implements Component.
func (t *TransformComponent) Destroy() {
	if t.Owner != nil {
		t.local = t.Owner.Transform
	}
	t.Owner = nil
}

// Value returns the local Transform snapshot.
func (t *TransformComponent) Value() Transform {
	if t == nil {
		return NewTransform()
	}
	if t.Owner != nil {
		return t.Owner.Transform
	}
	return t.local
}

// GetPosition returns the 3D location stored by this TransformComponent.
func (t *TransformComponent) GetPosition() Vector3 {
	return t.Value().Position
}

// SetPosition updates the 3D location on this TransformComponent (and Owner.Transform).
func (t *TransformComponent) SetPosition(pos Vector3) {
	if t == nil {
		return
	}
	t.local.Position = pos
	if t.Owner != nil {
		t.Owner.Transform.Position = pos
	} else {
		t.hasExplicit = true
	}
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
	t.local.Rotation = rot
	if t.Owner != nil {
		t.Owner.Transform.Rotation = rot
	} else {
		t.hasExplicit = true
	}
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
	t.local.Scale = scale
	if t.Owner != nil {
		t.Owner.Transform.Scale = scale
	} else {
		t.hasExplicit = true
	}
}

// WorldTransform evaluates the cascaded world-space transform through the parent hierarchy.
func (t *TransformComponent) WorldTransform() Transform {
	if t == nil {
		return NewTransform()
	}
	if t.Owner != nil {
		return t.Owner.GetWorldTransform()
	}
	return t.local
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
