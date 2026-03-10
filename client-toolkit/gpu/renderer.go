package gpu

import (
	"fmt"

	"github.com/tri2820/cheese/client-toolkit/dmabuf"
	"github.com/tri2820/cheese/client-toolkit/render"
	"github.com/tri2820/cheese/client-toolkit/surface"
)

// RendererConfig configures a new GPU Renderer.
type RendererConfig struct {
	// State is the DMA-BUF protocol state.
	State *dmabuf.State

	// Target is the Window or LayerSurface to render to.
	Target render.RenderTarget

	// Buffers is the number of buffers for double/triple buffering (default 2).
	Buffers int

	// CreateBuffers is called to create GPU resources and return DMA-BUF metadata.
	// Called on first configure and on resize.
	CreateBuffers func(width, height, count int) ([]dmabuf.BufferInfo, error)

	// Render is called each frame with a free buffer index.
	Render func(bufferIndex, width, height int, time uint32) error

	// DestroyBuffers is called before resize and on Close().
	DestroyBuffers func()
}

// Renderer handles high-level GPU rendering through DMA-BUF-backed wl_buffers.
type Renderer struct {
	state            *dmabuf.State
	surface          *surface.Surface
	target           render.RenderTarget
	buffers          []*dmabuf.Buffer
	bufferCount      int
	resizeHandlers   []func(width, height int)
	errorHandlers    []func(error)
	lastWidth        int
	lastHeight       int
	manualMode       bool
	ready            bool
	resourcesActive  bool
	onCreateBuffers  func(width, height, count int) ([]dmabuf.BufferInfo, error)
	onRender         func(bufferIndex, width, height int, time uint32) error
	onDestroyBuffers func()
	createBufferFn   func(width, height int, info dmabuf.BufferInfo) (*dmabuf.Buffer, error)
	destroyBufferFn  func(*dmabuf.Buffer) error
}

// NewRenderer creates a new GPU renderer attached to a render target.
func NewRenderer(config RendererConfig) (*Renderer, error) {
	if config.State == nil {
		return nil, fmt.Errorf("state is required")
	}
	if config.Target == nil {
		return nil, fmt.Errorf("target is required")
	}
	if config.CreateBuffers == nil {
		return nil, fmt.Errorf("CreateBuffers is required")
	}
	if config.Render == nil {
		return nil, fmt.Errorf("Render is required")
	}
	if config.DestroyBuffers == nil {
		return nil, fmt.Errorf("DestroyBuffers is required")
	}
	if config.Buffers < 1 {
		config.Buffers = 2
	}

	r := &Renderer{
		state:            config.State,
		surface:          config.Target.Surface(),
		target:           config.Target,
		bufferCount:      config.Buffers,
		lastWidth:        0,
		lastHeight:       0,
		onCreateBuffers:  config.CreateBuffers,
		onRender:         config.Render,
		onDestroyBuffers: config.DestroyBuffers,
	}
	r.createBufferFn = r.createBuffer
	r.destroyBufferFn = r.destroyBuffer

	config.Target.Surface().OnFrame(func(time uint32) {
		r.render(time)
	})

	config.Target.OnConfigure(func() {
		r.handleConfigure()
	})

	return r, nil
}

// OnResize registers a handler that fires when the renderer size changes.
// It is future-only and does not replay the current size.
func (r *Renderer) OnResize(fn func(width, height int)) {
	if fn == nil {
		return
	}
	r.resizeHandlers = append(r.resizeHandlers, fn)
}

// OnError registers a handler for internal renderer lifecycle errors.
func (r *Renderer) OnError(fn func(error)) {
	if fn == nil {
		return
	}
	r.errorHandlers = append(r.errorHandlers, fn)
}

// SetManualMode enables or disables manual frame control.
func (r *Renderer) SetManualMode(enabled bool) {
	r.manualMode = enabled
}

// ManualRender performs a complete render cycle in manual mode.
func (r *Renderer) ManualRender(time uint32) {
	r.render(time)
}

// Ready returns true if the renderer has created buffers for the current size.
func (r *Renderer) Ready() bool {
	return r.ready
}

