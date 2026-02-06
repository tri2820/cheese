package ui

import (
	"sync"

	"github.com/lithdew/casso"
	"github.com/tri2820/cheese/client-toolkit/buffer"
	"github.com/tri2820/cheese/client-toolkit/display"
)

// Priority represents constraint strength (alias for casso.Priority)
type Priority = casso.Priority

const (
	Required Priority = casso.Required
	Strong   Priority = casso.Strong
	Weak     Priority = casso.Weak
)

// RenderFunc is a function that renders to a viewport's pixel buffer.
type RenderFunc func(width, height int, time uint32, pixels []byte)

// Rect represents a rectangle for clipping.
type Rect struct {
	X, Y, W, H int
}

// Framebuffer represents a pixel buffer that views can draw into.
// The origin (0,0) is at the visible region of the view in the frame.
type Framebuffer struct {
	pixels []byte
	stride int
	width  int
	height int
}

// SetPixel sets a single pixel at the given coordinates.
func (fb Framebuffer) SetPixel(x, y int, r, g, b, a uint8) {
	if x < 0 || x >= fb.width || y < 0 || y >= fb.height {
		return
	}
	offset := y*fb.stride + x*4
	fb.pixels[offset] = b
	fb.pixels[offset+1] = g
	fb.pixels[offset+2] = r
	fb.pixels[offset+3] = a
}

// Width returns the framebuffer width.
func (fb Framebuffer) Width() int { return fb.width }

// Height returns the framebuffer height.
func (fb Framebuffer) Height() int { return fb.height }

// Drawable is an interface for renderable UI elements.
// Drawables draw in their local coordinate space (0,0 = top-left corner).
// Layout handles coordinate translation, clipping, and DPI.
type Drawable interface {
	Draw(fb Framebuffer, dpi float64)
	GetView() *View
}

// Layout wraps a casso.Solver with convenient methods for our types
type Layout struct {
	inner      *casso.Solver
	vars       map[casso.Symbol]*exprState // symbol → shared state
	renderReq  chan struct{}               // Render request channel (batched)
	resolving  bool                        // In constraint resolution pass
	resolveMut sync.Mutex                  // Protects resolving flag
	stopRender chan struct{}               // Stop render loop

	frames    []*buffer.Frame // Frames to notify when render is needed
	framesMut sync.Mutex      // Protects frames slice
	dirty     bool            // True when rendering is needed
	dirtyMut  sync.Mutex      // Protects dirty flag

	drawables    []Drawable // Drawables to render
	drawablesMut sync.Mutex // Protects drawables slice
}

// NewLayout creates a new constraint solver
func NewLayout() *Layout {
	return &Layout{
		inner:      casso.NewSolver(),
		vars:       make(map[casso.Symbol]*exprState),
		renderReq:  make(chan struct{}, 1),
		stopRender: make(chan struct{}),
	}
}

// AddFrame adds a frame to the layout.
// Sets up the frame's OnConfigured and OnRender handlers automatically.
// The onConfigured callback is invoked after the frame is configured, with the frame dimensions.
func (l *Layout) AddFrame(frame *buffer.Frame, onConfigured func(w, h int)) {
	// Store frame for render notifications
	l.framesMut.Lock()
	l.frames = append(l.frames, frame)
	l.framesMut.Unlock()

	// Set up configure handler
	frame.OnConfigured(func(w, h int) {
		// Set up render function for this frame
		renderFunc := func(frameW, frameH int, frameTime uint32, pixels []byte) {
			l.renderFrame(frame, pixels, frameW, frameH)
		}

		frame.OnRender(renderFunc)

		// Call app's configured callback if provided
		if onConfigured != nil {
			onConfigured(w, h)
		}
	})
}

// renderFrame renders all drawables to a single frame's pixel buffer.
func (l *Layout) renderFrame(frame *buffer.Frame, pixels []byte, width, height int) {
	stride := width * 4

	// Get viewport position (output position)
	viewportX := int(frame.OutputX())
	viewportY := int(frame.OutputY())

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

	// Render all drawables
	l.drawablesMut.Lock()
	drawables := l.drawables
	l.drawablesMut.Unlock()

	for _, d := range drawables {
		// Get view bounds
		view := d.GetView()
		if view == nil {
			continue
		}

		viewLeft := int(view.Left.Get())
		viewTop := int(view.Top.Get())
		viewRight := int(view.Right.Get())
		viewBottom := int(view.Bottom.Get())

		viewW := viewRight - viewLeft
		viewH := viewBottom - viewTop

		if viewW <= 0 || viewH <= 0 {
			continue
		}

		// Calculate view position relative to this Frame's viewport
		relX := viewLeft - viewportX
		relY := viewTop - viewportY

		// Visibility check (culling)
		if relX+viewW <= 0 || relY+viewH <= 0 || relX >= width || relY >= height {
			continue // Off-screen, skip
		}

		// Calculate clip region in view's local space
		clipX := max(-relX, 0)                   // Clip left if off-left
		clipY := max(-relY, 0)                   // Clip top if off-top
		clipW := min(viewW, width-relX) - clipX  // Clip right
		clipH := min(viewH, height-relY) - clipY // Clip bottom

		if clipW <= 0 || clipH <= 0 {
			continue
		}

		// Draw view with pre-offset pixel slice
		// View draws in local space (0,0) in the framebuffer
		offsetY := max(relY, 0)
		offsetX := max(relX, 0)
		offset := offsetY*stride + offsetX*4
		fb := Framebuffer{
			pixels: pixels[offset:],
			stride: stride,
			width:  clipW,
			height: clipH,
		}
		d.Draw(fb, dpi)
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

// addDrawable adds a drawable to the layout for rendering.
func (l *Layout) addDrawable(d Drawable) {
	l.drawablesMut.Lock()
	l.drawables = append(l.drawables, d)
	l.drawablesMut.Unlock()
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

// NewView creates view via layout
func (l *Layout) NewView() *View {
	return &View{
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

// OutputDPI returns the DPI for a given output, or 96 if unavailable.
func OutputDPI(output *display.Output) float64 {
	if output == nil {
		return 96
	}
	dpi := output.DPI()
	if dpi == 0 {
		return 96
	}
	return dpi
}
