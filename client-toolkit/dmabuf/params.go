package dmabuf

import (
	"fmt"

	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/protocols/linux_dmabuf_unstable_v1"
)

// Params is a builder for creating DMA-BUF backed wl_buffers.
// Add planes with Add(), then call CreateImmed() to create the buffer.
type Params struct {
	params *linux_dmabuf_unstable_v1.ZwpLinuxBufferParamsV1
	used   bool
}

// Add adds a DMA-BUF plane to the buffer parameters.
//
// Parameters:
//   - fd: the DMA-BUF file descriptor
//   - planeIdx: the plane index (0 for single-plane formats)
//   - offset: offset from the start of the DMA-BUF to the plane
//   - stride: stride (bytes per row) of the plane
//   - modifier: DRM format modifier (use ModLinear for linear layout)
func (p *Params) Add(fd int, planeIdx uint32, offset uint32, stride uint32, modifier Modifier) error {
	if p.used {
		return fmt.Errorf("params already used")
	}

	modHi := uint32(modifier >> 32)
	modLo := uint32(modifier & 0xFFFFFFFF)

	return p.params.Add(uintptr(fd), planeIdx, offset, stride, modHi, modLo)
}

// CreateImmed immediately creates a wl_buffer from the added planes.
// This is the synchronous version - the buffer is created immediately.
//
// Parameters:
//   - width: buffer width in pixels
//   - height: buffer height in pixels
//   - format: DRM pixel format (e.g., FormatXRGB8888)
//   - flags: buffer flags (usually 0)
//
// Returns the created wl_buffer which can be attached to a surface.
func (p *Params) CreateImmed(width, height int, format Format, flags uint32) (*client.WlBuffer, error) {
	if p.used {
		return nil, fmt.Errorf("params already used")
	}
	p.used = true

	buffer, err := p.params.CreateImmed(
		int32(width),
		int32(height),
		uint32(format),
		linux_dmabuf_unstable_v1.ZwpLinuxBufferParamsV1Flags(flags),
	)
	if err != nil {
		return nil, err
	}

	return buffer, nil
}

// Destroy destroys the params object without creating a buffer.
// Call this if you need to abort buffer creation.
func (p *Params) Destroy() error {
	if p.used {
		return nil
	}
	p.used = true
	return p.params.Destroy()
}

// Flags for buffer creation.
const (
	FlagYInvert     uint32 = 1 // Buffer is y-inverted
	FlagInterlaced  uint32 = 2 // Buffer is interlaced
	FlagBottomFirst uint32 = 4 // Bottom field first (for interlaced)
)