func (r *Renderer) emitError(err error) {
	if err == nil {
		return
	}
	for _, fn := range append([]func(error){}, r.errorHandlers...) {
		if fn != nil {
			fn(err)
		}
	}
}

func (r *Renderer) handleConfigure() {
	width := r.target.Width()
	height := r.target.Height()

	if width == r.lastWidth && height == r.lastHeight {
		r.render(0)
		return
	}

	r.ready = false
	r.destroyBuffers()

	infos, err := r.onCreateBuffers(width, height, r.bufferCount)
	if err != nil {
		r.emitError(fmt.Errorf("create gpu buffers: %w", err))
		return
	}
	r.resourcesActive = true
	if len(infos) != r.bufferCount {
		r.emitError(fmt.Errorf("create gpu buffers returned %d buffers, expected %d", len(infos), r.bufferCount))
		r.destroyBuffers()
		return
	}

	r.buffers = make([]*dmabuf.Buffer, r.bufferCount)
	for i := 0; i < r.bufferCount; i++ {
		info := infos[i]

		buf, err := r.createBufferFn(width, height, info)
		if err != nil {
			r.emitError(err)
			r.destroyBuffers()
			return
		}
		r.buffers[i] = buf
		r.buffers[i].UserData = i
	}

	r.lastWidth = width
	r.lastHeight = height
	r.ready = true

	for _, fn := range append([]func(int, int){}, r.resizeHandlers...) {
		if fn != nil {
			fn(width, height)
		}
	}

	r.render(0)
}

func (r *Renderer) render(time uint32) {
	if len(r.buffers) == 0 {
		return
	}

	width := r.lastWidth
	height := r.lastHeight

	bufferIndex := -1
	for i, buf := range r.buffers {
		if buf != nil && !buf.Busy() {
			bufferIndex = i
			break
		}
	}
	if bufferIndex == -1 {
		return
	}

	buf := r.buffers[bufferIndex]

	if err := r.onRender(bufferIndex, width, height, time); err != nil {
		r.emitError(fmt.Errorf("render gpu buffer %d: %w", bufferIndex, err))
		return
	}

	if err := r.surface.Attach(buf.WlBuffer(), 0, 0); err != nil {
		r.emitError(fmt.Errorf("attach dmabuf buffer: %w", err))
		return
	}
	if err := r.surface.Damage(0, 0, int32(width), int32(height)); err != nil {
		r.emitError(fmt.Errorf("damage surface: %w", err))
		return
	}
	if !r.manualMode {
		if err := r.surface.Frame(); err != nil {
			r.emitError(fmt.Errorf("request frame callback: %w", err))
		}
	}
	if err := r.surface.Commit(); err != nil {
		r.emitError(fmt.Errorf("commit surface: %w", err))
		return
	}

	buf.MarkBusy()
}

func (r *Renderer) destroyBuffers() {
	for _, buf := range r.buffers {
		if buf != nil {
			if err := r.destroyBufferFn(buf); err != nil {
				r.emitError(fmt.Errorf("destroy dmabuf buffer: %w", err))
			}
		}
	}
	r.buffers = nil
	r.ready = false

	if r.resourcesActive {
		r.onDestroyBuffers()
		r.resourcesActive = false
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

// Close destroys the renderer and frees resources.
func (r *Renderer) Close() error {
	r.destroyBuffers()
	return nil
}

func (r *Renderer) createBuffer(width, height int, info dmabuf.BufferInfo) (*dmabuf.Buffer, error) {
	params, err := r.state.CreateParams()
	if err != nil {
		return nil, fmt.Errorf("create dmabuf params: %w", err)
	}

	if err := params.Add(info.Fd, 0, 0, uint32(info.Stride), info.Modifier); err != nil {
		_ = params.Destroy()
		return nil, fmt.Errorf("add dmabuf plane: %w", err)
	}

	wlBuffer, err := params.CreateImmed(width, height, info.Format, 0)
	if err != nil {
		return nil, fmt.Errorf("create dmabuf buffer: %w", err)
	}

	return dmabuf.NewBuffer(wlBuffer, width, height, info.Format), nil
}

func (r *Renderer) destroyBuffer(buf *dmabuf.Buffer) error {
	if buf == nil {
		return nil
	}
	return buf.Destroy()
}
