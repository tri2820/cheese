package ui

import (
	"sync"

	"github.com/lithdew/casso"
	"github.com/tri2820/cheese/client-toolkit/shm"
)

// Priority represents constraint strength (alias for casso.Priority)
type Priority = casso.Priority

const (
	Required Priority = casso.Required
	Strong   Priority = casso.Strong
	Weak     Priority = casso.Weak
)

// RenderFunc is a function that renders to a layoutitemport's pixel buffer.
type RenderFunc func(width, height int, time uint32, pixels []byte)

// Layout wraps a casso.Solver with convenient methods for our types
type Layout struct {
	inner      *casso.Solver
	vars       map[casso.Symbol]*exprState // symbol → shared state
	renderReq  chan struct{}               // Render request channel (batched)
	resolving  bool                        // In constraint resolution pass
	resolveMut sync.Mutex                  // Protects resolving flag
	stopRender chan struct{}               // Stop render loop
	startOnce  sync.Once
	stopOnce   sync.Once
	errorMut   sync.Mutex
	onError    []func(error)

	frames       []*shm.Frame         // Frames to notify when render is needed
	maskForFrame map[*shm.Frame]*Mask // Maps frame to its mask
	framesMut    sync.Mutex           // Protects frames slice and maskForFrame map
	dirty        bool                 // True when rendering is needed
	dirtyMut     sync.Mutex           // Protects dirty flag

	widgets    []*Widget  // Widgets to manage
	widgetsMut sync.Mutex // Protects widgets slice
}

// NewLayout creates a new constraint solver
func NewLayout() *Layout {
	return &Layout{
		inner:        casso.NewSolver(),
		vars:         make(map[casso.Symbol]*exprState),
		renderReq:    make(chan struct{}, 1),
		stopRender:   make(chan struct{}),
		maskForFrame: make(map[*shm.Frame]*Mask),
	}
}

// AddFrame adds a frame to the layout.
// Sets up the frame's OnResize and render handlers automatically.
// The onResize callback is invoked after the frame receives a configured size.
func (l *Layout) AddFrame(frame *shm.Frame, mask *Mask, onResize func(w, h int)) {
	// Store frame for render notifications and map to mask
	l.framesMut.Lock()
	l.frames = append(l.frames, frame)
	l.maskForFrame[frame] = mask
	l.framesMut.Unlock()

	// Set up configure handler
	frame.OnResize(func(w, h int) {
		// Set up render function for this frame
		renderFunc := func(frameW, frameH int, frameTime uint32, pixels []byte) {
			l.renderFrame(frame, pixels, frameW, frameH)
		}

		frame.SetRender(renderFunc)

		// Call app's resize callback if provided
		if onResize != nil {
			onResize(w, h)
		}
	})
}

// RemoveFrame removes a frame from the layout's tracking.
// Called when a mask is destroyed to prevent rendering to closed frames.
func (l *Layout) RemoveFrame(frame *shm.Frame) {
	l.framesMut.Lock()
	defer l.framesMut.Unlock()

	// Remove from frames slice
	for i, f := range l.frames {
		if f == frame {
			l.frames = append(l.frames[:i], l.frames[i+1:]...)
			break
		}
	}

	// Remove from maskForFrame map
	delete(l.maskForFrame, frame)
}

