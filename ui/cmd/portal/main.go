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

	// Create 9 colorful rectangles arranged in 3x3 grid
	colors := []string{
		"#FF0000", "#FF7F00", "#FFFF00", // Red, Orange, Yellow
		"#00FF00", "#0000FF", "#4B0082", // Green, Blue, Indigo
		"#9400D3", "#FF1493", "#00CED1", // Violet, Pink, Cyan
	}

	// Each cell is 1/3 of content width and height
	cellW := 1.0 / 3.0
	cellH := 1.0 / 3.0

	for i, color := range colors {
		row := i / 3
		col := i % 3

		// Create positioned rectangle (relative to content size 0.0-1.0)
		rect := widget.NewPositionedRectangle(
			float64(col)*cellW, // x (0.0, 0.33, 0.66)
			float64(row)*cellH, // y (0.0, 0.33, 0.66)
			cellW,              // width (0.33)
			cellH,              // height (0.33)
		)
		rect.Color.Set(color)
	}

	// Add a label for testing text rendering
	label := widget.NewLabel("Portal Demo Testing Clipping Masks")
	label.Color.Set("#FFFFFF")
	label.FontSize.Set(24.0)

	log.Printf("Created widget with 9 colorful rectangles in 3x3 grid and label")

	// Set content size at 96 DPI baseline
	widget.ContentWidth_at96DPI = 600
	widget.ContentHeight_at96DPI = 200

	// Helper function to create a mask for an output
	createMask := func(output *display.Output, name string, relLeft, relRight, relTop, relBottom float64) {
		dpi := output.DPI()
		if dpi == 0 {
			dpi = 96
		}

		// Calculate physical content size at this output's DPI
		contentWidthAtDPI := int(widget.ContentWidth_at96DPI * dpi / 96.0)
		contentHeightAtDPI := int(widget.ContentHeight_at96DPI * dpi / 96.0)

		log.Printf("%s: %s at (%d, %d) size %dx%d DPI=%.1f",
			name, output.Name, output.X, output.Y, output.ModeWidth, output.ModeHeight, output.DPI())
		log.Printf("  Content size at 96 DPI: %dx%d", int(widget.ContentWidth_at96DPI), int(widget.ContentHeight_at96DPI))
		log.Printf("  Content size at %.1f DPI: %dx%d (physical pixels)", dpi, contentWidthAtDPI, contentHeightAtDPI)

		// Surface size (mask size) = visible portion of content
		surfaceWidth := int((relRight - relLeft) * float64(contentWidthAtDPI))
		surfaceHeight := int((relBottom - relTop) * float64(contentHeightAtDPI))

		mask := widget.NewMask(output.WlOutput(), ui.LayerConfig{
			Layer:         shell.LayerPositionTop,
			Name:          name,
			Anchor:        shell.AnchorTop | shell.AnchorLeft,
			Width:         uint32(surfaceWidth),
			Height:        uint32(surfaceHeight),
			ExclusiveZone: 0,
		})

		// Set clipping config on mask
		mask.ClipRelX = relLeft
		mask.ClipRelY = relTop
		mask.ClipRelW = relRight - relLeft
		mask.ClipRelH = relBottom - relTop

		// Position mask in frame-local coordinates
		ui.Eq(mask.Left, 0).Add()
		ui.Eq(mask.Top, 0).Add()
		ui.Eq(mask.Right, surfaceWidth).Add()
		ui.Eq(mask.Bottom, surfaceHeight).Add()

		log.Printf("  Surface (mask) size: %dx%d (visible region)", surfaceWidth, surfaceHeight)
	}

	// Create mask on output 0 (full content)
	createMask(outputs[0], "portal-output-0", 0.0, 0.3, 0.0, 1.0)

	// Create mask on output 1 (full content)
	createMask(outputs[1], "portal-output-1", 0.3, 1.0, 0.0, 1.0)
	log.Println()
	log.Println("Portal running... Press Ctrl+C to exit")

	// Run event loop
	if err := disp.Run(); err != nil {
		log.Fatalf("Dispatch error: %v", err)
	}
}
