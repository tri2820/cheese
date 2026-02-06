package ui

import (
	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui/common"
)

// drawCommand contains all pre-computed data needed to draw the rectangle.
type drawCommand struct {
	w, h  int
	color struct{ r, g, b, a uint8 }
}

// Rectangle is a layoutitem that extends LayoutItem with background color.
type Rectangle struct {
	*LayoutItem                        // Embedding: Rectangle IS-A LayoutItem
	Color       signals.Signal[string] // Background color (e.g., "#FF0000")
	layout      *Layout                // Reference to layout for render requests
	cmd         signals.Signal[drawCommand]
}

// NewRectangle creates a new Rectangle drawable in this widget.
func (w *Widget) NewRectangle() *Rectangle {
	// Create rectangle drawable
	rect := &Rectangle{
		LayoutItem: w.layout.NewLayoutItem(),
		Color:      signals.New("#FFFFFF"), // Default white
		layout:     w.layout,
	}

	// Derive draw command from bounds and color
	rect.cmd = signals.Derive(func() drawCommand {
		width := int(rect.Right.Get() - rect.Left.Get())
		height := int(rect.Bottom.Get() - rect.Top.Get())

		c := common.ParseColor(rect.Color.Get())

		return drawCommand{
			w: width, h: height,
			color: struct{ r, g, b, a uint8 }{r: c.R, g: c.G, b: c.B, a: c.A},
		}
	}, rect.Left, rect.Top, rect.Right, rect.Bottom, rect.Color)

	// Request render when draw command changes
	signals.Effect(func() {
		w.layout.RequestRender()
	}, rect.cmd)

	// Track symbols from the LayoutItem
	w.symbols = append(w.symbols,
		rect.Left.state.symbol,
		rect.Right.state.symbol,
		rect.Top.state.symbol,
		rect.Bottom.state.symbol,
	)

	// Add drawable to widget
	w.AddDrawable(rect)

	return rect
}

// Draw renders the rectangle to the pixel buffer.
// The framebuffer origin (0,0) is at the layoutitem's visible region.
func (r *Rectangle) Draw(fb Framebuffer, dpi float64) {
	cmd := r.cmd.Get()

	// Draw to the entire framebuffer (already clipped by layout)
	for y := 0; y < fb.Height(); y++ {
		for x := 0; x < fb.Width(); x++ {
			fb.SetPixel(x, y, cmd.color.r, cmd.color.g, cmd.color.b, cmd.color.a)
		}
	}
}

// GetLayoutItem returns the embedded LayoutItem (for Drawable interface).
func (r *Rectangle) GetLayoutItem() *LayoutItem {
	return r.LayoutItem
}
