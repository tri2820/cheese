package main

import (
	"log"
	"os"

	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/protocols/xdg_shell"
)

type Display struct {
	display    *client.WlDisplay
	registry   *client.WlRegistry
	compositor *client.WlCompositor
	shell      *xdg_shell.XdgWmBase
	shm        *client.WlShm
	hasXrgb    bool
}

type Buffer struct {
	buffer  *client.WlBuffer
	shmData []byte
	busy    bool
}

type Window struct {
	display          *Display
	width, height    int
	surface          *client.WlSurface
	xdgSurface       *xdg_shell.XdgSurface
	xdgToplevel      *xdg_shell.XdgToplevel
	buffers          [2]Buffer
	callback         *client.WlCallback
	waitForConfigure bool
}

func handle(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// Event handlers for Display

func (d *Display) handleShmFormat(ev client.WlShmFormatEvent) {
	if ev.Format == client.WlShmFormatXrgb8888 {
		d.hasXrgb = true
	}
}

func (d *Display) handleWmBasePing(ev xdg_shell.XdgWmBasePingEvent) {
	if err := d.shell.Pong(ev.Serial); err != nil {
		log.Printf("failed to pong: %v", err)
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
	}
}

func (d *Display) handleRegistryGlobalRemove(ev client.WlRegistryGlobalRemoveEvent) {
	// Handle global removal if needed
}

// Event handlers for Buffer

func (buf *Buffer) handleBufferRelease(ev client.WlBufferReleaseEvent) {
	buf.busy = false
}

// Event handlers for Window

func (w *Window) handleSurfaceConfigure(ev xdg_shell.XdgSurfaceConfigureEvent) {
	handle(w.xdgSurface.AckConfigure(ev.Serial))

	if w.waitForConfigure {
		w.redraw(nil, 0)
		w.waitForConfigure = false
	}
}

func (w *Window) handleToplevelConfigure(ev xdg_shell.XdgToplevelConfigureEvent) {
	// Handle resize if needed
}

func (w *Window) handleToplevelClose(ev xdg_shell.XdgToplevelCloseEvent) {
	log.Println("Window close requested")
	os.Exit(0)
}

func (w *Window) handleCallbackDone(ev client.WlCallbackDoneEvent) {
	w.redraw(w.callback, ev.CallbackData)
}

// Helper functions

func createShmBuffer(w *Window, buffer *Buffer, width int, height int, format client.WlShmFormat) error {
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

	pool, err := w.display.shm.CreatePool(uintptr(fd.Fd()), int32(size))
	if err != nil {
		return err
	}

	buf, err := pool.CreateBuffer(0, int32(width), int32(height), int32(stride), format)
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

	// Dispatch until sync callback is done
	for !done {
		if err := d.display.Context().Dispatch(); err != nil {
			log.Fatal("Dispatch error:", err)
		}
	}

	if d.shm == nil {
		log.Fatal("No wl_shm global")
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

func createWindow(d *Display, width, height int) *Window {
	w := &Window{
		display: d,
		width:   width,
		height:  height,
	}

	surf, err := d.compositor.CreateSurface()
	if err != nil {
		log.Fatal("Cannot create surface:", err)
	}
	w.surface = surf

	if d.shell == nil {
		log.Fatal("No xdg_wm_base available")
	}

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

	handle(w.xdgToplevel.SetTitle("Cheese Rainbow Bar"))
	handle(w.xdgToplevel.SetAppId("cheese-rainbow"))
	handle(w.surface.Commit())
	w.waitForConfigure = true

	return w
}

func (w *Window) nextBuffer() *Buffer {
	var buffer *Buffer

	if !w.buffers[0].busy {
		buffer = &w.buffers[0]
	} else if !w.buffers[1].busy {
		buffer = &w.buffers[1]
	} else {
		log.Println("All buffers busy")
		return nil
	}

	if buffer.buffer == nil {
		err := createShmBuffer(w, buffer, w.width, w.height, client.WlShmFormatXrgb8888)
		if err != nil {
			log.Println("Failed to create buffer:", err)
			return nil
		}

		// Initialize buffer with white
		for i := range buffer.shmData {
			buffer.shmData[i] = 0xff
		}
	}

	return buffer
}

func (w *Window) paintPixels(time uint32, buf *Buffer) {
	var iter = 0

	for y := 0; y < w.height; y++ {
		for x := 0; x < w.width; x++ {
			// Create animated rainbow gradient
			var v int
			v = (x + int(time)/16) * 0x0080401

			r := (v >> 16) & 0xff
			g := (v >> 8) & 0xff
			b := v & 0xff

			buf.shmData[iter] = byte(b)   // B
			iter++
			buf.shmData[iter] = byte(g)   // G
			iter++
			buf.shmData[iter] = byte(r)   // R
			iter++
			buf.shmData[iter] = 0xff      // A
			iter++
		}
	}
}

func (w *Window) redraw(callback *client.WlCallback, time uint32) {
	// Note: callbacks are automatically destroyed by the compositor after the done event

	buffer := w.nextBuffer()
	if buffer == nil {
		return
	}

	w.paintPixels(time, buffer)

	handle(w.surface.Attach(buffer.buffer, 0, 0))
	handle(w.surface.Damage(0, 0, int32(w.width), int32(w.height)))

	cb, err := w.surface.Frame()
	handle(err)
	w.callback = cb
	w.callback.SetDoneHandler(w.handleCallbackDone)

	handle(w.surface.Commit())
	buffer.busy = true
}

func main() {
	log.Println("Starting Cheese Rainbow Bar...")

	display := createDisplay()
	// Create a bar-like window (wide but short)
	window := createWindow(display, 800, 60)

	// Initialize damage to full surface
	handle(window.surface.Damage(0, 0, int32(window.width), int32(window.height)))

	if !window.waitForConfigure {
		window.redraw(nil, 0)
	}

	log.Println("Rainbow bar is running! Close the window to exit.")

	// Event loop
	for {
		if err := display.display.Context().Dispatch(); err != nil {
			log.Printf("Dispatch error: %v", err)
			break
		}
	}

	log.Println("Cheese Rainbow Bar exiting")
}
