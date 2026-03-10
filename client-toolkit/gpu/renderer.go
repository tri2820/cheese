package gpu

import (
	"fmt"

	"github.com/tri2820/cheese/client-toolkit/dmabuf"
	"github.com/tri2820/cheese/client-toolkit/render"
	"github.com/tri2820/cheese/client-toolkit/surface"
)

// BufferSet owns one renderable DMA-BUF generation.
type BufferSet struct {
	// Infos describes the exported DMA-BUFs for this generation.
	Infos []dmabuf.BufferInfo

	// Destroy releases the backing GPU resources for this generation.
	Destroy func()
}

// RendererConfig configures a new GPU Renderer.
type RendererConfig struct {
	// State is the DMA-BUF protocol state.
	State *dmabuf.State

	// Target is the Window or LayerSurface to render to.
	Target render.RenderTarget

	// Buffers is the number of buffers for double/triple buffering (default 2).
	Buffers int

	// CreateBuffers creates one DMA-BUF generation for the configured size.
	CreateBuffers func(width, height, count int) (*BufferSet, error)

	// Render is called each frame with a free buffer index from the active generation.
	Render func(bufferIndex, width, height int, time uint32) error
}

type generation struct {
	width   int
	height  int
	set     *BufferSet
	buffers []*dmabuf.Buffer
}

func (g *generation) hasBusyBuffers() bool {
	for _, buf := range g.buffers {
		if buf != nil && buf.Busy() {
			return true
		}
	}
	return false
}

// Renderer handles high-level GPU rendering through DMA-BUF-backed wl_buffers.
type Renderer struct {
	state            *dmabuf.State
	surface          *surface.Surface
	target           render.RenderTarget
	bufferCount      int
	resizeHandlers   []func(width, height int)
	errorHandlers    []func(error)
	lastWidth        int
	lastHeight       int
	lastFrameTime    uint32
	waitingForBuffer bool
	manualMode       bool
	ready            bool
	active           *generation
	retired          []*generation
	onCreateBuffers  func(width, height, count int) (*BufferSet, error)
	onRender         func(bufferIndex, width, height int, time uint32) error
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
	if config.Buffers < 1 {
		config.Buffers = 2
	}

	r := &Renderer{
		state:           config.State,
		surface:         config.Target.Surface(),
		target:          config.Target,
		bufferCount:     config.Buffers,
		onCreateBuffers: config.CreateBuffers,
		onRender:        config.Render,
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
	if width <= 0 || height <= 0 {
		return
	}

	if r.active != nil && width == r.active.width && height == r.active.height {
		r.render(0)
		return
	}

	next, err := r.createGeneration(width, height)
	if err != nil {
		r.emitError(err)
		return
	}

	if r.active != nil {
		r.retired = append(r.retired, r.active)
	}

	r.active = next
	r.lastWidth = width
	r.lastHeight = height
	r.ready = true

	for _, fn := range append([]func(int, int){}, r.resizeHandlers...) {
		if fn != nil {
			fn(width, height)
		}
	}

	r.render(0)
	r.collectRetired()
}

func (r *Renderer) createGeneration(width, height int) (*generation, error) {
	set, err := r.onCreateBuffers(width, height, r.bufferCount)
	if err != nil {
		return nil, fmt.Errorf("create gpu buffers: %w", err)
	}
	if set == nil {
		return nil, fmt.Errorf("create gpu buffers returned nil BufferSet")
	}
	if len(set.Infos) != r.bufferCount {
		r.destroySet(set)
		return nil, fmt.Errorf("create gpu buffers returned %d buffers, expected %d", len(set.Infos), r.bufferCount)
	}

	gen := &generation{
		width:   width,
		height:  height,
		set:     set,
		buffers: make([]*dmabuf.Buffer, r.bufferCount),
	}

	for i := 0; i < r.bufferCount; i++ {
		buf, err := r.createBufferFn(width, height, set.Infos[i])
		if err != nil {
			r.destroyGeneration(gen)
			return nil, err
		}
		index := i
		buf.OnRelease(func() {
			r.handleBufferRelease(gen, index)
		})
		buf.UserData = i
		gen.buffers[i] = buf
	}

	return gen, nil
}

func (r *Renderer) handleBufferRelease(gen *generation, _ int) {
	if gen == nil {
		return
	}

	if gen != r.active {
		r.collectRetired()
		return
	}

	if r.waitingForBuffer && !r.manualMode {
		r.waitingForBuffer = false
		r.render(r.lastFrameTime)
	}

	r.collectRetired()
}

func (r *Renderer) collectRetired() {
	if len(r.retired) == 0 {
		return
	}

	next := r.retired[:0]
	for _, gen := range r.retired {
		if gen == nil {
			continue
		}
		if gen.hasBusyBuffers() {
			next = append(next, gen)
			continue
		}
		r.destroyGeneration(gen)
	}
	r.retired = next
}

func (r *Renderer) render(time uint32) {
	if r.surface == nil || r.active == nil || len(r.active.buffers) == 0 {
		return
	}
	if time != 0 {
		r.lastFrameTime = time
	}

	width := r.active.width
	height := r.active.height

	bufferIndex := -1
	for i, buf := range r.active.buffers {
		if buf != nil && !buf.Busy() {
			bufferIndex = i
			break
		}
	}
	if bufferIndex == -1 {
		r.waitingForBuffer = true
		return
	}
	r.waitingForBuffer = false

	buf := r.active.buffers[bufferIndex]

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

func (r *Renderer) destroySet(set *BufferSet) {
	if set == nil || set.Destroy == nil {
		return
	}
	set.Destroy()
}

func (r *Renderer) destroyGeneration(gen *generation) {
	if gen == nil {
		return
	}
	for _, buf := range gen.buffers {
		if buf != nil {
			if err := r.destroyBufferFn(buf); err != nil {
				r.emitError(fmt.Errorf("destroy dmabuf buffer: %w", err))
			}
		}
	}
	r.destroySet(gen.set)
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
	r.destroyGeneration(r.active)
	r.active = nil
	for _, gen := range r.retired {
		r.destroyGeneration(gen)
	}
	r.retired = nil
	r.ready = false
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
