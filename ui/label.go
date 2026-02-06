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

// Label is a coordinate-free content that renders text.
type Label struct {
	widget     *Widget
	Text       signals.Signal[string]
	Color      signals.Signal[string]
	FontSize   signals.Signal[float64]
	FontFamily signals.Signal[string]
	layout     *Layout
	cmd        signals.Signal[labelDrawCommand]
}

// NewLabel creates a new Label content (coordinate-free).
func (w *Widget) NewLabel(text string) *Label {
	// Create label content
	label := &Label{
		widget:     w,
		Text:       signals.New(text),
		Color:      signals.New("#FFFFFF"), // Default white
		FontSize:   signals.New(12.0),      // Default 12pt
		FontFamily: signals.New("Liberation Sans"),
		layout:     w.layout,
	}

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
	w.contents = append(w.contents, label)
	w.mu.Unlock()

	return label
}

// Draw renders the label to the pixel buffer.
// Coordinate-free: draws to the framebuffer (already clipped by layout).
func (l *Label) Draw(fb Framebuffer, dpi float64) {
	cmd := l.cmd.Get()

	// Load font face
	face, err := common.GetFont(cmd.fontFamily, cmd.fontSize, dpi)
	if err != nil {
		// Silently fail - don't crash on missing fonts
		return
	}

	// Get image view of framebuffer
	img := fb.AsImage()

	// Calculate text position (vertically centered)
	m := face.Metrics()
	y := (fb.Height() + m.Ascent.Ceil() - m.Descent.Ceil()) / 2

	// Draw text
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(common.ParseColor(l.Color.Get())),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(0), Y: fixed.I(y)},
	}
	drawer.DrawString(cmd.text)
}
