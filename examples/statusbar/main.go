package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/protocols/wlr_layer_shell_unstable_v1"
)

type Display struct {
	display    *client.WlDisplay
	registry   *client.WlRegistry
	compositor *client.WlCompositor
	layerShell *wlr_layer_shell_unstable_v1.ZwlrLayerShellV1
	shm        *client.WlShm
	output     *client.WlOutput
	hasXrgb    bool
}

type Buffer struct {
	buffer  *client.WlBuffer
	shmData []byte
	busy    bool
}

type StatusBar struct {
	display      *Display
	width        int
	height       int
	surface      *client.WlSurface
	layerSurface *wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1
	buffers      [2]Buffer
	callback     *client.WlCallback
	configured   bool
}

func handle(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// Event handlers for Display

func (d *Display) handleShmFormat(ev client.WlShmFormatEvent) {
	if ev.Format == uint32(client.WlShmFormatXrgb8888) {
		d.hasXrgb = true
	}
}

func (d *Display) handleRegistryGlobal(ev client.WlRegistryGlobalEvent) {
	switch ev.Interface {
	case "wl_compositor":
		if ev.Version >= 1 {
			comp := client.NewWlCompositor(d.display.Context())
			if err := d.registry.Bind(ev.Name, "wl_compositor", 1, comp); err != nil {
				log.Printf("failed to bind compositor: %v", err)
				return
			}
			d.compositor = comp
		}
	case "zwlr_layer_shell_v1":
		if ev.Version >= 1 {
			shell := wlr_layer_shell_unstable_v1.NewZwlrLayerShellV1(d.display.Context())
			if err := d.registry.Bind(ev.Name, "zwlr_layer_shell_v1", 1, shell); err != nil {
				log.Printf("failed to bind layer shell: %v", err)
				return
			}
			d.layerShell = shell
			log.Println("Bound to wlr-layer-shell")
		}
	case "wl_shm":
		if ev.Version >= 1 {
			shm := client.NewWlShm(d.display.Context())
			if err := d.registry.Bind(ev.Name, "wl_shm", 1, shm); err != nil {
				log.Printf("failed to bind shm: %v", err)
				return
			}
			d.shm = shm
			d.shm.SetFormatHandler(d.handleShmFormat)
		}
	case "wl_output":
		if ev.Version >= 1 && d.output == nil {
			output := client.NewWlOutput(d.display.Context())
			if err := d.registry.Bind(ev.Name, "wl_output", 1, output); err != nil {
				log.Printf("failed to bind output: %v", err)
				return
			}
			d.output = output
		}
	}
}

func (d *Display) handleRegistryGlobalRemove(ev client.WlRegistryGlobalRemoveEvent) {
	// Handle global removal if needed
}

// Event handlers for Buffer

func (buf *Buffer) handleBufferRelease(ev client.WlBufferReleaseEvent) {
	buf.busy = false
}

// Event handlers for StatusBar

func (sb *StatusBar) handleLayerSurfaceConfigure(ev wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1ConfigureEvent) {
	log.Printf("Layer surface configure: serial=%d, width=%d, height=%d", ev.Serial, ev.Width, ev.Height)

	// If compositor suggests dimensions, use them
	if ev.Width > 0 {
		sb.width = int(ev.Width)
	}
	if ev.Height > 0 {
		sb.height = int(ev.Height)
	}

	handle(sb.layerSurface.AckConfigure(ev.Serial))

	if !sb.configured {
		sb.configured = true
		sb.redraw(nil, 0)
	}
}

func (sb *StatusBar) handleLayerSurfaceClosed(ev wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1ClosedEvent) {
	log.Println("Layer surface closed")
	os.Exit(0)
}

func (sb *StatusBar) handleCallbackDone(ev client.WlCallbackDoneEvent) {
	sb.redraw(sb.callback, ev.CallbackData)
}

// Helper functions

func createShmBuffer(sb *StatusBar, buffer *Buffer, width int, height int, format client.WlShmFormat) error {
	stride := width * 4
	size := stride * height

	fd, err := CreateAnonymousFile(int64(size))
	if err != nil {
		return err
	}
	defer fd.Close()

	data, err := Mmap(int(fd.Fd()), 0, size, ProtRead|ProtWrite, MapShared)
	if err != nil {
		return err
	}

	pool, err := sb.display.shm.CreatePool(uintptr(fd.Fd()), int32(size))
	if err != nil {
		return err
	}

	buf, err := pool.CreateBuffer(0, int32(width), int32(height), int32(stride), uint32(format))
	if err != nil {
		return err
	}

	buffer.buffer = buf
	buf.SetReleaseHandler(buffer.handleBufferRelease)

	if err := pool.Destroy(); err != nil {
		return err
	}

	buffer.shmData = data
	return nil
}

func createDisplay() *Display {
	d := &Display{
		hasXrgb: false,
	}

	display, err := client.Connect("")
	if err != nil {
		log.Fatal("Could not connect to Wayland:", err)
	}
	d.display = display

	// Set error handler
	d.display.SetErrorHandler(func(ev client.WlDisplayErrorEvent) {
		log.Fatalf("Display error: object_id=%v code=%d message=%s\n",
			ev.ObjectId, ev.Code, ev.Message)
	})

	registry, err := d.display.GetRegistry()
	if err != nil {
		log.Fatal("Could not get registry:", err)
	}
	d.registry = registry

	d.registry.SetGlobalHandler(d.handleRegistryGlobal)
	d.registry.SetGlobalRemoveHandler(d.handleRegistryGlobalRemove)

	// Roundtrip to get globals
	done := false
	callback, err := d.display.Sync()
	handle(err)
	callback.SetDoneHandler(func(ev client.WlCallbackDoneEvent) {
		done = true
	})

	for !done {
		if err := d.display.Context().Dispatch(); err != nil {
			log.Fatal("Dispatch error:", err)
		}
	}

	if d.shm == nil {
		log.Fatal("No wl_shm global")
	}

	if d.layerShell == nil {
		log.Fatal("No zwlr_layer_shell_v1 global - compositor doesn't support wlr-layer-shell")
	}

	// Another roundtrip to get formats
	done = false
	callback, err = d.display.Sync()
	handle(err)
	callback.SetDoneHandler(func(ev client.WlCallbackDoneEvent) {
		done = true
	})

	for !done {
		if err := d.display.Context().Dispatch(); err != nil {
			log.Fatal("Dispatch error:", err)
		}
	}

	if !d.hasXrgb {
		log.Fatal("WL_SHM_FORMAT_XRGB8888 not available")
	}

	return d
}

func createStatusBar(d *Display, width, height int) *StatusBar {
	sb := &StatusBar{
		display: d,
		width:   width,
		height:  height,
	}

	surf, err := d.compositor.CreateSurface()
	if err != nil {
		log.Fatal("Cannot create surface:", err)
	}
	sb.surface = surf

	// Create layer surface (this is the key difference from xdg-shell!)
	layerSurf, err := d.layerShell.GetLayerSurface(
		sb.surface,
		d.output, // nil for any output, or specific output
		uint32(wlr_layer_shell_unstable_v1.ZwlrLayerShellV1LayerTop),
		"statusbar",
	)
	if err != nil {
		log.Fatal("Cannot create layer surface:", err)
	}
	sb.layerSurface = layerSurf

	// Set up event handlers
	sb.layerSurface.SetConfigureHandler(sb.handleLayerSurfaceConfigure)
	sb.layerSurface.SetClosedHandler(sb.handleLayerSurfaceClosed)

	// Configure the layer surface
	// Anchor to top left and right = full width bar at top
	anchor := wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1AnchorTop |
		wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1AnchorLeft |
		wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1AnchorRight

	handle(sb.layerSurface.SetAnchor(uint32(anchor)))
	handle(sb.layerSurface.SetSize(0, uint32(height))) // width=0 means "full width"
	handle(sb.layerSurface.SetExclusiveZone(int32(height))) // Reserve space

	// Commit to trigger first configure
	handle(sb.surface.Commit())

	return sb
}

func (sb *StatusBar) nextBuffer() *Buffer {
	var buffer *Buffer

	if !sb.buffers[0].busy {
		buffer = &sb.buffers[0]
	} else if !sb.buffers[1].busy {
		buffer = &sb.buffers[1]
	} else {
		log.Println("All buffers busy")
		return nil
	}

	if buffer.buffer == nil {
		err := createShmBuffer(sb, buffer, sb.width, sb.height, client.WlShmFormatXrgb8888)
		if err != nil {
			log.Println("Failed to create buffer:", err)
			return nil
		}
	}

	return buffer
}

func (sb *StatusBar) paintPixels(time uint32, buf *Buffer) {
	currentTime := time / 1000 // Convert to seconds
	hours := (currentTime / 3600) % 24
	minutes := (currentTime / 60) % 60
	seconds := currentTime % 60

	timeStr := fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)

	// Fill with dark gray background
	for i := 0; i < len(buf.shmData); i += 4 {
		buf.shmData[i] = 0x30     // B
		buf.shmData[i+1] = 0x30   // G
		buf.shmData[i+2] = 0x30   // R
		buf.shmData[i+3] = 0xff   // A
	}

	// Draw animated gradient on left side
	barWidth := 200
	for y := 0; y < sb.height; y++ {
		for x := 0; x < barWidth && x < sb.width; x++ {
			idx := (y*sb.width + x) * 4
			if idx+3 < len(buf.shmData) {
				v := (x + int(time)/16) * 0x0080401
				buf.shmData[idx] = byte(v & 0xff)           // B
				buf.shmData[idx+1] = byte((v >> 8) & 0xff)  // G
				buf.shmData[idx+2] = byte((v >> 16) & 0xff) // R
				buf.shmData[idx+3] = 0xff                   // A
			}
		}
	}

	// Simple "text" rendering - just show a white block where text would be
	// In a real app, you'd use a font rendering library like freetype
	textX := sb.width/2 - 40
	textY := sb.height/2 - 8
	textWidth := 80
	textHeight := 16

	_ = timeStr // Would be used for actual text rendering

	for y := textY; y < textY+textHeight && y < sb.height; y++ {
		for x := textX; x < textX+textWidth && x < sb.width; x++ {
			if x >= 0 && y >= 0 {
				idx := (y*sb.width + x) * 4
				if idx+3 < len(buf.shmData) {
					buf.shmData[idx] = 0xff   // B
					buf.shmData[idx+1] = 0xff // G
					buf.shmData[idx+2] = 0xff // R
					buf.shmData[idx+3] = 0xff // A
				}
			}
		}
	}
}

