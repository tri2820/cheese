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

	// Create UI layout with virtual desktop
	layout := ui.NewLayout()

	// Start render loop
	go layout.RenderLoop()

	// Create a rectangle for the bar
	bar := layout.NewRectangle()
	ui.Eq(bar.Left, 0).Add()
	ui.Eq(bar.Top, 0).Add()

	// Set color to dark gray (like bar.go)
	bar.Color.Set("#303030")

	// Create surface
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		log.Fatalf("Failed to create surface: %v", err)
	}
	defer surf.Close()

	// Create layer surface (compositor chooses output)
	layer, err := shell.NewLayer(surf, disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          "ui-test",
		Anchor:        shell.AnchorTop | shell.AnchorLeft | shell.AnchorRight,
		Width:         0, // Full width
		Height:        24,
		ExclusiveZone: 24,
	})
	if err != nil {
		log.Fatalf("Failed to create layer: %v", err)
	}
	defer layer.Close()

	// Create frame
	frame, err := buffer.NewFrame(disp.Shm(), layer, disp, buffer.FrameConfig{
		Format:  client.WlShmFormatArgb8888,
		Buffers: 2,
	})
	if err != nil {
		log.Fatalf("Failed to create frame: %v", err)
	}
	defer frame.Close()
	frame.SetManualMode(true)

	// Add frame to layout - Layout handles OnConfigured and OnRender wiring
	layout.AddFrame(frame, func(w, h int) {
		log.Printf("Output configured: %dx%d at output pos (%d, %d)", w, h, frame.OutputX(), frame.OutputY())
		// Update bar bounds to match virtual dimensions
		bar.Right.Set(float64(frame.OutputX()) + float64(w))
		bar.Bottom.Set(float64(frame.OutputY()) + float64(h))
	})

	log.Println("Gray bar running... Press Ctrl+C to exit")

	// Run event loop (dispatches Wayland events including configure)
	if err := disp.Run(); err != nil {
		log.Fatalf("Dispatch error: %v", err)
	}
}
