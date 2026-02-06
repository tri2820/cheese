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
// The pixels slice is already offset to the widget's origin, so the widget
// can draw from (0,0) in its local coordinate space.
func (r *Rectangle) Draw(pixels []byte, stride int, clip Rect, dpi float64) {
	cmd := r.cmd.Get()

	// Only draw within the clip region (in widget's local space)
	for localY := clip.Y; localY < clip.Y+clip.H; localY++ {
		pixelRow := pixels[localY*stride:]

		for localX := clip.X; localX < clip.X+clip.W; localX++ {
			// ARGB8888 format: B,G,R,A on little-endian
			bgra := []byte{cmd.color.b, cmd.color.g, cmd.color.r, cmd.color.a}
			copy(pixelRow[localX*4:], bgra)
		}
	}
}
