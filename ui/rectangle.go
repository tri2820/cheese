package ui

import (
	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui/common"
)

// drawCommand contains all pre-computed data needed to draw the rectangle.
type drawCommand struct {
	x, y, w, h int
	color      struct{ r, g, b, a uint8 }
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
		x := int(r.Left.Get())
		y := int(r.Top.Get())
		w := int(r.Right.Get() - r.Left.Get())
		h := int(r.Bottom.Get() - r.Top.Get())

		c := common.ParseColor(r.Color.Get())

		return drawCommand{
			x: x, y: y, w: w, h: h,
			color: struct{ r, g, b, a uint8 }{r: c.R, g: c.G, b: c.B, a: c.A},
		}
	}, r.Left, r.Top, r.Right, r.Bottom, r.Color)

	// Request render when draw command changes
	signals.Effect(func() {
		l.RequestRender()
	}, r.cmd)

	return r
}

// Draw renders the rectangle to the pixel buffer.
func (r *Rectangle) Draw(pixels []byte, stride, width, height int) {
	cmd := r.cmd.Get()
	x, y, w, h := cmd.x, cmd.y, cmd.w, cmd.h

	// Skip drawing if bounds are invalid
	if w <= 0 || h <= 0 {
		return
	}

	// Clamp bounds to surface
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > width {
		w = width - x
	}
	if y+h > height {
		h = height - y
	}
	if w <= 0 || h <= 0 {
		return
	}

	// ARGB8888 format: B,G,R,A on little-endian
	bgra := []byte{cmd.color.b, cmd.color.g, cmd.color.r, cmd.color.a}

	for row := y; row < y+h; row++ {
		pixelRow := pixels[row*stride:]
		for col := x; col < x+w; col++ {
			copy(pixelRow[col*4:], bgra)
		}
	}
}
