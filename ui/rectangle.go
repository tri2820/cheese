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
	rect.Content.disposers = append(rect.Content.disposers, func() {
		rect.color.Dispose()
	})

	// Request render when color changes
	rect.Content.disposers = append(rect.Content.disposers, signals.Effect(func() {
		w.layout.RequestRender()
	}, rect.color))

	// Add content to widget
	w.mu.Lock()
	w.contents = append(w.contents, &rect.Content)
	w.mu.Unlock()

	return rect
}

// draw renders the rectangle to the pixel buffer.
// Uses LayoutItem position to determine where to draw within the framebuffer.
func (r *Rectangle) draw(ctx DrawContext) {
	c := r.color.Get()
	fb := ctx.Framebuffer

	// Get position from LayoutItem (solved by cassowary) in widget coordinates
	// Apply DPI scale to convert to physical pixels
	left := int(r.LayoutItem.Left.Get() * ctx.Scale)
	top := int(r.LayoutItem.Top.Get() * ctx.Scale)
	right := int(r.LayoutItem.Right.Get() * ctx.Scale)
	bottom := int(r.LayoutItem.Bottom.Get() * ctx.Scale)

	width := right - left
	height := bottom - top

	if width <= 0 || height <= 0 {
		return
	}

	// Transform to framebuffer coordinates by subtracting offset
	// Offset is the visible region origin in physical pixels
	fbLeft := left - ctx.OffsetX
	fbTop := top - ctx.OffsetY
	fbRight := right - ctx.OffsetX
	fbBottom := bottom - ctx.OffsetY

	// Clamp to visible region (intersection with framebuffer)
	x0 := max(0, fbLeft)
	y0 := max(0, fbTop)
	x1 := min(fb.Width(), fbRight)
	y1 := min(fb.Height(), fbBottom)

	// Not visible
	if x1 <= x0 || y1 <= y0 {
		return
	}

	// Fill the rectangle region
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			fb.SetPixel(x, y, c.r, c.g, c.b, c.a)
		}
	}
}
