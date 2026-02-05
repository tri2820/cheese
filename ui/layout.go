package ui

import (
	"sync"

	"github.com/lithdew/casso"
	"github.com/tri2820/cheese/client-toolkit/buffer"
	"github.com/tri2820/cheese/signals"
)

// Priority represents constraint strength (alias for casso.Priority)
type Priority = casso.Priority

const (
	Required Priority = casso.Required
	Strong   Priority = casso.Strong
	Weak     Priority = casso.Weak
)

// Viewport represents a region of the virtual desktop.
type Viewport struct {
	X      int // X position in virtual space
	Y      int // Y position in virtual space
	Width  int // Width of this viewport
	Height int // Height of this viewport
}

// RenderFunc is a function that renders to a viewport's pixel buffer.
type RenderFunc func(width, height int, time uint32, pixels []byte)

// Widget is an interface for renderable UI elements.
type Widget interface {
	Draw(pixels []byte, stride, width, height int)
}

// Layout wraps a casso.Solver with convenient methods for our types
type Layout struct {
	inner       *casso.Solver
	vars        map[casso.Symbol]*exprState // symbol → shared state
	renderReq   chan struct{}               // Render request channel (batched)
	resolving   bool                        // In constraint resolution pass
	resolveMut  sync.Mutex                  // Protects resolving flag
	stopRender  chan struct{}               // Stop render loop

	// Virtual desktop
	virtualWidth  signals.Signal[int] // Virtual desktop width
	virtualHeight signals.Signal[int] // Virtual desktop height
	virtualBuffer []byte              // Virtual pixel buffer
	virtualStride int                 // Bytes per row in virtual buffer
	viewports     []*Viewport         // Registered viewports
	viewportsMut  sync.Mutex          // Protects viewports slice
	frames        []*buffer.Frame     // Frames to notify when render is needed
	framesMut     sync.Mutex          // Protects frames slice
	dirty         bool                // True when virtual buffer needs re-rendering
	dirtyMut      sync.Mutex          // Protects dirty flag

	widgets     []Widget // Widgets to render
	widgetsMut  sync.Mutex // Protects widgets slice
}

// NewLayout creates a new constraint solver
func NewLayout() *Layout {
	return &Layout{
		inner:         casso.NewSolver(),
		vars:          make(map[casso.Symbol]*exprState),
		renderReq:     make(chan struct{}, 1),
		stopRender:    make(chan struct{}),
		virtualWidth:  signals.New(0),
		virtualHeight: signals.New(0),
	}
}

// AddFrame adds a frame to the layout.
// Sets up the frame's OnConfigured and OnRender handlers automatically.
// The onConfigured callback is invoked after the viewport is created, with the viewport dimensions.
func (l *Layout) AddFrame(frame *buffer.Frame, onConfigured func(w, h int)) {
	// Store frame for render notifications
	l.framesMut.Lock()
	l.frames = append(l.frames, frame)
	l.framesMut.Unlock()

	// Set up configure handler to create viewport when frame gets dimensions
	frame.OnConfigured(func(w, h int) {
		x := int(frame.OutputX())
		y := int(frame.OutputY())

		l.viewportsMut.Lock()
		viewport := &Viewport{
			X:      x,
			Y:      y,
			Width:  w,
			Height: h,
		}
		l.viewports = append(l.viewports, viewport)
		l.viewportsMut.Unlock()

		// Expand virtual buffer if needed
		l.expandVirtualBuffer(x+w, y+h)

		// Create render function for this viewport
		renderFunc := func(frameW, frameH int, frameTime uint32, pixels []byte) {
			// Render to virtual buffer if dirty
			l.maybeRenderToVirtual()

			// Blit from virtual buffer to this viewport's pixels
			l.blitToViewport(pixels, frameW, frameH, x, y)
		}

		// Set render function on frame
		frame.OnRender(renderFunc)

		// Call app's configured callback if provided
		if onConfigured != nil {
			onConfigured(w, h)
		}
	})
}

// expandVirtualBuffer enlarges the virtual buffer to accommodate the given dimensions.
func (l *Layout) expandVirtualBuffer(minWidth, minHeight int) {
	currentW := l.virtualWidth.Get()
	currentH := l.virtualHeight.Get()

	newW := currentW
	newH := currentH

	if minWidth > newW {
		newW = minWidth
	}
	if minHeight > newH {
		newH = minHeight
	}

	if newW != currentW || newH != currentH {
		// Allocate new virtual buffer
		l.virtualWidth.Set(newW)
		l.virtualHeight.Set(newH)
		l.virtualStride = newW * 4
		l.virtualBuffer = make([]byte, l.virtualStride*newH)
	}
}

