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

	display *display.Display // Display connection for mask frame creation

	frames       []*buffer.Frame         // Frames to notify when render is needed
	maskForFrame map[*buffer.Frame]*Mask // Maps frame to its mask
	framesMut    sync.Mutex              // Protects frames slice and maskForFrame map
	dirty        bool                    // True when rendering is needed
	dirtyMut     sync.Mutex              // Protects dirty flag

	widgets    []*Widget  // Widgets to manage
	widgetsMut sync.Mutex // Protects widgets slice
}

// NewLayout creates a new constraint solver
func NewLayout(disp *display.Display) *Layout {
	return &Layout{
		inner:        casso.NewSolver(),
		vars:         make(map[casso.Symbol]*exprState),
		renderReq:    make(chan struct{}, 1),
		stopRender:   make(chan struct{}),
		display:      disp,
		maskForFrame: make(map[*buffer.Frame]*Mask),
	}
}

// AddFrame adds a frame to the layout.
// Sets up the frame's OnConfigured and OnRender handlers automatically.
// The onConfigured callback is invoked after the frame is configured, with the frame dimensions.
func (l *Layout) AddFrame(frame *buffer.Frame, mask *Mask, onConfigured func(w, h int)) {
	// Store frame for render notifications and map to mask
	l.framesMut.Lock()
	l.frames = append(l.frames, frame)
	l.maskForFrame[frame] = mask
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

// renderFrame renders all contents from the widget to a single frame's pixel buffer.
func (l *Layout) renderFrame(frame *buffer.Frame, pixels []byte, width, height int) {
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
	contentWidth := widget.Width
	contentHeight := widget.Height
	widget.mu.RUnlock()

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
		// Compute physical content size at this DPI
		contentW := int(contentWidth * dpi / 96.0)
		contentH := int(contentHeight * dpi / 96.0)

		// Create temp buffer for full content
		tempPixels := make([]byte, contentW*contentH*4)
		tempStride := contentW * 4
		tempFb := Framebuffer{
			pixels: tempPixels,
			stride: tempStride,
			width:  contentW,
			height: contentH,
		}

		// Render all contents to temp buffer
		for _, content := range contents {
			content.Draw(tempFb, dpi)
		}

		// Get clipping config from mask (which portion of content to show)
		clipX := 0.0
		clipY := 0.0
		clipW := 1.0
		clipH := 1.0
		if mask.ClipRelW > 0 {
			clipX = mask.ClipRelX
			clipW = mask.ClipRelW
		}
		if mask.ClipRelH > 0 {
			clipY = mask.ClipRelY
			clipH = mask.ClipRelH
		}

		// Calculate source start position in content buffer
		srcStartX := int(clipX * float64(contentW))
		srcStartY := int(clipY * float64(contentH))
		srcW := int(clipW * float64(contentW))
		srcH := int(clipH * float64(contentH))

		// Copy visible region from temp buffer to frame
		// Copy from clipped region in content buffer
		copyW := min(width, srcW, contentW-srcStartX)
		copyH := min(height, srcH, contentH-srcStartY)
		for y := 0; y < copyH; y++ {
			for x := 0; x < copyW; x++ {
				srcOffset := (srcStartY+y)*tempStride + (srcStartX+x)*4
				dstOffset := y*stride + x*4
				// Copy BGRA pixel
				fb.pixels[dstOffset] = tempPixels[srcOffset]
				fb.pixels[dstOffset+1] = tempPixels[srcOffset+1]
				fb.pixels[dstOffset+2] = tempPixels[srcOffset+2]
				fb.pixels[dstOffset+3] = tempPixels[srcOffset+3]
			}
		}
	} else {
		// No DPI normalization - render directly to framebuffer
		for _, content := range contents {
			content.Draw(fb, dpi)
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
	// Sync values from solver after adding constraints
	l.syncFromSolver()
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
	state.onChange = append(state.onChange, func() {
		l.resolve(symCopy)
	})

	return Expr{kind: exprVar, state: state, layout: l}
}

// syncFromSolver reads all values from cassowary and updates expr states.
// Called after adding constraints to immediately sync values.
func (l *Layout) syncFromSolver() {
	l.resolveMut.Lock()
	l.resolving = true
	l.resolveMut.Unlock()

	// Get resolved values from casso and update expressions
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
