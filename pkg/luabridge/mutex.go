package luabridge

import (
	"bytes"
	"runtime"
	"strconv"
	"sync"
)

// reentrantMutex is a goroutine-aware recursive mutex that prevents deadlocks
// when Lua scripts invoke Go bindings (such as Emit, Spawn, or Despawn) that
// synchronously dispatch back into Lua hooks on the same LuaBridge.
type reentrantMutex struct {
	mu        sync.Mutex
	cond      *sync.Cond
	owner     uint64
	recursion int
}

func (r *reentrantMutex) Lock() {
	gid := currentGoroutineID()
	r.mu.Lock()
	if r.cond == nil {
		r.cond = sync.NewCond(&r.mu)
	}
	for r.recursion > 0 && r.owner != gid {
		r.cond.Wait()
	}
	r.owner = gid
	r.recursion++
	r.mu.Unlock()
}

func (r *reentrantMutex) Unlock() {
	gid := currentGoroutineID()
	r.mu.Lock()
	if r.recursion == 0 || r.owner != gid {
		r.mu.Unlock()
		panic("luabridge: unlock of unowned reentrantMutex")
	}
	r.recursion--
	if r.recursion == 0 {
		r.owner = 0
		r.cond.Signal()
	}
	r.mu.Unlock()
}

func currentGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Stack header starts with: "goroutine <id> ["
	rest := bytes.TrimPrefix(buf[:n], []byte("goroutine "))
	idx := bytes.IndexByte(rest, ' ')
	if idx <= 0 {
		return 0
	}
	id, err := strconv.ParseUint(string(rest[:idx]), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
