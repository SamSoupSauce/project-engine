package engine

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// EncodeRGBABufferBase64 converts raw bottom-up OpenGL RGBA pixels (width x height x 4 bytes)
// into a base64-encoded PNG (default) or JPEG image.
func EncodeRGBABufferBase64(pixels []byte, width, height int, format string) (encoded string, normFormat string, err error) {
	if width <= 0 || height <= 0 {
		return "", "", fmt.Errorf("frame capture: invalid dimensions %dx%d", width, height)
	}
	expected := width * height * 4
	if len(pixels) < expected {
		return "", "", fmt.Errorf("frame capture: pixel buffer too small (%d < %d)", len(pixels), expected)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	stride := width * 4
	for y := 0; y < height; y++ {
		srcRow := (height - 1 - y) * stride
		dstRow := y * img.Stride
		copy(img.Pix[dstRow:dstRow+stride], pixels[srcRow:srcRow+stride])
	}

	var buf bytes.Buffer
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "png":
		normFormat = "png"
		if err := png.Encode(&buf, img); err != nil {
			return "", "", fmt.Errorf("frame capture: png encode: %w", err)
		}
	case "jpg", "jpeg":
		normFormat = "jpeg"
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return "", "", fmt.Errorf("frame capture: jpeg encode: %w", err)
		}
	default:
		return "", "", fmt.Errorf("frame capture: unsupported format %q (expected png or jpeg)", format)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), normFormat, nil
}

// CaptureGLBackbuffer reads the current OpenGL backbuffer and returns a base64-encoded PNG or JPEG.
// Must be called on the thread owning the active OpenGL context before SwapBuffers.
func CaptureGLBackbuffer(width, height int, format string) (encoded string, normFormat string, err error) {
	if width <= 0 || height <= 0 {
		return "", "", fmt.Errorf("frame capture: invalid framebuffer size %dx%d", width, height)
	}
	pixels := make([]byte, width*height*4)
	gl.ReadBuffer(gl.BACK)
	gl.PixelStorei(gl.PACK_ALIGNMENT, 1)
	gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
	return EncodeRGBABufferBase64(pixels, width, height, format)
}
