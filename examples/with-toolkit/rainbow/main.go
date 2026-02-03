package main

import (
	"log"

	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/toolkit/buffer"
	"github.com/tri2820/cheese/toolkit/display"
	"github.com/tri2820/cheese/toolkit/shell"
	"github.com/tri2820/cheese/toolkit/surface"
)

type App struct {
	display   *display.Display
	window    *shell.Window
	pool      *buffer.Pool
	width     int
	height    int
	waitForConfigure bool
}

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

	// Create window
	win, err := shell.NewToplevel(surf, disp.XdgWmBase(), shell.Config{
		Title: "Cheese Rainbow Bar (Toolkit)",
		AppId: "cheese-rainbow-toolkit",
	})
	if err != nil {
		log.Fatal("Failed to create window:", err)
	}

	// Create SHM pool for double buffering
	poolSize := 2 * width * height * 4
	pool, err := buffer.NewPool(disp.Shm(), buffer.Config{
		Width:  width,
		Height: height * 2,
		Format: buffer.FormatXRGB8888,
		Size:   poolSize,
	})
	if err != nil {
		log.Fatal("Failed to create pool:", err)
	}

	app := &App{
		display:   disp,
		window:    win,
		pool:      pool,
		width:     width,
		height:    height,
		waitForConfigure: true,
	}

	// Set up frame callback handler
	win.Surface().SetFrameHandler(func(time uint32) {
		// Frame callback fires when compositor is done with previous frame
		app.redraw(time)
	})

	// Set up configure handler - directly redraw on first configure
	win.SetConfigureHandler(func() {
		if app.waitForConfigure {
			// First configure - draw immediately without waiting for frame callback
			app.redraw(0)
			app.waitForConfigure = false
		}
	})

	// Pre-allocate 2 slots for double buffering
	_, err = app.pool.NewSlot(width * height * 4)
	if err != nil {
		log.Fatal("Failed to create slot 1:", err)
	}
	_, err = app.pool.NewSlot(width * height * 4)
	if err != nil {
		log.Fatal("Failed to create slot 2:", err)
	}

	// Initial damage
	win.Surface().Damage(0, 0, int32(width), int32(height))

	// Request first frame after configure
	if !app.waitForConfigure {
		win.Surface().RequestFrame(app.redraw)
	}

	log.Println("Rainbow bar is running! Close the window to exit.")

	// Run event loop
	if err := disp.Run(); err != nil {
		log.Printf("Dispatch error: %v", err)
	}

	log.Println("Cheese Rainbow Bar exiting")
}

func (a *App) redraw(time uint32) {
	slot := a.pool.FindFree()
	if slot == nil {
		return
	}

	// Get or create buffer from slot
	stride := a.width * 4
	var buf *buffer.Buffer
	if slot.Buffer() == nil {
		newBuf, err := slot.NewBuffer(a.width, a.height, stride, buffer.FormatXRGB8888)
		if err != nil {
			log.Printf("Failed to create buffer: %v", err)
			return
		}
		buf = newBuf
	} else {
		buf = slot.Buffer()
	}

	a.paintPixels(time, slot)

	winSurf := a.window.Surface()
	winSurf.Attach(buf.WlBuffer(), 0, 0)
	winSurf.Damage(0, 0, int32(a.width), int32(a.height))

	// Request next frame
	winSurf.RequestFrame(a.redraw)

	winSurf.Commit()
	slot.Mark()
}

func (a *App) paintPixels(time uint32, slot *buffer.Slot) {
	data := slot.Mmap()
	iter := 0

	for y := 0; y < a.height; y++ {
		for x := 0; x < a.width; x++ {
			// Create animated rainbow gradient
			v := (x + int(time)/16) * 0x0080401

			r := (v >> 16) & 0xff
			g := (v >> 8) & 0xff
			b := v & 0xff

			data[iter] = byte(b)   // B
			iter++
			data[iter] = byte(g)   // G
			iter++
			data[iter] = byte(r)   // R
			iter++
			data[iter] = 0xff      // A
			iter++
		}
	}
}
