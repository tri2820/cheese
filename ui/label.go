package ui

import (
	"image"

	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui/common"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Justify defines text alignment options.
type Justify string

const (
	JustifyLeft   Justify = "left"
	JustifyCenter Justify = "center"
	JustifyRight  Justify = "right"
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
	Justify    signals.Signal[Justify] // Text justification
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
		Justify:    signals.New(JustifyLeft),
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
	label.Content.disposers = append(label.Content.disposers, func() {
		label.cmd.Dispose()
	})

	// Request render when draw command changes
	label.Content.disposers = append(label.Content.disposers, signals.Effect(func() {
		w.layout.RequestRender()
	}, label.cmd))

	// Add content to widget
	w.mu.Lock()
	w.contents = append(w.contents, &label.Content)
	w.mu.Unlock()

	return label
}

// draw renders the label to the pixel buffer.
// Uses LayoutItem position to determine where to draw within the framebuffer.
func (l *Label) draw(ctx DrawContext) {
	cmd := l.cmd.Get()
	fb := ctx.Framebuffer

	// Get position from LayoutItem (solved by cassowary) in widget coordinates
	// Apply DPI scale to convert to physical pixels
	left := int(l.LayoutItem.Left.Get() * ctx.Scale)
	top := int(l.LayoutItem.Top.Get() * ctx.Scale)
	right := int(l.LayoutItem.Right.Get() * ctx.Scale)
	bottom := int(l.LayoutItem.Bottom.Get() * ctx.Scale)

	width := right - left
	height := bottom - top

	if width <= 0 || height <= 0 {
		return
	}

	// Transform to framebuffer coordinates by subtracting offset
	fbLeft := left - ctx.OffsetX
	fbTop := top - ctx.OffsetY
	fbRight := right - ctx.OffsetX
	fbBottom := bottom - ctx.OffsetY

	// Clamp to visible region (intersection with framebuffer)
	x0 := max(0, fbLeft)
	y0 := max(0, fbTop)
	x1 := min(fb.Width(), fbRight)
	y1 := min(fb.Height(), fbBottom)

	if x0 >= x1 || y0 >= y1 {
		return
	}

	// Calculate DPI for font loading from scale
	dpi := 96.0 * ctx.Scale

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
	y := fbTop + (height+m.Ascent.Ceil()-m.Descent.Ceil())/2

	// Calculate x position based on justification
	x := fbLeft
	justify := l.Justify.Get()
	switch justify {
	case JustifyCenter:
		// Measure text width to center it
		textWidth := font.MeasureString(face, cmd.text).Ceil()
		x = fbLeft + (width-textWidth)/2
	case JustifyRight:
		// Measure text width to align to right
		textWidth := font.MeasureString(face, cmd.text).Ceil()
		x = fbRight - textWidth
	case JustifyLeft:
		// Default left alignment
	}

	// Draw text at positioned location
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(common.ParseColor(l.Color.Get())),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)},
	}
	drawer.DrawString(cmd.text)
}

// Cleanup is inherited from BaseContent
