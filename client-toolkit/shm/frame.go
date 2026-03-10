package shm

import (
	"errors"
	"fmt"

	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/render"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
)

// Frame manages SHM-backed frame lifecycle for a render target.
// It handles frame callbacks, buffer management, and output tracking.
type Frame struct {
	swapchain      *Swapchain
	surface        *surface.Surface
	shm            *client.WlShm
	format         client.WlShmFormat
	buffers        int
	target         render.RenderTarget
	onRender       func(width, height int, time uint32, pixels []byte)
	resizeHandlers []func(width, height int)
	errorHandlers  []func(error)
	lastWidth      int
	lastHeight     int
	manualMode     bool
	ready          bool
	output         *display.Output
}

// FrameConfig configures a new Frame.
type FrameConfig struct {
	// Format is the pixel format.
	Format client.WlShmFormat

	// Buffers is the number of buffers for double/triple buffering.
	Buffers int
}

// NewFrame creates a new SHM frame helper attached to a render target.
// Shm, Target, and Display are required positional arguments.
func NewFrame(wlShm *client.WlShm, target render.RenderTarget, disp *display.Display, config FrameConfig) (*Frame, error) {
	if wlShm == nil {
		return nil, fmt.Errorf("shm is required")
	}
	if target == nil {
		return nil, fmt.Errorf("target is required")
	}
	if disp == nil {
		return nil, fmt.Errorf("display is required")
	}
	if config.Buffers < 1 {
		config.Buffers = 2
	}

	f := &Frame{
		surface:    target.Surface(),
		shm:        wlShm,
		format:     config.Format,
		buffers:    config.Buffers,
		target:     target,
		lastWidth:  0,
		lastHeight: 0,
	}

	target.Surface().OnFrame(func(time uint32) {
		f.render(time)
	})

	target.OnConfigure(func() {
		f.render(0)
	})

	target.Surface().OnEnter(func(wlOutput *client.WlOutput) {
		if output := disp.OutputByWlOutput(wlOutput); output != nil {
			f.output = output
		}
	})

	target.Surface().OnLeave(func(wlOutput *client.WlOutput) {
		if f.output != nil && f.output.WlOutput() == wlOutput {
			f.output = nil
		}
	})

	return f, nil
}

// SetRender sets the render delegate.
func (f *Frame) SetRender(fn func(width, height int, time uint32, pixels []byte)) {
	f.onRender = fn
}

// OnResize registers a handler that fires when the compositor configures a new
// surface size. It is future-only and does not replay the current size.
func (f *Frame) OnResize(fn func(width, height int)) {
	if fn == nil {
		return
	}
	f.resizeHandlers = append(f.resizeHandlers, fn)
}

// OnError registers a handler for internal frame lifecycle errors.
func (f *Frame) OnError(fn func(error)) {
	if fn == nil {
		return
	}
	f.errorHandlers = append(f.errorHandlers, fn)
}

func (f *Frame) emitError(err error) {
	if err == nil {
		return
	}
	for _, fn := range append([]func(error){}, f.errorHandlers...) {
		if fn != nil {
			fn(err)
		}
	}
}

func (f *Frame) render(time uint32) {
	width := f.target.Width()
	height := f.target.Height()

	if width != f.lastWidth || height != f.lastHeight {
		f.ready = false

		if f.swapchain != nil {
			if err := f.swapchain.Close(); err != nil {
				f.emitError(fmt.Errorf("close swapchain: %w", err))
			}
		}

		swapchain, err := NewSwapchain(SwapchainConfig{
			Shm:     f.shm,
			Buffers: f.buffers,
			Width:   width,
			Height:  height,
			Format:  f.format,
		})
		if err != nil {
			f.emitError(fmt.Errorf("create swapchain: %w", err))
			return
		}
		swapchain.SetSurface(f.surface)
		f.swapchain = swapchain
		f.lastWidth = width
		f.lastHeight = height
		f.ready = true

		for _, fn := range append([]func(int, int){}, f.resizeHandlers...) {
			if fn != nil {
				fn(width, height)
			}
		}
	}

	if !f.ready || f.onRender == nil {
		return
	}

	pixels, err := f.swapchain.Acquire()
	if err != nil {
		if !errors.Is(err, ErrNoFreeBuffer) {
			f.emitError(fmt.Errorf("acquire swapchain buffer: %w", err))
		}
		return
	}

	f.onRender(width, height, time, pixels)

	if !f.manualMode {
		if err := f.surface.Frame(); err != nil {
			f.emitError(fmt.Errorf("request frame callback: %w", err))
		}
	}

	if err := f.swapchain.Present(); err != nil {
		f.emitError(fmt.Errorf("present frame: %w", err))
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

// Close destroys the frame helper and frees resources.
func (f *Frame) Close() error {
	if f.swapchain != nil {
		return f.swapchain.Close()
	}
	return nil
}

// SetManualMode enables or disables manual frame control.
func (f *Frame) SetManualMode(enabled bool) {
	f.manualMode = enabled
}

// ManualRender performs a complete render cycle in manual mode.
func (f *Frame) ManualRender(time uint32) {
	f.render(time)
}

// Ready returns true if the helper has received valid dimensions.
func (f *Frame) Ready() bool {
	return f.ready
}

// Output returns the current output this surface is on.
func (f *Frame) Output() *display.Output {
	return f.output
}
