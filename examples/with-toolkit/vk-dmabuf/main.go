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
var lastFrameTime time.Time

const targetFPS = 60
const minFrameTime = time.Second / targetFPS

// App holds the application state
type App struct {
	disp       *display.Display
	surf       *surface.Surface
	win        *shell.ToplevelSurface
	dmabufState *dmabuf.State

	width      int
	height     int
	buffers    [2]*dmabuf.Buffer
	configured bool
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

	// Create app
	app := &App{
		disp:        disp,
		dmabufState: dmabufState,
		width:       400,
		height:      300,
	}

	// Create surface
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		log.Fatal("Failed to create surface:", err)
	}
	app.surf = surf

	// Create toplevel window
	win, err := shell.NewToplevel(surf, disp.XdgWmBase(), shell.ToplevelConfig{
		Title:  "Cheese Vulkan DmaBuf (Toolkit)",
		AppId:  "cheese-vk-dmabuf-toolkit",
		Width:  app.width,
		Height: app.height,
	})
	if err != nil {
		log.Fatal("Failed to create window:", err)
	}
	app.win = win

	// Set up configure handler
	win.SetConfigureHandler(func() {
		app.width = win.Width()
		app.height = win.Height()

		if !app.configured {
			app.configured = true
			log.Printf("Initializing dmabuf buffers at %dx%d...", app.width, app.height)
			if err := initDmaBufBuffers(app.width, app.height); err != nil {
				log.Fatal("Failed to initialize dmabuf buffers:", err)
			}
			app.render()
		}
	})

	// Set up frame callback
	surf.SetFrameHandler(func(time uint32) {
		app.render()
	})

	log.Println("Running! Close the window to exit.")

	// Event loop
	if err := disp.Run(); err != nil {
		log.Printf("Dispatch error: %v", err)
	}

	app.cleanup()
	log.Println("Cheese Vulkan DmaBuf Example exiting")
}

func (app *App) render() {
	// Frame rate limiting
	if !lastFrameTime.IsZero() {
		elapsed := time.Since(lastFrameTime)
		if elapsed < minFrameTime {
			time.Sleep(minFrameTime - elapsed)
		}
	}
	lastFrameTime = time.Now()

	// Find free buffer
	var bufferIndex int = -1
	for i, buf := range app.buffers {
		if buf == nil || !buf.Busy() {
			bufferIndex = i
			break
		}
	}
	if bufferIndex == -1 {
		log.Println("All buffers busy, skipping frame")
		return
	}

	// Create wl_buffer if needed
	if app.buffers[bufferIndex] == nil {
		dmabufBuf := &dmabufBuffers[bufferIndex]

		// Create params and add plane
		params, err := app.dmabufState.CreateParams()
		if err != nil {
			log.Printf("Failed to create params: %v", err)
			return
		}

		err = params.Add(dmabufBuf.fd, 0, 0, uint32(dmabufBuf.stride), dmabuf.ModLinear)
		if err != nil {
			log.Printf("Failed to add plane: %v", err)
			return
		}

		wlBuffer, err := params.CreateImmed(app.width, app.height, dmabuf.FormatXRGB8888, 0)
		if err != nil {
			log.Printf("Failed to create buffer: %v", err)
			return
		}

		app.buffers[bufferIndex] = dmabuf.NewBuffer(wlBuffer, app.width, app.height, dmabuf.FormatXRGB8888)
		app.buffers[bufferIndex].UserData = bufferIndex
		log.Printf("Created dmabuf wl_buffer %d", bufferIndex)
	}

	buf := app.buffers[bufferIndex]

	// Calculate elapsed time for animation
	elapsed := time.Since(startTime).Seconds()
	animTime := float32(elapsed)

	// Render to the dmabuf buffer (Vulkan renders to persistent image and copies to dmabuf)
	if err := renderToDmaBuf(bufferIndex, animTime); err != nil {
		log.Printf("Vulkan render error: %v", err)
		return
	}

	// Attach and present using toolkit surface
	if err := app.surf.Attach(buf.WlBuffer(), 0, 0); err != nil {
		log.Printf("Failed to attach: %v", err)
		return
	}
	if err := app.surf.Damage(0, 0, int32(app.width), int32(app.height)); err != nil {
		log.Printf("Failed to damage: %v", err)
		return
	}

	// Request frame callback
	if err := app.surf.Frame(); err != nil {
		log.Printf("Failed to request frame: %v", err)
	}

	if err := app.surf.Commit(); err != nil {
		log.Printf("Failed to commit: %v", err)
		return
	}

	buf.MarkBusy()
}

func (app *App) cleanup() {
	for _, buf := range app.buffers {
		if buf != nil {
			buf.Destroy()
		}
	}
	CleanupTriangleRenderer()
	cleanupDmaBufBuffers()
	cleanupVulkan()
}
