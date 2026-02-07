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

	if len(outputs) == 0 {
		log.Fatalf("No outputs found")
	}

	// Create ONE widget with red square (size in 96 DPI units)
	widget := ui.NewWidget(layout)
	widget.Width = 10000.
	widget.Height = 10000.

	rect := widget.NewRectangle()
	rect.Color.Set("#FF0000")
	ui.Eq(rect.Left, 0).Add()
	ui.Eq(rect.Top, 0).Add()
	ui.Eq(rect.Right, widget.Width).Add()
	ui.Eq(rect.Bottom, widget.Height).Add()

	// Create a mask for each output
	for i, output := range outputs {
		squareSize := output.ModeHeight / 2 // Desired physical size: half of monitor height
		// Calculate physical content size at this output's DPI

		log.Printf("Output %d: %s at (%d, %d) size %dx%d",
			i, output.Name, output.X, output.Y, output.ModeWidth, output.ModeHeight)
		log.Printf("  Square size: %d (half of monitor height %d)", squareSize, output.ModeHeight)

		mask := widget.NewMask(output.WlOutput(), ui.LayerConfig{
			Layer: shell.LayerPositionTop,
			Name:  "test-pos-square",
		})

		// Position at top-left corner of the output
		ui.Eq(mask.Left, 0).Add()
		ui.Eq(mask.Top, 0).Add()
		ui.Eq(mask.Width(), float64(squareSize)).Add()
		ui.Eq(mask.Height(), float64(squareSize)).Add()
	}

	log.Println("Test position running... Press Ctrl+C to exit")

	// Run event loop
	if err := disp.Run(); err != nil {
		log.Fatalf("Dispatch error: %v", err)
	}
}
