package luabridge

/*
#include <stdint.h>
#include <stdbool.h>
*/
import "C"

import (
	"sync"
	"unsafe"

	"github.com/AllenDang/cimgui-go/imgui"
)

// DrawListCall captures a DrawList FFI invocation for headless inspection/testing.
type DrawListCall struct {
	Op       string
	MinX     float32
	MinY     float32
	MaxX     float32
	MaxY     float32
	Color    uint32
	Rounding float32
	Text     string
}

var (
	drawRecorderMu sync.Mutex
	drawRecorder   *[]DrawListCall
)

// SetDrawListRecorder enables or disables recording of DrawList FFI calls for tests.
func SetDrawListRecorder(buf *[]DrawListCall) {
	drawRecorderMu.Lock()
	drawRecorder = buf
	drawRecorderMu.Unlock()
}

func recordDrawCall(call DrawListCall) {
	drawRecorderMu.Lock()
	if drawRecorder != nil {
		*drawRecorder = append(*drawRecorder, call)
	}
	drawRecorderMu.Unlock()
}

//export DrawList_AddRectFilled
func DrawList_AddRectFilled(drawList unsafe.Pointer, minX, minY, maxX, maxY C.float, color C.uint32_t, rounding C.float) {
	if drawList != nil {
		dl := imgui.NewDrawListFromC(drawList)
		if float32(rounding) > 0 {
			dl.AddRectFilledV(
				imgui.NewVec2(float32(minX), float32(minY)),
				imgui.NewVec2(float32(maxX), float32(maxY)),
				uint32(color),
				float32(rounding),
				imgui.DrawFlagsNone,
			)
		} else {
			dl.AddRectFilled(
				imgui.NewVec2(float32(minX), float32(minY)),
				imgui.NewVec2(float32(maxX), float32(maxY)),
				uint32(color),
			)
		}
	}
	if drawRecorder != nil {
		recordDrawCall(DrawListCall{
			Op:       "AddRectFilled",
			MinX:     float32(minX),
			MinY:     float32(minY),
			MaxX:     float32(maxX),
			MaxY:     float32(maxY),
			Color:    uint32(color),
			Rounding: float32(rounding),
		})
	}
}

//export DrawList_PushClipRect
func DrawList_PushClipRect(drawList unsafe.Pointer, minX, minY, maxX, maxY C.float, intersect C.bool) {
	if drawList != nil {
		dl := imgui.NewDrawListFromC(drawList)
		dl.PushClipRectV(
			imgui.NewVec2(float32(minX), float32(minY)),
			imgui.NewVec2(float32(maxX), float32(maxY)),
			bool(intersect),
		)
	}
	if drawRecorder != nil {
		recordDrawCall(DrawListCall{
			Op:   "PushClipRect",
			MinX: float32(minX),
			MinY: float32(minY),
			MaxX: float32(maxX),
			MaxY: float32(maxY),
		})
	}
}

//export DrawList_PopClipRect
func DrawList_PopClipRect(drawList unsafe.Pointer) {
	if drawList != nil {
		dl := imgui.NewDrawListFromC(drawList)
		dl.PopClipRect()
	}
	if drawRecorder != nil {
		recordDrawCall(DrawListCall{Op: "PopClipRect"})
	}
}

//export DrawList_AddText
func DrawList_AddText(drawList unsafe.Pointer, x, y C.float, color C.uint32_t, text *C.char) {
	txt := ""
	if text != nil {
		txt = C.GoString(text)
	}
	if drawList != nil {
		dl := imgui.NewDrawListFromC(drawList)
		dl.AddTextVec2(imgui.NewVec2(float32(x), float32(y)), uint32(color), txt)
	}
	if drawRecorder != nil {
		recordDrawCall(DrawListCall{
			Op:    "AddText",
			MinX:  float32(x),
			MinY:  float32(y),
			Color: uint32(color),
			Text:  txt,
		})
	}
}

//export igImDrawList_AddRectFilled
func igImDrawList_AddRectFilled(drawList unsafe.Pointer, minX, minY, maxX, maxY C.float, color C.uint32_t, rounding C.float) {
	DrawList_AddRectFilled(drawList, minX, minY, maxX, maxY, color, rounding)
}

//export igImDrawList_AddText
func igImDrawList_AddText(drawList unsafe.Pointer, x, y C.float, color C.uint32_t, text *C.char) {
	DrawList_AddText(drawList, x, y, color, text)
}
