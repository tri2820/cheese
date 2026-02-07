package main

import (
	"log"

	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/ui"
)

func main() {
	// Connect to Wayland display
	disp, err := display.Connect(display.Config{
		Required: display.RequiredGlobals{
			Compositor: true,
			Shm:        true,
			LayerShell: true,
		},
	})
	if err != nil {
		log.Fatalf("Failed to connect to display: %v", err)
	}
	defer disp.Close()

	// Create layout
	layout := ui.NewLayout(disp)
	go layout.RenderLoop()

	// Get all outputs
	outputs := disp.ReadyOutputs()
	log.Printf("Found %d outputs", len(outputs))

	if len(outputs) < 2 {
		log.Fatalf("Portal demo requires at least 2 outputs, found %d", len(outputs))
	}

	// Create ONE widget with contents (coordinate-free, shared across outputs)
	widget := ui.NewWidget(layout)

	// Set content size at 96 DPI baseline
	widget.Width = 600
	widget.Height = 600

	// Create 9 colorful rectangles arranged in 3x3 grid
	colors := []string{
		"#FF0000", "#FF7F00", "#FFFF00", // Red, Orange, Yellow
		"#00FF00", "#0000FF", "#4B0082", // Green, Blue, Indigo
		"#9400D3", "#FF1493", "#00CED1", // Violet, Pink, Cyan
	}

	// Each cell is 1/3 of content size in pixels
	cellPixelW := widget.Width / 3.0
	cellPixelH := widget.Height / 3.0

	for i, color := range colors {
		row := i / 3
		col := i % 3

		// Create rectangle with constraint-based positioning
		rect := widget.NewRectangle()
		rect.Color.Set(color)

		// Position using constraints (in content coordinate space 0-600 x 0-200)
		ui.Eq(rect.Left, float64(col)*cellPixelW).Add()
		ui.Eq(rect.Top, float64(row)*cellPixelH).Add()
		ui.Eq(rect.Right, float64(col+1)*cellPixelW).Add()
		ui.Eq(rect.Bottom, float64(row+1)*cellPixelH).Add()
	}

	// Add a label for testing text rendering
	label := widget.NewLabel("Portal Demo Testing Clipping Masks")
	label.Color.Set("#FFFFFF")
	label.FontSize.Set(24.0)
	label.Justify.Set(ui.JustifyCenter)

	// Center label vertically with equal spacing
	ui.Eq(label.Left, 0).Add()
	ui.Eq(label.Right, widget.Width).Add()
	ui.Eq(label.Top, 0).Add()
	ui.Eq(label.Bottom, widget.Height).Add()

	log.Printf("Created widget with 9 colorful rectangles in 3x3 grid and label")

	// Helper function to create a mask for an output
	// Anchor is always AnchorTop|AnchorLeft - position controlled by margin via reactive Effect
	createMask := func(output *display.Output, name string, clipX, clipY float64, surfaceWidth, surfaceHeight int) *ui.Mask {
		log.Printf("%s: %s at (%d, %d) size %dx%d DPI=%.1f",
			name, output.Name, output.X, output.Y, output.ModeWidth, output.ModeHeight, output.DPIOrDefault())
		log.Printf("  Clip origin: (%.1f, %.1f) in widget coordinates", clipX, clipY)
		log.Printf("  Surface (mask) size: %dx%d (visible region)", surfaceWidth, surfaceHeight)

		mask := widget.NewMask(output.WlOutput(), ui.LayerConfig{
			Layer: shell.LayerPositionTop,
			Name:  name,
		})

		// Set clip origin in widget coordinates (96 DPI units)
		mask.ClipX = clipX
		mask.ClipY = clipY

		return mask
	}

	// Output 0: shows left 30% of content (0 to 180) at right edge
	// Widget is 600 wide, so left 30% = 180 widget units
	clipLeft0 := 0.0
	clipTop0 := 0.0
	// Surface size in physical pixels: 30% of widget width scaled by DPI
	surfaceW0 := int(0.3 * float64(outputs[0].ScaleFrom96DPI(widget.Width)))
	surfaceH0 := int(1.0 * float64(outputs[0].ScaleFrom96DPI(widget.Height)))
	mask0 := createMask(outputs[0], "portal-output-0", clipLeft0, clipTop0, surfaceW0, surfaceH0)

	// Position at right edge, vertically centered
	ui.Eq(mask0.Right, float64(outputs[0].ModeWidth)).Add()
	ui.Eq(mask0.Width(), float64(surfaceW0)).Add()
	ui.Eq(mask0.CenterY(), float64(outputs[0].ModeHeight)/2).Add()
	ui.Eq(mask0.Height(), float64(surfaceH0)).Add()

	// Output 1: shows right 70% of content (180 to 600) at left edge
	clipLeft1 := 0.3 * widget.Width  // Start at 30% from left
	clipTop1 := 0.0
	surfaceW1 := int(0.7 * float64(outputs[1].ScaleFrom96DPI(widget.Width)))
	surfaceH1 := int(1.0 * float64(outputs[1].ScaleFrom96DPI(widget.Height)))
	mask1 := createMask(outputs[1], "portal-output-1", clipLeft1, clipTop1, surfaceW1, surfaceH1)

	// Position at left edge, vertically centered
	ui.Eq(mask1.Left, 0).Add()
	ui.Eq(mask1.Width(), float64(surfaceW1)).Add()
	ui.Eq(mask1.CenterY(), float64(outputs[1].ModeHeight)/2).Add()
	ui.Eq(mask1.Height(), float64(surfaceH1)).Add()
	log.Println()
	log.Println("Portal running... Press Ctrl+C to exit")

	// Run event loopz
	if err := disp.Run(); err != nil {
		log.Fatalf("Dispatch error: %v", err)
	}
}
