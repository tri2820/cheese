package dmabuf

import (
	"fmt"

	"github.com/tri2820/cheese/client-toolkit/render"
	"github.com/tri2820/cheese/client-toolkit/surface"
)

// BufferInfo contains the DMA-BUF metadata for a single buffer.
type BufferInfo struct {
	Fd       int      // DMA-BUF file descriptor
	Stride   int      // Bytes per row
	Format   Format   // DRM format (e.g., FormatXRGB8888)
	Modifier Modifier // DRM modifier (e.g., ModLinear)
}

// RendererConfig configures a new DMA-BUF Renderer.
type RendererConfig struct {
	// State is the DMA-BUF protocol state
	State *State

	// Target is the Window or LayerSurface to render to
	Target render.RenderTarget

	// Buffers is the number of buffers for double/triple buffering (default 2)
	Buffers int

	// OnCreateBuffers is called to create GPU resources and return dmabuf metadata.
	// Called on first configure and on resize.
	// Returns metadata for each buffer (fd, stride, format, modifier).
	OnCreateBuffers func(width, height, count int) ([]BufferInfo, error)

	// OnRender is called each frame with a free buffer index.
	// The user should render to their GPU resource at this index.
	OnRender func(bufferIndex, width, height int, time uint32) error

	// OnDestroyBuffers is called before resize and on Close().
	// The user should clean up their GPU resources.
	OnDestroyBuffers func()
}

// Renderer handles high-level DMA-BUF rendering to any surface.
// It manages buffer lifecycle and the Wayland protocol dance,
// calling user callbacks for GPU operations.
type Renderer struct {
	state           *State
	surface         *surface.Surface
	target          render.RenderTarget
	buffers         []*Buffer
	bufferCount     int
	lastWidth       int
	lastHeight      int
	onCreateBuffers func(width, height, count int) ([]BufferInfo, error)
	onRender        func(bufferIndex, width, height int, time uint32) error
	onDestroyBuffers func()
}

// NewRenderer creates a new DMA-BUF renderer attached to a surface.
func NewRenderer(config RendererConfig) (*Renderer, error) {
	if config.State == nil {
		return nil, fmt.Errorf("state is required")
	}
	if config.Target == nil {
		return nil, fmt.Errorf("target is required")
	}
	if config.OnCreateBuffers == nil {
		return nil, fmt.Errorf("OnCreateBuffers is required")
	}
	if config.OnRender == nil {
		return nil, fmt.Errorf("OnRender is required")
	}
	if config.OnDestroyBuffers == nil {
		return nil, fmt.Errorf("OnDestroyBuffers is required")
	}
	if config.Buffers < 1 {
		config.Buffers = 2 // default to double buffering
	}

	r := &Renderer{
		state:           config.State,
		surface:         config.Target.Surface(),
		target:          config.Target,
		bufferCount:     config.Buffers,
		lastWidth:       0,
		lastHeight:      0,
		onCreateBuffers: config.OnCreateBuffers,
		onRender:        config.OnRender,
		onDestroyBuffers: config.OnDestroyBuffers,
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

	// Ask user to create GPU resources and get metadata
	infos, err := r.onCreateBuffers(width, height, r.bufferCount)
	if err != nil {
		fmt.Printf("failed to create buffers: %v\n", err)
		return
	}
	if len(infos) != r.bufferCount {
		fmt.Printf("OnCreateBuffers returned %d buffers, expected %d\n", len(infos), r.bufferCount)
		return
	}

	// Create wl_buffers from the metadata
	r.buffers = make([]*Buffer, r.bufferCount)
	for i := 0; i < r.bufferCount; i++ {
		info := infos[i]

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

	// Call user's render function
	if err := r.onRender(bufferIndex, width, height, time); err != nil {
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

// destroyBuffers destroys wl_buffers and tells user to clean up.
func (r *Renderer) destroyBuffers() {
	// Destroy wl_buffers first
	for _, buf := range r.buffers {
		if buf != nil {
			buf.Destroy()
		}
	}
	r.buffers = nil

	// Tell user to clean up GPU resources
	r.onDestroyBuffers()
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
