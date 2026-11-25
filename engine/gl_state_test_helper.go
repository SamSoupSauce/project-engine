package engine

import "unsafe"

func unsafePtrFromInt32(p *int32) unsafe.Pointer {
	return unsafe.Pointer(p)
}
