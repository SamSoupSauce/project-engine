package engine

// ScriptLifecycle defines engine lifecycle hooks implemented by scripting components.
type ScriptLifecycle interface {
	Component
	OnLoad(scene Scene)
	OnUnload(scene Scene)
	OnSpawn(sender any)
	OnEvent(name string, sender *GameObject, data any)
	OnTick(dtMs int)
	OnDespawn(sender *GameObject, data any)
}

// EventBus defines a scene-level dispatcher for named GameObject events.
type EventBus interface {
	Emit(name string, sender *GameObject, data any)
}

// SceneController extends Scene with scene-graph management, queries, and lifecycle control.
type SceneController interface {
	Scene
	EventBus
	GetName() string
	IsLoaded() bool
	GetRoot() *GameObject
	Spawn(obj *GameObject, sender any)
	Despawn(obj *GameObject, sender *GameObject, data any)
	FindByID(id string) *GameObject
	FindByName(name string) *GameObject
	GetObjects() []*GameObject
	ObjectCount() int
}
