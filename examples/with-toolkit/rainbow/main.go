package main

import (
	"log"

	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/client-toolkit/buffer"
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/surface"
)

func main() {
	log.Println("Starting Cheese Rainbow Bar (Toolkit)...")

	// Connect to display
	disp := display.MustConnect(display.Config{
		Required: display.RequiredGlobals{
			Compositor: true,
			Shm:        true,
			XdgWmBase:  true,
		},
	})

	// Check for XRGB format support
	if !disp.HasFormat(client.WlShmFormatXrgb8888) {
		log.Fatal("XRGB8888 not supported")
	}

	width, height := 800, 60

	// Create surface
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		log.Fatal("Failed to create surface:", err)
	}

	// Create toplevel surface
	win, err := shell.NewToplevel(surf, disp.XdgWmBase(), shell.ToplevelConfig{
		Title:  "Cheese Rainbow Bar (Toolkit)",
		AppId:  "cheese-rainbow-toolkit",
		Width:  width,
		Height: height,
	})
	if err != nil {
		log.Fatal("Failed to create window:", err)
	}

	// Create renderer - pass window as target
	renderer, err := buffer.NewRenderer(buffer.RendererConfig{
		Shm:     disp.Shm(),
		Target:  win,
		Format:  client.WlShmFormatXrgb8888,
		Buffers: 2,
	})
	if err != nil {
		log.Fatal("Failed to create renderer:", err)
	}

	// Set up render callback - dimensions are passed automatically
	renderer.OnRender(func(w, h int, time uint32, pixels []byte) {
		paintPixels(time, pixels, w, h)
	})

	log.Println("Rainbow bar is running! Close the window to exit.")

	// Run event loop
	if err := disp.Run(); err != nil {
		log.Printf("Dispatch error: %v", err)
	}

	log.Println("Cheese Rainbow Bar exiting")
}

func paintPixels(time uint32, data []byte, width, height int) {
	iter := 0

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Create animated rainbow gradient
			v := (x + int(time)/16) * 0x0080401

			r := (v >> 16) & 0xff
			g := (v >> 8) & 0xff
			b := v & 0xff

			data[iter] = byte(b) // B
			iter++
			data[iter] = byte(g) // G
			iter++
			data[iter] = byte(r) // R
			iter++
			data[iter] = 0xff    // A
			iter++
		}
	}
}
