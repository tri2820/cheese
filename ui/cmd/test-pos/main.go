package main

import (
	"log"

	"github.com/tri2820/cheese/ui"
)

func main() {
	// Connect to Wayland display
	disp, err := ui.Connect(ui.DisplayConfig{
		Required: ui.RequiredGlobals{
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
	layout := ui.NewLayout()
	layout.Start()
	defer layout.Close()
	layout.OnError(func(err error) {
		log.Printf("ui error: %v", err)
	})

	// Get all outputs
	outputs := disp.Outputs()
	log.Printf("Found %d outputs", len(outputs))

	if len(outputs) == 0 {
		log.Fatalf("No outputs found")
	}

	// Create ONE widget with red square (size in 96 DPI units)
	widget := ui.NewWidget(layout)
	widget.SetSize(10000, 10000)

	rect := widget.NewRectangle()
	rect.Color.Set("#FF0000")
	widget.Fill(rect.LayoutItem)

	// Create a mask for each output
	for i, output := range outputs {
		squareSize := output.Height() / 2 // Desired physical size: half of monitor height
		// Calculate physical content size at this output's DPI

		log.Printf("Output %d: %s at (%d, %d) size %dx%d",
			i, output.Name(), output.X(), output.Y(), output.Width(), output.Height())
		log.Printf("  Square size: %d (half of monitor height %d)", squareSize, output.Height())

		mask, err := widget.NewMask(output, ui.LayerConfig{
			Layer: ui.LayerTop,
			Name:  "test-pos-square",
		})
		if err != nil {
			log.Fatalf("Failed to create mask for output %s: %v", output.Name(), err)
		}

		// Position at top-left corner of the output
		mask.Own(
			ui.Eq(mask.Left, 0),
			ui.Eq(mask.Top, 0),
			ui.Eq(mask.Width(), float64(squareSize)),
			ui.Eq(mask.Height(), float64(squareSize)),
		)
	}

	log.Println("Test position running... Press Ctrl+C to exit")

	// Run event loop
	if err := disp.Run(); err != nil {
		log.Fatalf("Dispatch error: %v", err)
	}
}
