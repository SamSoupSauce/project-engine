package engine

import (
	"errors"
	"reflect"
	"testing"
)

func TestTransformComponentDefaultAndRenderableConstraint(t *testing.T) {
	// 1. NewGameObject starts with zero default components; TransformComponent must be added explicitly.
	obj := NewGameObject("hero", "Hero")
	if len(obj.Components) != 0 || obj.GetTransformComponent() != nil {
		t.Fatalf("expected NewGameObject to start with 0 components, got %d", len(obj.Components))
	}

	// Attaching Renderable before TransformComponent triggers constraint error and blocks rendering.
	rend := NewRenderable("hero_mesh", "steel")
	obj.AddComponent(rend)
	if !errors.Is(rend.ConstraintErr, ErrMissingTransformComponent) {
		t.Fatalf("expected ErrMissingTransformComponent without TransformComponent, got %v", rend.ConstraintErr)
	}
	buf := NewRenderPassBuffer(4)
	buf.ExtractFromHierarchy(obj)
	if len(buf.Commands) != 0 {
		t.Fatalf("expected Renderable without TransformComponent to emit 0 DrawCommands, got %d", len(buf.Commands))
	}

	// 2. Adding TransformComponent satisfies constraint and positions the entity cleanly.
	tc := NewTransformComponentAt(NewVector3(3, 4, 5))
	obj.AddComponent(tc)
	if err := rend.ValidateConstraint(); err != nil {
		t.Fatalf("expected Renderable constraint satisfied once TransformComponent added, got %v", err)
	}
	tc.Translate(NewVector3(1, -2, 0.5))
	if p := tc.GetPosition(); p.X != 4 || p.Y != 2 || p.Z != 5.5 {
		t.Fatalf("unexpected position after Translate: %+v", p)
	}

	buf.Reset()
	buf.ExtractFromHierarchy(obj)
	if len(buf.Commands) != 1 || buf.Commands[0].WorldTransform.Position.X != 4 {
		t.Fatalf("expected 1 DrawCommand at X=4, got %+v", buf.Commands)
	}

	// 3. Removing TransformComponent violates the Renderable constraint again.
	if !obj.RemoveComponent(reflect.TypeOf((*TransformComponent)(nil))) {
		t.Fatalf("expected RemoveComponent(TransformComponent) to succeed")
	}
	if err := rend.ValidateConstraint(); !errors.Is(err, ErrMissingTransformComponent) {
		t.Fatalf("expected ErrMissingTransformComponent after removal, got %v", err)
	}
	buf.Reset()
	buf.ExtractFromHierarchy(obj)
	if len(buf.Commands) != 0 {
		t.Fatalf("expected 0 DrawCommands after removing TransformComponent, got %d", len(buf.Commands))
	}
}
