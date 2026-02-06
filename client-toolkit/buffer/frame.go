package buffer

import (
	"fmt"

	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/render"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
)

// Frame manages Wayland frame lifecycle for a surface.
// It handles frame callbacks, buffer management, and output tracking.
type Frame struct {
	swapchain    *Swapchain
	surface      *surface.Surface
	shm          *client.WlShm
	format       client.WlShmFormat
	buffers      int
	target       render.RenderTarget
	onRender     func(width, height int, time uint32, pixels []byte)
	onConfigured func(width, height int) // Called when frame gets first valid dimensions
	display      *display.Display        // For output lookups
	lastWidth    int
	lastHeight   int
	manualMode   bool            // when true, doesn't auto-request next frame
	ready        bool            // true when we've received valid dimensions
	output       *display.Output // Current output the surface is on
	outputX      int32           // Output position (cached from geometry events)
	outputY      int32
}

// FrameConfig configures a new Frame.
type FrameConfig struct {
	// Format is the pixel format
	Format client.WlShmFormat

	// Buffers is the number of buffers for double/triple buffering
	Buffers int
}

// NewFrame creates a new frame attached to a surface.
// Shm, Target, and Display are required positional arguments (compile-time enforced).
func NewFrame(shm *client.WlShm, target render.RenderTarget, display *display.Display, config FrameConfig) (*Frame, error) {
	if shm == nil {
		return nil, fmt.Errorf("shm is required")
	}
	if target == nil {
		return nil, fmt.Errorf("target is required")
	}
	if display == nil {
		return nil, fmt.Errorf("display is required")
	}
	if config.Buffers < 1 {
		config.Buffers = 2 // default to double buffering
	}

	f := &Frame{
		surface:    target.Surface(),
		shm:        shm,
		format:     config.Format,
		buffers:    config.Buffers,
		target:     target,
		lastWidth:  0, // Force swapchain creation on first render
		lastHeight: 0,
		display:    display,
	}

	// Set up frame handler
	target.Surface().SetFrameHandler(func(time uint32) {
		f.render(time)
	})

	// Set up configure handler
	target.SetConfigureHandler(func() {
		f.render(0)
	})

	// Set up enter handler to capture output and subscribe to position changes
	target.Surface().SetEnterHandler(func(wlOutput *client.WlOutput) {
		if output := display.OutputByWlOutput(wlOutput); output != nil {
			f.output = output
			f.outputX = output.X
			f.outputY = output.Y

			// Subscribe to geometry changes to track position over time
			output.SetGeometryHandler(func(ev client.WlOutputGeometryEvent) {
				f.outputX = ev.X
				f.outputY = ev.Y
			})
		}
	})

	// Set up leave handler to clear output reference
	target.Surface().SetLeaveHandler(func(wlOutput *client.WlOutput) {
		if f.output != nil && f.output.WlOutput() == wlOutput {
			f.output = nil
		}
	})

	return f, nil
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
func (f *Frame) OnRender(fn func(width, height int, time uint32, pixels []byte)) {
	f.onRender = fn
}

// OnConfigured sets a callback that fires when the renderer receives its first
// valid dimensions from the compositor. This happens once when the surface
// is configured.
//
// Usage:
//
//	renderer.OnConfigured(func(w, h int) {
//	    log.Printf("Surface configured: %dx%d", w, h)
//	})
func (f *Frame) OnConfigured(fn func(width, height int)) {
	f.onConfigured = fn
	// If already ready, call immediately
	if f.ready {
		fn(f.lastWidth, f.lastHeight)
	}
}

// render is the internal render handler.
func (f *Frame) render(time uint32) {
	width := f.target.Width()
	height := f.target.Height()

	// Recreate swapchain if dimensions changed
	if width != f.lastWidth || height != f.lastHeight {
		if f.swapchain != nil {
			f.swapchain.Close()
		}

		swapchain, err := NewSwapchain(SwapchainConfig{
			Shm:     f.shm,
			Buffers: f.buffers,
			Width:   width,
			Height:  height,
			Format:  f.format,
		})
		if err != nil {
			fmt.Printf("failed to create swapchain: %v\n", err)
			return
		}
		swapchain.SetSurface(f.surface)
		f.swapchain = swapchain
		f.lastWidth = width
		f.lastHeight = height

		// Let user know we are ready to render.
		f.ready = true

		// Fire configured callback on first ready
		if f.onConfigured != nil {
			f.onConfigured(width, height)
		}
	}

	if !f.ready {
		return
	}

	if f.onRender == nil {
		return
	}

	// Acquire buffer (may fail if all buffers are busy - that's OK, skip this frame)
	pixels, err := f.swapchain.Acquire()
	if err != nil {
		// No free buffer means compositor hasn't released yet - skip this frame
		// The next frame callback will try again
		return
	}

	// Call user's render function
	f.onRender(width, height, time, pixels)

	// Request frame callback BEFORE committing
	// This associates the callback with the upcoming commit
	// Skip if in manual mode (user controls when to request frames)
	if !f.manualMode {
		if err := f.surface.Frame(); err != nil {
			fmt.Printf("failed to request frame: %v\n", err)
		}
	}

	// Present the frame (includes commit)
	if err := f.swapchain.Present(); err != nil {
		fmt.Printf("failed to present: %v\n", err)
		return
	}
}

// Width returns the current width of the render buffers.
func (f *Frame) Width() int {
	return f.lastWidth
}

// Height returns the current height of the render buffers.
func (f *Frame) Height() int {
	return f.lastHeight
}

// Stride returns the stride of the render buffers.
func (f *Frame) Stride() int {
	if f.swapchain == nil {
		return 0
	}
	return f.swapchain.Stride()
}

// Format returns the pixel format.
func (f *Frame) Format() client.WlShmFormat {
	return f.format
}

// Close destroys the renderer and frees resources.
func (f *Frame) Close() error {
	if f.swapchain != nil {
		return f.swapchain.Close()
	}
	return nil
}

// SetManualMode enables or disables manual frame control.
// When enabled, the renderer will not automatically request the next frame
// after rendering. Call Render() to manually trigger renders.
func (f *Frame) SetManualMode(enabled bool) {
	f.manualMode = enabled
}

// Render performs a complete render cycle: acquire buffer, render, and present.
// Should only be used in manual mode and after the surface is configured.
func (f *Frame) ManualRender(time uint32) {
	// warn if not in manual mode
	if !f.manualMode {
		fmt.Println("Warning: ManualRender() called while not in manual mode")
	}
	f.render(time)
}

// Ready returns true if the renderer has received valid dimensions
// from the compositor and is ready to render.
func (f *Frame) Ready() bool {
	return f.ready
}

// OutputX returns the X position of the output this surface is on.
func (f *Frame) OutputX() int32 {
	return f.outputX
}

// OutputY returns the Y position of the output this surface is on.
func (f *Frame) OutputY() int32 {
	return f.outputY
}

// Output returns the current output this surface is on.
func (f *Frame) Output() *display.Output {
	return f.output
}

// SetOutputPosition sets the output position. Call this from OnOutputEnter
// when you receive wl_output enter events.
func (f *Frame) SetOutputPosition(x, y int32) {
	f.outputX = x
	f.outputY = y
}
