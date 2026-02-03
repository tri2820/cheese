package main

import (
	"log"
	"os"
	"time"

	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/protocols/linux_dmabuf_unstable_v1"
	"github.com/tri2820/cheese/protocols/xdg_shell"
)

var startTime time.Time
var lastFrameTime time.Time
const targetFPS = 60
const minFrameTime = time.Second / targetFPS

type Display struct {
	display    *client.WlDisplay
	registry   *client.WlRegistry
	compositor *client.WlCompositor
	shell      *xdg_shell.XdgWmBase
	dmabuf     *linux_dmabuf_unstable_v1.ZwpLinuxDmabufV1
}

type Buffer struct {
	buffer   *client.WlBuffer
	busy     bool
	bufferIndex int // Index into dmabufBuffers array
}

type Window struct {
	display    *Display
	width      int
	height     int
	surface    *client.WlSurface
	xdgSurface *xdg_shell.XdgSurface
	xdgToplevel *xdg_shell.XdgToplevel
	buffers    [2]Buffer
	callback   *client.WlCallback
	configured bool
}

func handle(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// Event handlers for Display

func (d *Display) handleWmBasePing(ev xdg_shell.XdgWmBasePingEvent) {
	handle(d.shell.Pong(ev.Serial))
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
	case "xdg_wm_base":
		if ev.Version >= 1 {
			shell := xdg_shell.NewXdgWmBase(d.display.Context())
			if err := d.registry.Bind(ev.Name, "xdg_wm_base", 1, shell); err != nil {
				log.Printf("failed to bind xdg_wm_base: %v", err)
				return
			}
			d.shell = shell
			d.shell.SetPingHandler(d.handleWmBasePing)
		}
	case "zwp_linux_dmabuf_v1":
		if ev.Version >= 3 {
			version := ev.Version
			if version > 4 {
				version = 4
			}
			dmabuf := linux_dmabuf_unstable_v1.NewZwpLinuxDmabufV1(d.display.Context())
			if err := d.registry.Bind(ev.Name, "zwp_linux_dmabuf_v1", version, dmabuf); err != nil {
				log.Printf("failed to bind dmabuf: %v", err)
				return
			}
			d.dmabuf = dmabuf
			log.Println("Bound to zwp_linux_dmabuf_v1")
		}
	}
}

func (d *Display) handleRegistryGlobalRemove(ev client.WlRegistryGlobalRemoveEvent) {}

// Event handlers for Buffer

func (buf *Buffer) handleBufferRelease(ev client.WlBufferReleaseEvent) {
	buf.busy = false
}

// Event handlers for Window

func (w *Window) handleSurfaceConfigure(ev xdg_shell.XdgSurfaceConfigureEvent) {
	handle(w.xdgSurface.AckConfigure(ev.Serial))

	if !w.configured {
		w.configured = true
		// Initialize dmabuf buffers after getting actual window size
		log.Printf("Initializing dmabuf buffers at %dx%d...", w.width, w.height)
		err := initDmaBufBuffers(w.width, w.height)
		if err != nil {
			log.Fatal("Failed to initialize dmabuf buffers:", err)
		}
		w.render()
	}
}

func (w *Window) handleToplevelConfigure(ev xdg_shell.XdgToplevelConfigureEvent) {
	if ev.Width > 0 && ev.Height > 0 {
		w.width = int(ev.Width)
		w.height = int(ev.Height)
	}
}

func (w *Window) handleToplevelClose(ev xdg_shell.XdgToplevelCloseEvent) {
	log.Println("Window close requested")
	w.cleanup()
	CleanupTriangleRenderer()
	cleanupDmaBufBuffers()
	cleanupVulkan()
	os.Exit(0)
}

func (w *Window) handleCallbackDone(ev client.WlCallbackDoneEvent) {
	w.render()
}

func (w *Window) cleanup() {
	for i := range w.buffers {
		if w.buffers[i].buffer != nil {
			w.buffers[i].buffer.Destroy()
		}
	}
	if w.xdgToplevel != nil {
		w.xdgToplevel.Destroy()
	}
	if w.xdgSurface != nil {
		w.xdgSurface.Destroy()
	}
	if w.surface != nil {
		w.surface.Destroy()
	}
}

func (w *Window) nextBuffer() *Buffer {
	if !w.buffers[0].busy {
		return &w.buffers[0]
	}
	if !w.buffers[1].busy {
		return &w.buffers[1]
	}
	log.Println("All buffers busy, skipping frame")
	return nil
}

