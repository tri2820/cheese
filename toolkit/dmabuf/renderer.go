package dmabuf

import (
	"fmt"

	"github.com/tri2820/cheese/toolkit/render"
	"github.com/tri2820/cheese/toolkit/surface"
)

// BufferInfo contains the DMA-BUF metadata for a single buffer.
type BufferInfo struct {
	Fd       int      // DMA-BUF file descriptor
	Stride   int      // Bytes per row
	Format   Format   // DRM format (e.g., FormatXRGB8888)
	Modifier Modifier // DRM modifier (e.g., ModLinear)
}

// BufferProvider is the interface that users implement to provide GPU resources.
// The Renderer calls these methods to coordinate buffer lifecycle.
type BufferProvider interface {
	// CreateBuffers creates GPU resources at given dimensions.
	// Called on first configure and on resize.
	CreateBuffers(width, height, count int) error

	// BufferInfo returns dmabuf metadata for buffer at index.
	// Called once per buffer after CreateBuffers succeeds.
	BufferInfo(index int) BufferInfo

	// Render renders to the GPU resource at index.
	// Called each frame with a free buffer index.
	Render(index, width, height int, time uint32) error

	// DestroyBuffers cleans up GPU resources.
	// Called before resize and on Close().
	DestroyBuffers()
}

// RendererConfig configures a new DMA-BUF Renderer.
type RendererConfig struct {
	// State is the DMA-BUF protocol state
	State *State

	// Target is the Window or LayerSurface to render to
	Target render.RenderTarget

	// Buffers is the number of buffers for double/triple buffering (default 2)
	Buffers int

	// Provider is the user's GPU implementation
	Provider BufferProvider
}

// Renderer handles high-level DMA-BUF rendering to any surface.
// It coordinates buffer creation/destruction and the Wayland protocol dance,
// delegating actual GPU resource management to a BufferProvider.
type Renderer struct {
	state       *State
	surface     *surface.Surface
	target      render.RenderTarget
	provider    BufferProvider
	buffers     []*Buffer
	bufferCount int
	lastWidth   int
	lastHeight  int
}

// NewRenderer creates a new DMA-BUF renderer attached to a surface.
func NewRenderer(config RendererConfig) (*Renderer, error) {
	if config.State == nil {
		return nil, fmt.Errorf("state is required")
	}
	if config.Target == nil {
		return nil, fmt.Errorf("target is required")
	}
	if config.Provider == nil {
		return nil, fmt.Errorf("provider is required")
	}
	if config.Buffers < 1 {
		config.Buffers = 2 // default to double buffering
	}

	r := &Renderer{
		state:       config.State,
		surface:     config.Target.Surface(),
		target:      config.Target,
		provider:    config.Provider,
		bufferCount: config.Buffers,
		lastWidth:   0,
		lastHeight:  0,
	}

	// Set up frame handler
	config.Target.Surface().SetFrameHandler(func(time uint32) {
		r.render(time)
	})

	// Set up configure handler
	config.Target.SetConfigureHandler(func() {
		r.handleConfigure()
	})

	return r, nil
}

// handleConfigure handles configure events (resize).
func (r *Renderer) handleConfigure() {
	width := r.target.Width()
	height := r.target.Height()

	// Check if dimensions changed
	if width == r.lastWidth && height == r.lastHeight {
		// No size change, just render
		r.render(0)
		return
	}

	// Destroy old buffers
	r.destroyBuffers()

	// Ask provider to create new GPU resources
	if err := r.provider.CreateBuffers(width, height, r.bufferCount); err != nil {
		fmt.Printf("failed to create buffers: %v\n", err)
		return
	}

	// Create wl_buffers from the provider's GPU resources
	r.buffers = make([]*Buffer, r.bufferCount)
	for i := 0; i < r.bufferCount; i++ {
		info := r.provider.BufferInfo(i)

		params, err := r.state.CreateParams()
		if err != nil {
			fmt.Printf("failed to create params: %v\n", err)
			return
		}

		err = params.Add(info.Fd, 0, 0, uint32(info.Stride), info.Modifier)
		if err != nil {
			fmt.Printf("failed to add plane: %v\n", err)
			return
		}

		wlBuffer, err := params.CreateImmed(width, height, info.Format, 0)
		if err != nil {
			fmt.Printf("failed to create buffer: %v\n", err)
			return
		}

		r.buffers[i] = NewBuffer(wlBuffer, width, height, info.Format)
		r.buffers[i].UserData = i
	}

	r.lastWidth = width
	r.lastHeight = height

	// Render first frame
	r.render(0)
}

// render handles frame callbacks and renders.
func (r *Renderer) render(time uint32) {
	if len(r.buffers) == 0 {
		return
	}

	width := r.lastWidth
	height := r.lastHeight

	// Find a free buffer
	bufferIndex := -1
	for i, buf := range r.buffers {
		if buf != nil && !buf.Busy() {
			bufferIndex = i
			break
		}
	}
	if bufferIndex == -1 {
		// All buffers busy, skip this frame
		return
	}

	buf := r.buffers[bufferIndex]

	// Call provider's render function
	if err := r.provider.Render(bufferIndex, width, height, time); err != nil {
		fmt.Printf("render error: %v\n", err)
		return
	}

	// Attach buffer to surface
	if err := r.surface.Attach(buf.WlBuffer(), 0, 0); err != nil {
		fmt.Printf("failed to attach: %v\n", err)
		return
	}

	// Damage the entire surface
	if err := r.surface.Damage(0, 0, int32(width), int32(height)); err != nil {
		fmt.Printf("failed to damage: %v\n", err)
		return
	}

	// Request frame callback BEFORE commit
	if err := r.surface.Frame(); err != nil {
		fmt.Printf("failed to request frame: %v\n", err)
	}

	// Commit the surface
	if err := r.surface.Commit(); err != nil {
		fmt.Printf("failed to commit: %v\n", err)
		return
	}

	buf.MarkBusy()
}

// destroyBuffers destroys wl_buffers and tells provider to clean up.
func (r *Renderer) destroyBuffers() {
	// Destroy wl_buffers first
	for _, buf := range r.buffers {
		if buf != nil {
			buf.Destroy()
		}
	}
	r.buffers = nil

	// Tell provider to clean up GPU resources
	r.provider.DestroyBuffers()
}

// Width returns the current width of the render buffers.
func (r *Renderer) Width() int {
	return r.lastWidth
}

// Height returns the current height of the render buffers.
func (r *Renderer) Height() int {
	return r.lastHeight
}

// Close destroys the renderer and frees resources.
func (r *Renderer) Close() error {
	r.destroyBuffers()
	return nil
}
