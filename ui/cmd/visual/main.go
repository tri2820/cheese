package main

import (
	"log"

	"github.com/tri2820/cheese/client-toolkit/buffer"
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/signals"
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

	// Create surface
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		log.Fatalf("Failed to create surface: %v", err)
	}
	defer surf.Close()

	// Create layer surface
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

	// Create renderer
	renderer, err := buffer.NewRenderer(buffer.RendererConfig{
		Shm:     disp.Shm(),
		Target:  layer,
		Format:  client.WlShmFormatArgb8888,
		Buffers: 2,
	})
	if err != nil {
		log.Fatalf("Failed to create renderer: %v", err)
	}
	defer renderer.Close()

	// Create UI layout - this sets up OnRender callback
	layout := ui.NewLayout()
	renderer.SetManualMode(true)
	layout.SetRenderer(renderer)

	// Create a frame for the bar
	bar := layout.NewFrame()
	ui.Eq(bar.Left, 0).Add()
	ui.Eq(bar.Top, 0).Add()

	// Set color to dark gray (like bar.go)
	bar.Color.Set("#303030")

	// Watch for renderer dimension changes and update bar bounds reactively
	signals.Effect(func() {
		w := layout.Width().Get()
		h := layout.Height().Get()
		if w > 0 && h > 0 {
			log.Printf("Renderer dimensions: %dx%d", w, h)
			bar.Right.Set(float64(w))
			bar.Bottom.Set(float64(h))
		}
	}, layout.Width(), layout.Height())

	log.Println("Gray bar running... Press Ctrl+C to exit")

	// Run event loop (dispatches Wayland events including configure)
	if err := disp.Run(); err != nil {
		log.Fatalf("Dispatch error: %v", err)
	}
}
