package ui

import (
	"sync"

	"github.com/lithdew/casso"
)

// Widget manages a group of drawables and frames with lifecycle management.
type Widget struct {
	mu        sync.RWMutex
	layout    *Layout
	drawables []Drawable
	symbols   []casso.Symbol // Track all symbols for cleanup
}

// NewWidget creates a new widget attached to the given layout.
func NewWidget(layout *Layout) *Widget {
	w := &Widget{
		layout:    layout,
		drawables: make([]Drawable, 0),
		symbols:   make([]casso.Symbol, 0),
	}
	layout.addWidget(w)
	return w
}

// AddDrawable adds a drawable to this widget.
func (w *Widget) AddDrawable(d Drawable) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.drawables = append(w.drawables, d)
}

// Remove removes this widget and all its resources.
func (w *Widget) Remove() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Remove all casso variables
	for _, sym := range w.symbols {
		w.layout.removeVar(sym)
	}

	// Remove this widget from layout's widget list
	w.layout.removeWidget(w)

	// Clear slices to allow GC
	w.drawables = nil
	w.symbols = nil

	// Request re-render
	w.layout.RequestRender()

	return nil
}
