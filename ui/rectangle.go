package ui

import (
	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui/common"
)

// Rectangle is a content with background color.
// Embeds BaseContent for positioning within content space.
type Rectangle struct {
	Content                                            // Positioning and common fields
	Color   signals.Signal[string]                     // Background color (e.g., "#FF0000")
	color   signals.Signal[struct{ r, g, b, a uint8 }] // Parsed color
}

// NewRectangle creates a new Rectangle content with constraint-based positioning.
func (w *Widget) NewRectangle() *Rectangle {
	// Create rectangle content
	rect := &Rectangle{
		Content: Content{
			LayoutItem: w.layout.NewLayoutItem(), // Create LayoutItem for positioning
			widget:     w,
			layout:     w.layout,
		},
		Color: signals.New("#FFFFFF"), // Default white
	}

	// Set draw function
	rect.Content.DrawFunc = rect.draw

	// Derive parsed color from color string
	rect.color = signals.Derive(func() struct{ r, g, b, a uint8 } {
		c := common.ParseColor(rect.Color.Get())
		return struct{ r, g, b, a uint8 }{r: c.R, g: c.G, b: c.B, a: c.A}
	}, rect.Color)

	// Request render when color changes
	signals.Effect(func() {
		w.layout.RequestRender()
	}, rect.color)

	// Add content to widget
	w.mu.Lock()
	w.contents = append(w.contents, &rect.Content)
	w.mu.Unlock()

	return rect
}

// draw renders the rectangle to the pixel buffer.
// Uses LayoutItem position to determine where to draw within the framebuffer.
func (r *Rectangle) draw(fb Framebuffer, dpi float64) {
	c := r.color.Get()

	// Get position from LayoutItem (solved by cassowary)
	left := int(r.LayoutItem.Left.Get())
	top := int(r.LayoutItem.Top.Get())
	right := int(r.LayoutItem.Right.Get())
	bottom := int(r.LayoutItem.Bottom.Get())

	width := right - left
	height := bottom - top

	if width <= 0 || height <= 0 {
		return
	}

	// Clamp to framebuffer bounds
	x0 := max(0, min(left, fb.Width()))
	y0 := max(0, min(top, fb.Height()))
	x1 := max(0, min(right, fb.Width()))
	y1 := max(0, min(bottom, fb.Height()))

	// Fill the rectangle region
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			fb.SetPixel(x, y, c.r, c.g, c.b, c.a)
		}
	}
}
