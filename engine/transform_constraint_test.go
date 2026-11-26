package engine

import (
	"errors"
	"reflect"
	"testing"
)

func TestTransformComponentDefaultAndRenderableConstraint(t *testing.T) {
	// 1. NewGameObject automatically attaches a TransformComponent keeping 3D position synchronized.
	obj := NewGameObject("hero", "Hero")
	tc := obj.GetTransformComponent()
	if tc == nil {
		t.Fatalf("expected NewGameObject to attach TransformComponent by default")
	}

	tc.SetPosition(NewVector3(3, 4, 5))
	if obj.Transform.Position.X != 3 || obj.Transform.Position.Y != 4 || obj.Transform.Position.Z != 5 {
		t.Fatalf("expected TransformComponent.SetPosition to sync Owner.Transform, got %+v", obj.Transform.Position)
	}
	tc.Translate(NewVector3(1, -2, 0.5))
	if p := tc.GetPosition(); p.X != 4 || p.Y != 2 || p.Z != 5.5 {
		t.Fatalf("unexpected position after Translate: %+v", p)
	}

	// 2. Renderable validates TransformComponent constraint and extracts DrawCommand using it.
	rend := NewRenderable("hero_mesh", "steel")
	obj.AddComponent(rend)
	if err := rend.ValidateConstraint(); err != nil {
		t.Fatalf("expected Renderable constraint satisfied when TransformComponent present, got %v", err)
	}

	buf := NewRenderPassBuffer(4)
	buf.ExtractFromHierarchy(obj)
	if len(buf.Commands) != 1 || buf.Commands[0].WorldTransform.Position.X != 4 {
		t.Fatalf("expected 1 DrawCommand at X=4, got %+v", buf.Commands)
	}

	// 3. Removing TransformComponent violates the Renderable constraint and suppresses render extraction/dispatch.
	if !obj.RemoveComponent(reflect.TypeOf((*TransformComponent)(nil))) {
		t.Fatalf("expected RemoveComponent(TransformComponent) to succeed")
	}
	if err := rend.ValidateConstraint(); !errors.Is(err, ErrMissingTransformComponent) {
		t.Fatalf("expected ErrMissingTransformComponent after removal, got %v", err)
	}

	buf.Reset()
	buf.ExtractFromHierarchy(obj)
	if len(buf.Commands) != 0 {
		t.Fatalf("expected Renderable without TransformComponent to be skipped by extraction, got %d commands", len(buf.Commands))
	}

	// 4. Raw struct GameObject without TransformComponent also triggers constraint failure on Renderable.
	raw := &GameObject{ID: "raw", Name: "Raw", Active: true, Transform: NewTransform()}
	rawRend := NewRenderable("raw_mesh", "mat")
	raw.AddComponent(rawRend)
	if !errors.Is(rawRend.ConstraintErr, ErrMissingTransformComponent) {
		t.Fatalf("expected raw GameObject Renderable Init ConstraintErr, got %v", rawRend.ConstraintErr)
	}
}
