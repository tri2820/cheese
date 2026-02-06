package ui

import (
	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui/common"
)

// Rectangle is a coordinate-free content with background color.
type Rectangle struct {
	widget *Widget                    // Parent widget
	Color  signals.Signal[string]     // Background color (e.g., "#FF0000")
	layout *Layout                    // Reference to layout for render requests
	color  signals.Signal[struct{ r, g, b, a uint8 }] // Parsed color
}

// NewRectangle creates a new Rectangle content (coordinate-free).
func (w *Widget) NewRectangle() *Rectangle {
	// Create rectangle content
	rect := &Rectangle{
		widget: w,
		Color:  signals.New("#FFFFFF"), // Default white
		layout: w.layout,
	}

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
	w.contents = append(w.contents, rect)
	w.mu.Unlock()

	return rect
}

// Draw renders the rectangle to the pixel buffer.
// Coordinate-free: draws to the entire framebuffer (already clipped by layout).
func (r *Rectangle) Draw(fb Framebuffer, dpi float64) {
	c := r.color.Get()

	// Fill the entire framebuffer with the color
	for y := 0; y < fb.Height(); y++ {
		for x := 0; x < fb.Width(); x++ {
			fb.SetPixel(x, y, c.r, c.g, c.b, c.a)
		}
	}
}

// PositionedRectangle is a rectangle with relative position (0.0-1.0) within content.
type PositionedRectangle struct {
	widget *Widget
	Color  signals.Signal[string]
	layout *Layout
	color  signals.Signal[struct{ r, g, b, a uint8 }]

	// Position as fraction of content size (0.0 to 1.0)
	relX, relY float64
	relW, relH float64
}

// NewPositionedRectangle creates a positioned rectangle within content space.
// x, y, w, h are relative to content size (0.0 to 1.0).
func (w *Widget) NewPositionedRectangle(x, y, width, height float64) *PositionedRectangle {
	rect := &PositionedRectangle{
		widget: w,
		Color:  signals.New("#FFFFFF"),
		layout: w.layout,
		relX:   x,
		relY:   y,
		relW:   width,
		relH:   height,
	}

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
	w.contents = append(w.contents, rect)
	w.mu.Unlock()

	return rect
}

// Draw renders the positioned rectangle.
// Converts relative position to absolute pixels based on framebuffer size.
func (r *PositionedRectangle) Draw(fb Framebuffer, dpi float64) {
	c := r.color.Get()

	// Convert relative position to absolute pixels
	fbW := fb.Width()
	fbH := fb.Height()

	x0 := int(r.relX * float64(fbW))
	y0 := int(r.relY * float64(fbH))
	x1 := int((r.relX + r.relW) * float64(fbW))
	y1 := int((r.relY + r.relH) * float64(fbH))

	// Clamp to framebuffer bounds
	x0 = max(0, min(x0, fbW))
	y0 = max(0, min(y0, fbH))
	x1 = max(0, min(x1, fbW))
	y1 = max(0, min(y1, fbH))

	// Fill the rectangle region
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			fb.SetPixel(x, y, c.r, c.g, c.b, c.a)
		}
	}
}
