package main

import (
	"fmt"
	"log"

	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/toolkit/buffer"
	"github.com/tri2820/cheese/toolkit/display"
	"github.com/tri2820/cheese/toolkit/shell"
	"github.com/tri2820/cheese/toolkit/surface"
)

func main() {
	log.Println("Starting Cheese Status Bar...")

	// Connect to display with layer shell support
	disp := display.MustConnect(display.Config{
		Required: display.RequiredGlobals{
			Compositor: true,
			Shm:        true,
			LayerShell: true,
		},
	})

	// Check for XRGB format support
	if !disp.HasFormat(client.WlShmFormatXrgb8888) {
		log.Fatal("XRGB8888 not supported")
	}

	height := 30

	// Create surface
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		log.Fatal("Failed to create surface:", err)
	}

	// Create layer surface at the top of the screen
	layer, err := shell.NewLayer(surf, disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          "statusbar",
		Anchor:        shell.AnchorTop | shell.AnchorLeft | shell.AnchorRight,
		Width:         0,  // 0 = full width (will be set by compositor)
		Height:        uint32(height),
		ExclusiveZone: int32(height), // Reserve space for the bar
	})
	if err != nil {
		log.Fatal("Failed to create layer surface:", err)
	}

	// Create renderer - pass layer as target
	renderer, err := buffer.NewRenderer(buffer.RendererConfig{
		Shm:     disp.Shm(),
		Target:  layer,
		Format:  buffer.FormatXRGB8888,
		Buffers: 2,
	})
	if err != nil {
		log.Fatal("Failed to create renderer:", err)
	}

	// Set up render callback - dimensions are passed automatically
	renderer.OnRender(func(w, h int, time uint32, pixels []byte) {
		paintPixels(time, pixels, w, h)
	})

	log.Println("Status bar is running! It should appear at the top of your screen.")
	log.Println("Press Ctrl+C to exit.")

	// Run event loop
	if err := disp.Run(); err != nil {
		log.Printf("Dispatch error: %v", err)
	}

	log.Println("Cheese Status Bar exiting")
}

func paintPixels(time uint32, data []byte, width, height int) {
	// Fill with dark gray background
	for i := 0; i < len(data); i += 4 {
		data[i] = 0x30   // B
		data[i+1] = 0x30 // G
		data[i+2] = 0x30 // R
		data[i+3] = 0xff // A
	}

	// Draw animated gradient on left side
	barWidth := 200
	for y := 0; y < height; y++ {
		for x := 0; x < barWidth && x < width; x++ {
			idx := (y*width + x) * 4
			if idx+3 < len(data) {
				v := (x + int(time)/16) * 0x0080401
				data[idx] = byte(v & 0xff)              // B
				data[idx+1] = byte((v >> 8) & 0xff)     // G
				data[idx+2] = byte((v >> 16) & 0xff)    // R
				data[idx+3] = 0xff                      // A
			}
		}
	}

	// Draw time in center (simplified - just a white block for now)
	currentTime := time / 1000 // Convert to seconds
	hours := (currentTime / 3600) % 24
	minutes := (currentTime / 60) % 60
	seconds := currentTime % 60
	timeStr := fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)

	_ = timeStr // TODO: Render actual text

	// Draw a placeholder white box where the clock would be
	textX := width/2 - 40
	textY := height/2 - 8
	textWidth := 80
	textHeight := 16

	for y := textY; y < textY+textHeight && y < height; y++ {
		for x := textX; x < textX+textWidth && x < width; x++ {
			if x >= 0 && y >= 0 {
				idx := (y*width + x) * 4
				if idx+3 < len(data) {
					data[idx] = 0xff   // B
					data[idx+1] = 0xff // G
					data[idx+2] = 0xff // R
					data[idx+3] = 0xff // A
				}
			}
		}
	}
}
