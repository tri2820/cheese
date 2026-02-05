package ui

import (
	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui/common"
)

// Rectangle is a widget that extends Element with background color.
type Rectangle struct {
	*Element                       // Embedding: Rectangle IS-A Element
	Color   signals.Signal[string] // Background color (e.g., "#FF0000")
	layout  *Layout                // Reference to layout for command registration
}

// NewRectangle creates a new Rectangle widget.
func (l *Layout) NewRectangle() *Rectangle {
	r := &Rectangle{
		Element: l.NewElement(),
		Color:   signals.New("#FFFFFF"), // Default white
		layout:  l,
	}

	// Create effect that runs when bounds or color change
	signals.Effect(func() {
		x := int(r.Left.Get())
		y := int(r.Top.Get())
		w := int(r.Right.Get() - r.Left.Get())
		h := int(r.Bottom.Get() - r.Top.Get())

		// Skip drawing if bounds are invalid (happens during initial setup)
		if w <= 0 || h <= 0 {
			return
		}

		c := common.ParseColor(r.Color.Get())

		if l.cmdList != nil {
			l.cmdList.Add(DrawRect{X: x, Y: y, W: w, H: h, Color: c})
		}

		l.RequestRender()
	}, r.Left, r.Top, r.Right, r.Bottom, r.Color)

	return r
}
