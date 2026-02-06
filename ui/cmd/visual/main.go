package main

import (
	"log"

	"github.com/tri2820/cheese/client-toolkit/buffer"
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
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

	// Create ONE shared layout for all outputs
	layout := ui.NewLayout()
	go layout.RenderLoop()

	// [APP] means *This is specific logic for this Bar app*

	// Create ONE rect that will span all monitors
	// [APP] Since we know that we are creating a vertical bar on the left, we can use a single view that spans the entire virtual desktop.
	// Each output's layer will then render the relevant portion of this shared view.
	// The reason this works is because our outputs are ALSO stacked vertically also, and ALSO aligned left.
	// If we wanted to draw the bar on the right, or draw so that it appears horizontally continuous across outputs at the top,
	// then we would need to create separate view for each output
	// and synchronize them.
	rect := layout.NewRectangle()
	ui.Eq(rect.Left, 0).Add()
	ui.Eq(rect.Top, 0).Add()
	rect.Color.Set("#303030")

	// Calculate virtual desktop bounds
	// [APP] This is needed because we need to set the rect size to cover the entire desktop
	outputs := disp.ReadyOutputs()
	var maxX, maxY int
	for _, output := range outputs {
		outputX := int(output.X)
		outputY := int(output.Y)
		right := outputX + int(output.ModeWidth)
		bottom := outputY + int(output.ModeHeight)
		if right > maxX {
			maxX = right
		}
		if bottom > maxY {
			maxY = bottom
		}
	}

	// Set rect to span entire virtual desktop (vertical rect on left)
	rect.Right.Set(30) // Fixed width 30px
	rect.Bottom.Set(float64(maxY))

	log.Printf("Virtual desktop: %dx%d, %d outputs", maxX, maxY, len(outputs))

	// Create a surface+layer+frame for EACH output
	for i, output := range outputs {
		log.Printf("Setting up output %d: %s at (%d, %d) size %dx%d",
			i, output.Name, output.X, output.Y, output.ModeWidth, output.ModeHeight)

		// Create surface for this output
		surf, err := surface.New(disp.Compositor())
		if err != nil {
			log.Fatalf("Failed to create surface: %v", err)
		}

		// Create layer surface for THIS output
		layer, err := shell.NewLayer(surf, disp.LayerShell(), shell.LayerConfig{
			Layer:         shell.LayerPositionTop,
			Name:          "ui-test",
			Anchor:        shell.AnchorTop | shell.AnchorLeft | shell.AnchorBottom,
			Width:         30,
			Height:        0,
			ExclusiveZone: 30,
			Output:        output.WlOutput(), // Bind to this output
		})
		if err != nil {
			log.Fatalf("Failed to create layer: %v", err)
		}

		// Create frame for this layer
		frame, err := buffer.NewFrame(disp.Shm(), layer, disp, buffer.FrameConfig{
			Format:  client.WlShmFormatArgb8888,
			Buffers: 2,
		})
		if err != nil {
			log.Fatalf("Failed to create frame: %v", err)
		}
		frame.SetManualMode(true)

		// Add frame to shared layout
		layout.AddFrame(frame, func(w, h int) {
			log.Printf("Frame configured: %dx%d at output pos (%d, %d)",
				w, h, frame.OutputX(), frame.OutputY())
		})
	}

	log.Println("Gray rect running... Press Ctrl+C to exit")

	// Run event loop
	if err := disp.Run(); err != nil {
		log.Fatalf("Dispatch error: %v", err)
	}
}
