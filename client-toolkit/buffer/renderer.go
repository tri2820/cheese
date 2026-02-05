package buffer

import (
	"fmt"

	"github.com/tri2820/cheese/client-toolkit/render"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
)

// Renderer handles high-level rendering to any surface.
// It manages a swapchain internally and provides a simple OnRender callback.
type Renderer struct {
	swapchain  *Swapchain
	surface    *surface.Surface
	shm        *client.WlShm
	format     client.WlShmFormat
	buffers    int
	target     render.RenderTarget
	onRender   func(width, height int, time uint32, pixels []byte)
	lastWidth  int
	lastHeight int
	manualMode bool // when true, doesn't auto-request next frame
	ready      bool // true when we've received valid dimensions
}

// RendererConfig configures a new Renderer.
type RendererConfig struct {
	// Shm is the wl_shm global
	Shm *client.WlShm

	// Target is the Window or LayerSurface to render to
	Target render.RenderTarget

	// Format is the pixel format
	Format client.WlShmFormat

	// Buffers is the number of buffers for double/triple buffering
	Buffers int
}

// NewRenderer creates a new renderer attached to a surface.
func NewRenderer(config RendererConfig) (*Renderer, error) {
	if config.Shm == nil {
		return nil, fmt.Errorf("shm is required")
	}
	if config.Target == nil {
		return nil, fmt.Errorf("target is required")
	}
	if config.Buffers < 1 {
		config.Buffers = 2 // default to double buffering
	}

	r := &Renderer{
		surface:    config.Target.Surface(),
		shm:        config.Shm,
		format:     config.Format,
		buffers:    config.Buffers,
		target:     config.Target,
		lastWidth:  0, // Force swapchain creation on first render
		lastHeight: 0,
	}

	// Set up frame handler
	config.Target.Surface().SetFrameHandler(func(time uint32) {
		r.render(time)
	})

	// Set up configure handler
	config.Target.SetConfigureHandler(func() {
		r.render(0)
	})

	return r, nil
}

// OnRender sets the render callback.
// The callback receives:
// - width, height: dimensions of the buffer
// - time: timestamp in milliseconds for animation
// - pixels: the buffer to draw into
//
// Usage:
//
//	renderer.OnRender(func(w, h int, time uint32, pixels []byte) {
//	    // draw into pixels with dimensions w x h
//	})
func (r *Renderer) OnRender(fn func(width, height int, time uint32, pixels []byte)) {
	r.onRender = fn
}

// render is the internal render handler.
func (r *Renderer) render(time uint32) {
	if r.onRender == nil {
		return
	}

	width := r.target.Width()
	height := r.target.Height()

	// Recreate swapchain if dimensions changed
	if width != r.lastWidth || height != r.lastHeight {
		if r.swapchain != nil {
			r.swapchain.Close()
		}

		swapchain, err := NewSwapchain(SwapchainConfig{
			Shm:     r.shm,
			Buffers: r.buffers,
			Width:   width,
			Height:  height,
			Format:  r.format,
		})
		if err != nil {
			fmt.Printf("failed to create swapchain: %v\n", err)
			return
		}
		swapchain.SetSurface(r.surface)
		r.swapchain = swapchain
		r.lastWidth = width
		r.lastHeight = height

		// Let user know we are ready to render.
		r.ready = true
	}

	if !r.ready {
		return
	}

	// Acquire buffer (may fail if all buffers are busy - that's OK, skip this frame)
	pixels, err := r.swapchain.Acquire()
	if err != nil {
		// No free buffer means compositor hasn't released yet - skip this frame
		// The next frame callback will try again
		return
	}

	// Call user's render function
	r.onRender(width, height, time, pixels)

	// Request frame callback BEFORE committing
	// This associates the callback with the upcoming commit
	// Skip if in manual mode (user controls when to request frames)
	if !r.manualMode {
		if err := r.surface.Frame(); err != nil {
			fmt.Printf("failed to request frame: %v\n", err)
		}
	}

	// Present the frame (includes commit)
	if err := r.swapchain.Present(); err != nil {
		fmt.Printf("failed to present: %v\n", err)
		return
	}
}

// Width returns the current width of the render buffers.
func (r *Renderer) Width() int {
	return r.lastWidth
}

// Height returns the current height of the render buffers.
func (r *Renderer) Height() int {
	return r.lastHeight
}

// Stride returns the stride of the render buffers.
func (r *Renderer) Stride() int {
	if r.swapchain == nil {
		return 0
	}
	return r.swapchain.Stride()
}

// Format returns the pixel format.
func (r *Renderer) Format() client.WlShmFormat {
	return r.format
}

// Close destroys the renderer and frees resources.
func (r *Renderer) Close() error {
	if r.swapchain != nil {
		return r.swapchain.Close()
	}
	return nil
}

// SetManualMode enables or disables manual frame control.
// When enabled, the renderer will not automatically request the next frame
// after rendering. Call Render() to manually trigger renders.
func (r *Renderer) SetManualMode(enabled bool) {
	r.manualMode = enabled
}

// Render performs a complete render cycle: acquire buffer, render, and present.
// Should only be used in manual mode and after the surface is configured.
func (r *Renderer) ManualRender(time uint32) {
	// warn if not in manual mode
	if !r.manualMode {
		fmt.Println("Warning: ManualRender() called while not in manual mode")
	}
	r.render(time)
}

// Ready returns true if the renderer has received valid dimensions
// from the compositor and is ready to render.
func (r *Renderer) Ready() bool {
	return r.ready
}
