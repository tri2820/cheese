package ui

import (
	"image"

	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui/common"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// labelDrawCommand contains all pre-computed data needed to draw the label.
type labelDrawCommand struct {
	text       string
	color      struct{ r, g, b, a uint8 }
	fontFamily string
	fontSize   float64
}

// Label is a content that renders text.
// Embeds BaseContent for positioning within content space.
type Label struct {
	Content                            // Positioning and common fields
	Text       signals.Signal[string]  // Text to display
	Color      signals.Signal[string]  // Text color
	FontSize   signals.Signal[float64] // Font size in points
	FontFamily signals.Signal[string]  // Font family name
	cmd        signals.Signal[labelDrawCommand]
}

// NewLabel creates a new Label content with constraint-based positioning.
func (w *Widget) NewLabel(text string) *Label {
	// Create label content
	label := &Label{
		Content: Content{
			LayoutItem: w.layout.NewLayoutItem(), // Create LayoutItem for positioning
			widget:     w,
			layout:     w.layout,
		},
		Text:       signals.New(text),
		Color:      signals.New("#FFFFFF"), // Default white
		FontSize:   signals.New(12.0),      // Default 12pt
		FontFamily: signals.New("Liberation Sans"),
	}

	// Set draw function
	label.Content.DrawFunc = label.draw

	// Derive draw command from text properties
	label.cmd = signals.Derive(func() labelDrawCommand {
		c := common.ParseColor(label.Color.Get())

		return labelDrawCommand{
			text:       label.Text.Get(),
			color:      struct{ r, g, b, a uint8 }{r: c.R, g: c.G, b: c.B, a: c.A},
			fontFamily: label.FontFamily.Get(),
			fontSize:   label.FontSize.Get(),
		}
	}, label.Text, label.Color, label.FontSize, label.FontFamily)

	// Request render when draw command changes
	signals.Effect(func() {
		w.layout.RequestRender()
	}, label.cmd)

	// Add content to widget
	w.mu.Lock()
	w.contents = append(w.contents, &label.Content)
	w.mu.Unlock()

	return label
}

// draw renders the label to the pixel buffer.
// Uses LayoutItem position to determine where to draw within the framebuffer.
func (l *Label) draw(fb Framebuffer, dpi float64) {
	cmd := l.cmd.Get()

	// Get position from LayoutItem (solved by cassowary)
	left := int(l.LayoutItem.Left.Get())
	top := int(l.LayoutItem.Top.Get())
	right := int(l.LayoutItem.Right.Get())
	bottom := int(l.LayoutItem.Bottom.Get())

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

	if x0 >= x1 || y0 >= y1 {
		return
	}

	// Load font face
	face, err := common.GetFont(cmd.fontFamily, cmd.fontSize, dpi)
	if err != nil {
		// Silently fail - don't crash on missing fonts
		return
	}

	// Get image view of framebuffer
	img := fb.AsImage()

	// Calculate text position (vertically centered within bounds)
	m := face.Metrics()
	y := top + (height+m.Ascent.Ceil()-m.Descent.Ceil())/2

	// Draw text at positioned location
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(common.ParseColor(l.Color.Get())),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(left), Y: fixed.I(y)},
	}
	drawer.DrawString(cmd.text)
}

// Cleanup is inherited from BaseContent
