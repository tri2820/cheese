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

	// Create ONE shared layout for all outputs (now requires display parameter)
	layout := ui.NewLayout(disp)
	go layout.RenderLoop()

	// Get all outputs
	outputs := disp.ReadyOutputs()
	log.Printf("Found %d outputs", len(outputs))

	// Create ONE widget with contents, then ONE mask per output
	widget := ui.NewWidget(layout)

	// Create contents (coordinate-free)
	rect := widget.NewRectangle()
	rect.Color.Set("#303030") // Dark gray

	label := widget.NewLabel("Hello World")
	label.FontSize.Set(14.0)
	label.Color.Set("#FFFFFF") // White text

	log.Printf("Created widget with rectangle and label contents")

	// Create a separate mask for EACH output
	for i, output := range outputs {
		log.Printf("Setting up output %d: %s at (%d, %d) size %dx%d",
			i, output.Name, output.X, output.Y, output.ModeWidth, output.ModeHeight)

		outputWidth := int(output.ModeWidth)
		barHeight := 30

		// Create mask for this output - mask embeds LayoutItem
		mask := widget.NewMask(output.WlOutput(), ui.LayerConfig{
			Layer:         shell.LayerPositionTop,
			Name:          "separate-top-bar",
			Anchor:        shell.AnchorTop | shell.AnchorLeft | shell.AnchorRight,
			Width:         0, // 0 = full width
			Height:        uint32(barHeight),
			ExclusiveZone: int32(barHeight),
		})

		// Position mask (positions entire widget on this output)
		// Use surface-local coordinates (0, 0) instead of global coordinates
		ui.Eq(mask.Left, 0).Add()
		ui.Eq(mask.Top, 0).Add()
		ui.Eq(mask.Right, outputWidth).Add()
		ui.Eq(mask.Bottom, barHeight).Add()

		log.Printf("  Mask: Left=%.0f, Top=%.0f, Right=%.0f, Bottom=%.0f",
			mask.Left.Get(), mask.Top.Get(), mask.Right.Get(), mask.Bottom.Get())
	}

	log.Println("Separate top bars running... Press Ctrl+C to exit")

	// Run event loop
	if err := disp.Run(); err != nil {
		log.Fatalf("Dispatch error: %v", err)
	}
}
