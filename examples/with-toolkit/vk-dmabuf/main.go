package main

import (
	"log"
	"time"

	"github.com/tri2820/cheese/toolkit/display"
	"github.com/tri2820/cheese/toolkit/dmabuf"
	"github.com/tri2820/cheese/toolkit/shell"
	"github.com/tri2820/cheese/toolkit/surface"
)

var startTime time.Time

// VulkanProvider implements dmabuf.BufferProvider using Vulkan.
type VulkanProvider struct{}

// NewVulkanProvider creates a new VulkanProvider.
func NewVulkanProvider() *VulkanProvider {
	return &VulkanProvider{}
}

func (p *VulkanProvider) CreateBuffers(width, height, count int) error {
	return initDmaBufBuffers(width, height, count)
}

func (p *VulkanProvider) BufferInfo(index int) dmabuf.BufferInfo {
	buf := &dmabufBuffers[index]
	return dmabuf.BufferInfo{
		Fd:       buf.fd,
		Stride:   buf.stride,
		Format:   dmabuf.FormatXRGB8888,
		Modifier: dmabuf.ModLinear,
	}
}

func (p *VulkanProvider) Render(index, width, height int, frameTime uint32) error {
	// Use wall clock time for smooth animation
	animTime := float32(time.Since(startTime).Seconds())
	return renderToDmaBuf(index, animTime)
}

func (p *VulkanProvider) DestroyBuffers() {
	cleanupDmaBufBuffers()
}

func main() {
	log.Println("Starting Cheese Vulkan DmaBuf Example (Toolkit)...")
	startTime = time.Now()

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

	// Create renderer with Vulkan provider
	renderer, err := dmabuf.NewRenderer(dmabuf.RendererConfig{
		State:    dmabufState,
		Target:   win,
		Buffers:  2,
		Provider: NewVulkanProvider(),
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

	CleanupTriangleRenderer()
	cleanupVulkan()
	log.Println("Cheese Vulkan DmaBuf Example exiting")
}