// renderFrame renders all contents from the widget to a single frame's pixel buffer.
func (l *Layout) renderFrame(frame *shm.Frame, pixels []byte, width, height int) {
	stride := width * 4

	// Get mask for this frame
	l.framesMut.Lock()
	mask := l.maskForFrame[frame]
	l.framesMut.Unlock()

	if mask == nil {
		return
	}

	// Get widget that owns this mask
	widget := mask.widget
	if widget == nil {
		return
	}

	// Get DPI from output
	dpi := 96.0
	if output := frame.Output(); output != nil {
		dpi = output.DPI()
		if dpi == 0 {
			dpi = 96
		}
	}

	// Clear frame to transparent
	for i := range pixels {
		pixels[i] = 0
	}

	// Get contents
	widget.mu.RLock()
	contents := widget.contents
	widget.mu.RUnlock()
	contentWidth := widget.ContentWidth()
	contentHeight := widget.ContentHeight()

	// Create framebuffer at (0,0) filling entire surface
	// Mask position is controlled by layer margin (via reactive Effect)
	fb := Framebuffer{
		pixels: pixels,
		stride: stride,
		width:  width,
		height: height,
	}

	// If ContentWidth_at96DPI is set, use DPI-normalized rendering
	if contentWidth > 0 && contentHeight > 0 {
		// Calculate DPI scale
		scale := dpi / 96.0

		// Get clip origin from mask (in widget coordinates)
		// The visible region size is determined by mask Width()/Height() constraints
		clipX := mask.ClipX
		clipY := mask.ClipY

		// Convert clip origin to physical pixels
		srcX := int(float64(clipX) * scale)
		srcY := int(float64(clipY) * scale)

		// Create draw context with coordinate transformation
		ctx := DrawContext{
			Framebuffer: fb,
			OffsetX:     srcX,
			OffsetY:     srcY,
			Scale:       scale,
		}

		// Render all contents directly to framebuffer
		for _, content := range contents {
			content.Draw(ctx)
		}
	} else {
		// No DPI normalization - render directly to framebuffer with no transform
		ctx := DrawContext{
			Framebuffer: fb,
			OffsetX:     0,
			OffsetY:     0,
			Scale:       1.0,
		}
		for _, content := range contents {
			content.Draw(ctx)
		}
	}

	// Clear dirty flag
	l.dirtyMut.Lock()
	l.dirty = false
	l.dirtyMut.Unlock()
}

// RequestRender schedules a render request.
// Batches requests during constraint resolution - only one render per pass.
func (l *Layout) RequestRender() {
	l.dirtyMut.Lock()
	l.dirty = true
	l.dirtyMut.Unlock()

	l.resolveMut.Lock()
	if l.resolving {
		// Don't render yet, we're in a constraint resolution pass
		l.resolveMut.Unlock()
		return
	}
	l.resolveMut.Unlock()

	select {
	case l.renderReq <- struct{}{}:
	default:
		// Already scheduled
	}
}

// RenderLoop runs the automatic render loop.
// Call this in a goroutine to trigger renders to all frames when needed.
func (l *Layout) RenderLoop() {
	for {
		select {
		case <-l.renderReq:
			// Trigger all registered frames to render
			l.framesMut.Lock()
			frames := l.frames
			l.framesMut.Unlock()
			for _, f := range frames {
				if f.Ready() {
					f.ManualRender(0)
				}
			}
		case <-l.stopRender:
			return
		}
	}
}

// addWidget adds a widget to the layout.
func (l *Layout) addWidget(w *Widget) {
	l.widgetsMut.Lock()
	defer l.widgetsMut.Unlock()
	l.widgets = append(l.widgets, w)
}

// removeWidget removes a widget from the layout.
func (l *Layout) removeWidget(w *Widget) {
	l.widgetsMut.Lock()
	defer l.widgetsMut.Unlock()
	for i, widget := range l.widgets {
		if widget == w {
			l.widgets = append(l.widgets[:i], l.widgets[i+1:]...)
			return
		}
	}
}

// removeVar removes a variable from the solver and vars map.
func (l *Layout) removeVar(sym casso.Symbol) {
	// Remove from casso solver
	_ = l.inner.RemoveVariable(sym)
	// Remove from vars map
	delete(l.vars, sym)
}

func (l *Layout) removeLayoutItem(item *LayoutItem) {
	if item == nil {
		return
	}

	l.removeVar(item.Left.state.symbol)
	l.removeVar(item.Top.state.symbol)
	l.removeVar(item.Right.state.symbol)
	l.removeVar(item.Bottom.state.symbol)
}

func (l *Layout) reportError(err error) {
	if err == nil {
		return
	}

	l.errorMut.Lock()
	handlers := append([]func(error){}, l.onError...)
	l.errorMut.Unlock()

	for _, handler := range handlers {
		if handler != nil {
			handler(err)
		}
	}
}

