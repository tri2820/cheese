package ui

// Content provides common fields and methods for all content types.
// Embed this in your content structs to get positioning, cleanup, and default rendering.
type Content struct {
	*LayoutItem         // Positioning within content space
	widget      *Widget // Parent widget
	layout      *Layout // Reference to layout for render requests

	// DrawFunc is the actual drawing implementation
	DrawFunc func(fb Framebuffer, dpi float64)
}

// Draw calls the custom draw function if set.
func (b *Content) Draw(fb Framebuffer, dpi float64) {
	if b.DrawFunc != nil {
		b.DrawFunc(fb, dpi)
	}
}

// Cleanup removes the content's LayoutItem symbols from the solver.
func (b *Content) Cleanup(layout *Layout) {
	layout.removeVar(b.LayoutItem.Left.state.symbol)
	layout.removeVar(b.LayoutItem.Top.state.symbol)
	layout.removeVar(b.LayoutItem.Right.state.symbol)
	layout.removeVar(b.LayoutItem.Bottom.state.symbol)
}