func (sb *StatusBar) redraw(callback *client.WlCallback, time uint32) {
	buffer := sb.nextBuffer()
	if buffer == nil {
		return
	}

	sb.paintPixels(time, buffer)

	handle(sb.surface.Attach(buffer.buffer, 0, 0))
	handle(sb.surface.Damage(0, 0, int32(sb.width), int32(sb.height)))

	cb, err := sb.surface.Frame()
	handle(err)
	sb.callback = cb
	sb.callback.SetDoneHandler(sb.handleCallbackDone)

	handle(sb.surface.Commit())
	buffer.busy = true
}

func main() {
	log.Println("Starting Cheese Status Bar (waybar-like)...")

	display := createDisplay()
	// Create a bar-like window - compositor will set actual width
	statusbar := createStatusBar(display, 1920, 30)

	log.Println("Status bar is running! It should appear at the top of your screen.")
	log.Println("Press Ctrl+C to exit.")

	// Start a ticker for updates
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			// Trigger a redraw every second
			if statusbar.configured {
				// Would normally trigger redraw here
			}
		}
	}()

	// Event loop
	for {
		if err := display.display.Context().Dispatch(); err != nil {
			log.Printf("Dispatch error: %v", err)
			break
		}
	}

	log.Println("Cheese Status Bar exiting")
}