// StopRenderLoop stops the automatic render loop.
func (l *Layout) StopRenderLoop() {
	l.stopOnce.Do(func() {
		close(l.stopRender)
	})
}

// Start launches the automatic render loop in a goroutine once.
func (l *Layout) Start() {
	l.startOnce.Do(func() {
		go l.RenderLoop()
	})
}

// Close stops the layout's render loop.
func (l *Layout) Close() {
	l.StopRenderLoop()
}

// OnError registers an asynchronous ui error handler.
func (l *Layout) OnError(fn func(error)) {
	if fn == nil {
		return
	}

	l.errorMut.Lock()
	defer l.errorMut.Unlock()
	l.onError = append(l.onError, fn)
}

// ConstraintHandle represents a handle to a constraint that can be removed
type ConstraintHandle struct {
	layout  *Layout
	markers []casso.Symbol
}

// Remove removes these constraints from the solver
func (h ConstraintHandle) Remove() {
	for _, marker := range h.markers {
		h.layout.inner.RemoveConstraint(marker)
	}
}

// Add adds constraints to the solver
// Accepts both single constraints and slices of constraints
// Uses the constraint's stored priority (defaults to Strong if not set)
// Returns a handle that can be used to remove the constraints
func (l *Layout) Add(constraints ...Constraints) ConstraintHandle {
	return l.AddWithPriority(Strong, constraints...)
}

// AddWithPriority adds constraints with a specific priority (overrides stored priority)
// Returns a handle that can be used to remove the constraints
func (l *Layout) AddWithPriority(priority Priority, constraints ...Constraints) ConstraintHandle {
	var markers []casso.Symbol
	for _, group := range constraints {
		for _, c := range group {
			marker, _ := l.inner.AddConstraintWithPriority(priority, c.ToCasso())
			markers = append(markers, marker)
		}
	}
	// Sync values from solver after adding constraints
	l.syncFromSolver()
	return ConstraintHandle{layout: l, markers: markers}
}

// NewVar creates a new variable expression
func (l *Layout) NewVar() Expr {
	state := &exprState{
		symbol: casso.New(),
		value:  0,
	}
	l.vars[state.symbol] = state

	// Capture symbol for closure
	symCopy := state.symbol
	// Watch for changes - immediate resolve on every Set()
	state.addOnChange(func() {
		l.resolve(symCopy)
	})

	return Expr{kind: exprVar, state: state, layout: l}
}

// syncFromSolver reads all values from casso and update expr states.
// Called after adding constraints to immediately sync values.
func (l *Layout) syncFromSolver() {
	l.resolveMut.Lock()
	l.resolving = true
	l.resolveMut.Unlock()

	// Get resolved values from casso and update expressions
	// Only trigger quiet observers for variables whose values actually changed
	for sym, state := range l.vars {
		newValue := l.inner.Val(sym)
		if newValue != state.value {
			state.value = newValue
			for _, obs := range append([]exprObserver(nil), state.onChangeQuiet...) {
				if obs.fn != nil {
					obs.fn()
				}
			}
		}
	}

	l.resolveMut.Lock()
	l.resolving = false
	l.resolveMut.Unlock()

	// Request render after constraint pass completes
	l.RequestRender()
}

// resolve syncs expr values → casso → expr values
// Runs immediately on every Expr.Set()
func (l *Layout) resolve(changedSymbol casso.Symbol) {
	l.resolveMut.Lock()
	l.resolving = true
	l.resolveMut.Unlock()

	state := l.vars[changedSymbol]
	// Edit with Weak priority so Strong constraints can override
	l.inner.Edit(changedSymbol, Weak)
	l.inner.Suggest(changedSymbol, state.value)

	l.resolveMut.Lock()
	l.resolving = false
	l.resolveMut.Unlock()

	// Sync all values from solver
	l.syncFromSolver()
}

// NewLayoutItem creates layoutitem via layout
func (l *Layout) NewLayoutItem() *LayoutItem {
	return &LayoutItem{
		layout: l,
		Left:   l.NewVar(),
		Right:  l.NewVar(),
		Top:    l.NewVar(),
		Bottom: l.NewVar(),
	}
}
