package main

import (
	"log"

	"github.com/tri2820/cheese/toolkit/display"
	"github.com/tri2820/cheese/toolkit/dmabuf"
	"github.com/tri2820/cheese/toolkit/shell"
	"github.com/tri2820/cheese/toolkit/surface"
)

func main() {
	log.Println("Starting Cheese Vulkan DmaBuf Example (Toolkit)...")

	// Connect to display with required globals
	disp := display.MustConnect(display.Config{
		Required: display.RequiredGlobals{
			Compositor: true,
			XdgWmBase:  true,
			Dmabuf:     true,
		},
	})

	// Get dmabuf state
	dmabufState := disp.Dmabuf()
	if dmabufState == nil {
		log.Fatal("DMA-BUF not available")
	}
	log.Printf("DMA-BUF protocol version: %d", dmabufState.Version())

	// Create surface
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		log.Fatal("Failed to create surface:", err)
	}

	// Create toplevel window
	win, err := shell.NewToplevel(surf, disp.XdgWmBase(), shell.ToplevelConfig{
		Title:  "Cheese Vulkan DmaBuf (Toolkit)",
		AppId:  "cheese-vk-dmabuf-toolkit",
		Width:  400,
		Height: 300,
	})
	if err != nil {
		log.Fatal("Failed to create window:", err)
	}

	// Create renderer with callbacks
	renderer, err := dmabuf.NewRenderer(dmabuf.RendererConfig{
		State:   dmabufState,
		Target:  win,
		Buffers: 2,
		OnCreateBuffers: func(width, height, count int) ([]dmabuf.BufferInfo, error) {
			buffers, err := initDmaBufBuffers(width, height, count)
			if err != nil {
				return nil, err
			}
			infos := make([]dmabuf.BufferInfo, len(buffers))
			for i, b := range buffers {
				infos[i] = dmabuf.BufferInfo{
					Fd:       b.fd,
					Stride:   b.stride,
					Format:   dmabuf.FormatXRGB8888,
					Modifier: dmabuf.ModLinear,
				}
			}
			return infos, nil
		},
		OnRender: func(bufferIndex, width, height int, frameTime uint32) error {
			// Render to Vulkan image at bufferIndex
			animTime := float32(frameTime) / 1000.0
			return renderToDmaBuf(bufferIndex, animTime)
		},
		OnDestroyBuffers: func() {
			// Cleanup Vulkan resources
			CleanupTriangleRenderer()
			cleanupDmaBufBuffers()
		},
	})
	if err != nil {
		log.Fatal("Failed to create renderer:", err)
	}
	defer renderer.Close()

	log.Println("Running! Close the window to exit.")

	// Event loop
	if err := disp.Run(); err != nil {
		log.Printf("Dispatch error: %v", err)
	}

	cleanupVulkan()
	log.Println("Cheese Vulkan DmaBuf Example exiting")
}
