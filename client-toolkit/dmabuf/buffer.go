package dmabuf

import (
	"github.com/tri2820/cheese/protocols/client"
)

// Buffer wraps a wl_buffer created from a DMA-BUF.
// It tracks whether the buffer is currently in use by the compositor.
type Buffer struct {
	wlBuffer        *client.WlBuffer
	width           int
	height          int
	format          Format
	busy            bool
	releaseHandlers []func()

	// User data - can store the backing resource (e.g., Vulkan image index)
	UserData interface{}
}

// NewBuffer wraps a wl_buffer with release tracking.
// The onRelease callback is called when the compositor releases the buffer.
func NewBuffer(wlBuffer *client.WlBuffer, width, height int, format Format) *Buffer {
	b := &Buffer{
		wlBuffer: wlBuffer,
		width:    width,
		height:   height,
		format:   format,
		busy:     false,
	}

	// Set up release handler
	wlBuffer.SetReleaseHandler(func(ev client.WlBufferReleaseEvent) {
		b.busy = false
		for _, fn := range append([]func(){}, b.releaseHandlers...) {
			if fn != nil {
				fn()
			}
		}
	})

	return b
}

// WlBuffer returns the underlying wl_buffer.
func (b *Buffer) WlBuffer() *client.WlBuffer {
	return b.wlBuffer
}

// Width returns the buffer width.
func (b *Buffer) Width() int {
	return b.width
}

// Height returns the buffer height.
func (b *Buffer) Height() int {
	return b.height
}

// Format returns the buffer format.
func (b *Buffer) Format() Format {
	return b.format
}

// Busy returns true if the buffer is currently in use by the compositor.
func (b *Buffer) Busy() bool {
	return b.busy
}

// MarkBusy marks the buffer as in use (call after attaching to surface).
func (b *Buffer) MarkBusy() {
	b.busy = true
}

// OnRelease registers a handler that fires when the compositor releases the buffer.
func (b *Buffer) OnRelease(fn func()) {
	if fn == nil {
		return
	}
	b.releaseHandlers = append(b.releaseHandlers, fn)
}

// Destroy destroys the wl_buffer.
func (b *Buffer) Destroy() error {
	return b.wlBuffer.Destroy()
}
