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

// Rectangle is a widget that extends Element with background color.
type Rectangle struct {
	*Element                        // Embedding: Rectangle IS-A Element
	Color    signals.Signal[string] // Background color (e.g., "#FF0000")
	layout   *Layout                // Reference to layout for render requests
	cmd      signals.Signal[drawCommand]
}

// NewRectangle creates a new Rectangle widget.
func (l *Layout) NewRectangle() *Rectangle {
	r := &Rectangle{
		Element: l.NewElement(),
		Color:   signals.New("#FFFFFF"), // Default white
		layout:  l,
	}

	// Track widget in layout for rendering
	l.addWidget(r)

	// Derive draw command from bounds and color
	r.cmd = signals.Derive(func() drawCommand {
		w := int(r.Right.Get() - r.Left.Get())
		h := int(r.Bottom.Get() - r.Top.Get())

		c := common.ParseColor(r.Color.Get())

		return drawCommand{
			w: w, h: h,
			color: struct{ r, g, b, a uint8 }{r: c.R, g: c.G, b: c.B, a: c.A},
		}
	}, r.Left, r.Top, r.Right, r.Bottom, r.Color)

	// Request render when draw command changes
	signals.Effect(func() {
		l.RequestRender()
	}, r.cmd)

	return r
}

// GetElement returns the embedded Element (for Widget interface reflection).
func (r *Rectangle) GetElement() *Element {
	return r.Element
}

// Draw renders the rectangle to the pixel buffer.
// The framebuffer origin (0,0) is at the widget's visible region.
func (r *Rectangle) Draw(fb Framebuffer, dpi float64) {
	cmd := r.cmd.Get()

	// Draw to the entire framebuffer (already clipped by layout)
	for y := 0; y < fb.Height(); y++ {
		for x := 0; x < fb.Width(); x++ {
			fb.SetPixel(x, y, cmd.color.r, cmd.color.g, cmd.color.b, cmd.color.a)
		}
	}
}