// createWaylandBufferFromDmaBuf creates a wl_buffer from a dmabuf file descriptor
func (w *Window) createWaylandBufferFromDmaBuf(fd int, width, height, stride int, format uint32, buf *Buffer) error {
	if w.display.dmabuf == nil {
		return os.ErrNotExist
	}

	// Create params object
	params, err := w.display.dmabuf.CreateParams()
	if err != nil {
		return err
	}

	// Add the dmabuf plane (single plane for DRM_FORMAT_XRGB8888)
	err = params.Add(
		uintptr(fd),     // dmabuf file descriptor
		0,               // plane index
		0,               // offset
		uint32(stride),  // stride
		0,               // modifier_hi (DRM_FORMAT_MOD_LINEAR = 0)
		0,               // modifier_lo
	)
	if err != nil {
		params.Destroy()
		return err
	}

	// Create the buffer immediately
	buffer, err := params.CreateImmed(
		int32(width),
		int32(height),
		format,
		0, // flags
	)
	if err != nil {
		return err
	}

	buffer.SetReleaseHandler(buf.handleBufferRelease)
	buf.buffer = buffer
	return nil
}

func (w *Window) render() {
	// Frame rate limiting
	if !lastFrameTime.IsZero() {
		elapsed := time.Since(lastFrameTime)
		if elapsed < minFrameTime {
			time.Sleep(minFrameTime - elapsed)
		}
	}
	lastFrameTime = time.Now()

	buffer := w.nextBuffer()
	if buffer == nil {
		return
	}

	// If this is the first time using this buffer slot, create the wl_buffer
	if buffer.buffer == nil {
		dmabufBuf := &dmabufBuffers[buffer.bufferIndex]
		err := w.createWaylandBufferFromDmaBuf(dmabufBuf.fd, w.width, w.height, dmabufBuf.stride, 0x34325258, buffer) // DRM_FORMAT_XRGB8888
		if err != nil {
			log.Printf("Failed to create Wayland buffer from dmabuf: %v", err)
			return
		}
	}

	// Calculate elapsed time for animation
	elapsed := time.Since(startTime).Seconds()
	animTime := float32(elapsed)

	// Render to the dmabuf buffer (Vulkan renders to persistent image and copies to dmabuf)
	err := renderToDmaBuf(buffer.bufferIndex, animTime)
	if err != nil {
		log.Printf("Vulkan render error: %v", err)
		return
	}

	// Attach and present
	handle(w.surface.Attach(buffer.buffer, 0, 0))
	handle(w.surface.Damage(0, 0, int32(w.width), int32(w.height)))

	callback, err := w.surface.Frame()
	handle(err)
	w.callback = callback
	w.callback.SetDoneHandler(w.handleCallbackDone)

	handle(w.surface.Commit())
	buffer.busy = true
}

func createDisplay() *Display {
	d := &Display{}

	display, err := client.Connect("")
	if err != nil {
		log.Fatal("Could not connect to Wayland:", err)
	}
	d.display = display

	d.display.SetErrorHandler(func(ev client.WlDisplayErrorEvent) {
		log.Fatalf("Display error: %s", ev.Message)
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

	if d.compositor == nil {
		log.Fatal("No wl_compositor global")
	}
	if d.shell == nil {
		log.Fatal("No xdg_wm_base global")
	}
	if d.dmabuf == nil {
		log.Fatal("No zwp_linux_dmabuf_v1 global (required for dmabuf import)")
	}

	return d
}

func createWindow(d *Display, width, height int) *Window {
	w := &Window{
		display: d,
		width:   width,
		height:  height,
		buffers: [2]Buffer{{bufferIndex: 0}, {bufferIndex: 1}},
	}

	surf, err := d.compositor.CreateSurface()
	if err != nil {
		log.Fatal("Cannot create surface:", err)
	}
	w.surface = surf

	xdgSurf, err := d.shell.GetXdgSurface(w.surface)
	if err != nil {
		log.Fatal("Cannot get xdg surface:", err)
	}
	w.xdgSurface = xdgSurf
	w.xdgSurface.SetConfigureHandler(w.handleSurfaceConfigure)

	xdgToplevel, err := w.xdgSurface.GetToplevel()
	if err != nil {
		log.Fatal("Cannot get xdg toplevel:", err)
	}
	w.xdgToplevel = xdgToplevel
	w.xdgToplevel.SetConfigureHandler(w.handleToplevelConfigure)
	w.xdgToplevel.SetCloseHandler(w.handleToplevelClose)

	handle(w.xdgToplevel.SetTitle("Cheese Vulkan DmaBuf"))
	handle(w.xdgToplevel.SetAppId("cheese-vk-dmabuf"))
	handle(w.surface.Commit())

	return w
}

func main() {
	log.Println("Starting Cheese Vulkan DmaBuf Example...")
	startTime = time.Now()

	display := createDisplay()
	window := createWindow(display, 400, 300)

	log.Println("Running! Close the window to exit.")

	// Event loop
	for {
		if err := display.display.Context().Dispatch(); err != nil {
			log.Printf("Dispatch error: %v", err)
			break
		}
	}

	window.cleanup()
	CleanupTriangleRenderer()
	cleanupDmaBufBuffers()
	cleanupVulkan()
	log.Println("Cheese Vulkan DmaBuf Example exiting")
}
