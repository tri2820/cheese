package ui

import "github.com/tri2820/cheese/signals"

// DrawContext provides affine coordinate transformation from widget space to framebuffer.
// Transformation: fb = (widget * Scale) - Offset
// Widget coordinates are in "96 DPI units" - Scale transforms to physical pixels.
// Offset defines the visible region origin in physical pixels.
type DrawContext struct {
	Framebuffer Framebuffer
	OffsetX     int // Visible region start in physical pixels
	OffsetY     int
	Scale       float64 // DPI multiplier: dpi/96.0
}

// Content provides common fields and methods for all content types.
// Embed this in your content structs to get positioning, cleanup, and default rendering.
type Content struct {
	*LayoutItem         // Positioning within content space
	widget      *Widget // Parent widget
	layout      *Layout // Reference to layout for render requests
	disposers   []signals.CancelFunc

	// DrawFunc is the actual drawing implementation
	DrawFunc func(ctx DrawContext)
}

// Draw calls the custom draw function if set.
func (b *Content) Draw(ctx DrawContext) {
	if b.DrawFunc != nil {
		b.DrawFunc(ctx)
	}
}

// Cleanup removes the content's LayoutItem symbols from the solver.
func (b *Content) Cleanup(layout *Layout) {
	for _, dispose := range b.disposers {
		if dispose != nil {
			dispose()
		}
	}
	b.disposers = nil

	layout.removeVar(b.LayoutItem.Left.state.symbol)
	layout.removeVar(b.LayoutItem.Top.state.symbol)
	layout.removeVar(b.LayoutItem.Right.state.symbol)
	layout.removeVar(b.LayoutItem.Bottom.state.symbol)
}
