package buffer

import (
	"fmt"

	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/toolkit/surface"
)

// Renderer handles high-level rendering to any surface.
// It manages a swapchain internally and provides a simple OnRender callback.
type Renderer struct {
	swapchain      *Swapchain
	surface        *surface.Surface
	setConfigure   func(func())
	onRender       func(uint32, []byte)
	firstFrameDone bool
}

// RendererConfig configures a new Renderer.
type RendererConfig struct {
	// Shm is the wl_shm global
	Shm *client.WlShm

	// Surface is the surface to render to
	Surface *surface.Surface

	// SetConfigure is a function to register a configure handler.
	// The renderer will call this to set up its internal configure handling.
	// For example: window.SetConfigureHandler or layer.SetConfigureHandler
	SetConfigure func(func())

	// Width and height of the render buffers
	Width  int
	Height int

	// Format is the pixel format
	Format Format

	// Buffers is the number of buffers for double/triple buffering
	Buffers int
}

// NewRenderer creates a new renderer attached to a surface.
func NewRenderer(config RendererConfig) (*Renderer, error) {
	if config.Surface == nil {
		return nil, fmt.Errorf("surface is required")
	}
	if config.SetConfigure == nil {
		return nil, fmt.Errorf("SetConfigure is required")
	}
	if config.Buffers < 1 {
		config.Buffers = 2 // default to double buffering
	}

	// Create swapchain
	swapchain, err := NewSwapchain(SwapchainConfig{
		Shm:     config.Shm,
		Buffers: config.Buffers,
		Width:   config.Width,
		Height:  config.Height,
		Format:  config.Format,
	})
	if err != nil {
		return nil, fmt.Errorf("create swapchain: %w", err)
	}

	// Attach swapchain to surface
	swapchain.SetSurface(config.Surface)

	r := &Renderer{
		swapchain:    swapchain,
		surface:      config.Surface,
		setConfigure: config.SetConfigure,
	}

	return r, nil
}

// OnRender sets the render callback.
// The callback receives:
// - time: timestamp in milliseconds for animation
// - pixels: the buffer to draw into
//
// The renderer automatically handles:
// - First configure event (synchronous render with time=0)
// - Frame callbacks for subsequent renders (with actual time)
// - Acquire → callback → present
func (r *Renderer) OnRender(fn func(time uint32, pixels []byte)) {
	r.onRender = fn

	// Set up frame callback handler first (before any render)
	r.surface.SetFrameHandler(func(time uint32) {
		r.render(time)
	})

	// Set up configure handler to handle first frame
	r.setConfigure(func() {
		if !r.firstFrameDone {
			// First configure - render synchronously
			r.render(0)
			r.firstFrameDone = true
		}
	})
}

// render is the internal render handler that acquires, calls user callback, presents.
func (r *Renderer) render(time uint32) {
	if r.onRender == nil {
		return
	}

	// Acquire buffer
	pixels, err := r.swapchain.Acquire()
	if err != nil {
		fmt.Printf("failed to acquire buffer: %v\n", err)
		return
	}

	// Call user's render function
	r.onRender(time, pixels)

	// Request frame callback BEFORE committing
	// This associates the callback with the upcoming commit
	if err := r.surface.Frame(); err != nil {
		fmt.Printf("failed to request frame: %v\n", err)
	}

	// Present the frame (includes commit)
	if err := r.swapchain.Present(); err != nil {
		fmt.Printf("failed to present: %v\n", err)
		return
	}
}

// Width returns the width of the render buffers.
func (r *Renderer) Width() int {
	return r.swapchain.Width()
}

// Height returns the height of the render buffers.
func (r *Renderer) Height() int {
	return r.swapchain.Height()
}

// Stride returns the stride of the render buffers.
func (r *Renderer) Stride() int {
	return r.swapchain.Stride()
}

// Format returns the pixel format.
func (r *Renderer) Format() Format {
	return r.swapchain.Format()
}

// Close destroys the renderer and frees resources.
func (r *Renderer) Close() error {
	return r.swapchain.Close()
}
