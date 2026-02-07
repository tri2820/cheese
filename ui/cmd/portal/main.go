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
	cellPixelW := widget.Width / 3.0  // 200px per cell
	cellPixelH := widget.Height / 3.0 // 66.67px per cell

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

	// Center label vertically with equal spacing (10px padding top and bottom)
	ui.Eq(label.Left, 0).Add()
	ui.Eq(label.Right, widget.Width).Add()
	ui.Eq(label.Top, 10).Add()                  // 10px from top
	ui.Eq(label.Bottom, widget.Height-10).Add() // 10px from bottom

	log.Printf("Created widget with 9 colorful rectangles in 3x3 grid and label")

	// Helper function to create a mask for an output
	// Anchor is always AnchorTop|AnchorLeft - position controlled by margin via reactive Effect
	createMask := func(output *display.Output, name string, relLeft, relRight, relTop, relBottom float64) (*ui.Mask, int, int) {
		// Calculate physical content size at this output's DPI
		contentWidthAtDPI := output.ScaleFrom96DPI(widget.Width)
		contentHeightAtDPI := output.ScaleFrom96DPI(widget.Height)

		log.Printf("%s: %s at (%d, %d) size %dx%d DPI=%.1f",
			name, output.Name, output.X, output.Y, output.ModeWidth, output.ModeHeight, output.DPIOrDefault())
		log.Printf("  Content size at 96 DPI: %dx%d", int(widget.Width), int(widget.Height))
		log.Printf("  Content size at %.1f DPI: %dx%d (physical pixels)", output.DPIOrDefault(), contentWidthAtDPI, contentHeightAtDPI)

		// Surface size (mask size) = visible portion of content
		surfaceWidth := int((relRight - relLeft) * float64(contentWidthAtDPI))
		surfaceHeight := int((relBottom - relTop) * float64(contentHeightAtDPI))

		mask := widget.NewMask(output.WlOutput(), ui.LayerConfig{
			Layer:         shell.LayerPositionTop,
			Name:          name,
			// Anchor removed - always AnchorTop|AnchorLeft (set in mask.go)
			Width:         uint32(surfaceWidth),  // Initial size - will be updated by constraints
			Height:        uint32(surfaceHeight), // Initial size - will be updated by constraints
			ExclusiveZone: 0,
		})

		// Set clipping config on mask (which portion of content to show)
		mask.ClipRelX = relLeft
		mask.ClipRelY = relTop
		mask.ClipRelW = relRight - relLeft
		mask.ClipRelH = relBottom - relTop

		log.Printf("  Surface (mask) size: %dx%d (visible region)", surfaceWidth, surfaceHeight)
		return mask, surfaceWidth, surfaceHeight
	}

	// Create mask on output 0 - shows left 30% of content at right edge
	mask0, surfaceW0, surfaceH0 := createMask(outputs[0], "portal-output-0",
		0.0, 0.3, 0.0, 1.0)
	// Position at right edge using constraints (reactive via Effect)
	ui.Eq(mask0.Right, float64(outputs[0].ModeWidth)).Add() // Right edge of output
	ui.Eq(mask0.Width(), float64(surfaceW0)).Add()          // Fixed width
	ui.Eq(mask0.Top, 0).Add()
	ui.Eq(mask0.Height(), float64(surfaceH0)).Add() // Fixed height

	// Create mask on output 1 - shows right 70% of content at left edge
	mask1, surfaceW1, surfaceH1 := createMask(outputs[1], "portal-output-1",
		0.3, 1.0, 0.0, 1.0)
	// Position at left edge using constraints (reactive via Effect)
	ui.Eq(mask1.Left, 0).Add()
	ui.Eq(mask1.Top, 0).Add()
	ui.Eq(mask1.Width(), float64(surfaceW1)).Add()   // Fixed width
	ui.Eq(mask1.Height(), float64(surfaceH1)).Add() // Fixed height
	log.Println()
	log.Println("Portal running... Press Ctrl+C to exit")

	// Run event loopz
	if err := disp.Run(); err != nil {
		log.Fatalf("Dispatch error: %v", err)
	}
}
