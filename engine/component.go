package engine

// Component defines the lifecycle interface for all modular behaviors attached to a GameObject.
type Component interface {
	// Init is called when the component is registered with its owning GameObject.
	Init(owner *GameObject)
	// Update is called once per frame with the elapsed delta time in seconds.
	Update(dt float32)
	// Destroy is called when the component or owner is being decommissioned.
	Destroy()
}