// blitToViewport copies a region from virtual buffer to viewport pixels.
func (l *Layout) blitToViewport(dstPixels []byte, dstW, dstH, srcX, srcY int) {
	if l.virtualBuffer == nil {
		return
	}

	vw := l.virtualWidth.Get()
	vh := l.virtualHeight.Get()

	// Calculate intersection
	srcX2 := srcX
	srcY2 := srcY
	if srcX2 < 0 {
		srcX2 = 0
	}
	if srcY2 < 0 {
		srcY2 = 0
	}

	srcW := dstW
	srcH := dstH
	if srcX2+srcW > vw {
		srcW = vw - srcX2
	}
	if srcY2+srcH > vh {
		srcH = vh - srcY2
	}

	if srcW <= 0 || srcH <= 0 {
		return
	}

	dstStride := dstW * 4

	// Blit row by row
	for y := 0; y < srcH; y++ {
		srcOffset := (srcY2+y)*l.virtualStride + srcX2*4
		dstOffset := y * dstStride
		copy(dstPixels[dstOffset:dstOffset+srcW*4], l.virtualBuffer[srcOffset:srcOffset+srcW*4])
	}
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
// Call this in a goroutine to trigger renders to all viewports when needed.
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

// renderToVirtual renders all widgets to the virtual buffer.
func (l *Layout) renderToVirtual() {
	if l.virtualBuffer == nil {
		return
	}

	w := l.virtualWidth.Get()
	h := l.virtualHeight.Get()

	if w <= 0 || h <= 0 {
		return
	}

	// Draw all widgets to virtual buffer
	l.widgetsMut.Lock()
	widgets := l.widgets
	l.widgetsMut.Unlock()

	for _, widget := range widgets {
		widget.Draw(l.virtualBuffer, l.virtualStride, w, h)
	}

	l.dirtyMut.Lock()
	l.dirty = false
	l.dirtyMut.Unlock()
}

// addWidget adds a widget to the layout for rendering.
func (l *Layout) addWidget(w Widget) {
	l.widgetsMut.Lock()
	l.widgets = append(l.widgets, w)
	l.widgetsMut.Unlock()
}

// maybeRenderToVirtual executes draw commands only if dirty flag is set.
func (l *Layout) maybeRenderToVirtual() {
	l.dirtyMut.Lock()
	dirty := l.dirty
	l.dirtyMut.Unlock()

	if dirty {
		l.renderToVirtual()
	}
}

// StopRenderLoop stops the automatic render loop.
func (l *Layout) StopRenderLoop() {
	close(l.stopRender)
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
	var markers []casso.Symbol
	for _, group := range constraints {
		for _, c := range group {
			priority := c.priority
			if priority == 0 {
				priority = Strong
			}
			marker, _ := l.inner.AddConstraintWithPriority(priority, c.ToCasso())
			markers = append(markers, marker)
		}
	}
	return ConstraintHandle{layout: l, markers: markers}
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
	state.onChange = append(state.onChange, func() {
		l.resolve(symCopy)
	})

	return Expr{kind: exprVar, state: state, layout: l}
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

	// Get resolved values from casso and update expressions
	// Use SetQuiet to avoid triggering OnChange (prevents infinite loop)
	for sym, state := range l.vars {
		newVal := l.inner.Val(sym)
		if newVal != state.value {
			state.value = newVal
			// Trigger quiet observers only
			for _, fn := range state.onChangeQuiet {
				fn()
			}
		}
	}

	l.resolveMut.Lock()
	l.resolving = false
	l.resolveMut.Unlock()

	// Request render after constraint pass completes
	l.RequestRender()
}

// NewElement creates element via layout
func (l *Layout) NewElement() *Element {
	return &Element{
		layout: l,
		Left:   l.NewVar(),
		Right:  l.NewVar(),
		Top:    l.NewVar(),
		Bottom: l.NewVar(),
	}
}

// Inner returns the underlying casso.Solver for advanced use
func (l *Layout) Inner() *casso.Solver {
	return l.inner
}

// VirtualWidth returns the virtual desktop width signal.
func (l *Layout) VirtualWidth() signals.Signal[int] {
	return l.virtualWidth
}

// VirtualHeight returns the virtual desktop height signal.
func (l *Layout) VirtualHeight() signals.Signal[int] {
	return l.virtualHeight
}
