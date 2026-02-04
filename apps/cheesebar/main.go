package main

import (
	"log"
	"sync"

	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/seat"
	"github.com/tri2820/cheese/protocols/client"
)

func main() {
	log.Println("Starting cheesebar...")

	// Create app to manage bars
	app := NewApp()

	// Connect to display with layer shell support
	disp := display.MustConnect(display.Config{
		Required: display.RequiredGlobals{
			Compositor: true,
			Shm:        true,
			LayerShell: true,
		},
	})

	// Check for ARGB format support
	if !disp.HasFormat(client.WlShmFormatArgb8888) {
		log.Fatal("ARGB8888 not supported")
	}

	// Set up pointer for hover tracking
	s := disp.FirstSeat()
	if s != nil {
		app.seat = s
		app.pointer = s.Pointer()
		if app.pointer != nil {
			app.setupPointerHandlers()
		}
	}

	// Set up output handler for monitor plug/unplug events
	disp.SetOutputHandler(func(output *display.Output, added bool) {
		if added {
			app.AddBar(disp, output)
		} else {
			app.RemoveBar(output)
		}
	})

	// Create bars for all existing outputs
	for _, output := range disp.ReadyOutputs() {
		app.AddBar(disp, output)
	}

	log.Println("cheesebar running")

	// Run event loop
	if err := disp.Run(); err != nil {
		log.Printf("Dispatch error: %v", err)
	}
}

// App manages multiple bars, one per monitor
type App struct {
	bars        map[*client.WlOutput]*Bar
	mu          sync.Mutex
	focusedBar  *Bar
	seat        *seat.Seat
	pointer     *seat.Pointer
}

func NewApp() *App {
	return &App{
		bars: make(map[*client.WlOutput]*Bar),
	}
}

func (a *App) AddBar(disp *display.Display, output *display.Output) {
	a.mu.Lock()
	defer a.mu.Unlock()

	wlOutput := output.WlOutput()
	name := output.Name
	log.Printf("Adding bar for output: %s", name)

	bar, err := NewBar(disp, output)
	if err != nil {
		log.Printf("Failed to create bar for %s: %v", name, err)
		return
	}

	a.bars[wlOutput] = bar
}

func (a *App) RemoveBar(output *display.Output) {
	a.mu.Lock()
	defer a.mu.Unlock()

	wlOutput := output.WlOutput()
	name := output.Name
	log.Printf("Removing bar for output: %s", name)

	if bar, ok := a.bars[wlOutput]; ok {
		if a.focusedBar == bar {
			a.focusedBar = nil
		}
		bar.Close()
		delete(a.bars, wlOutput)
	}
}

// setupPointerHandlers sets up the shared pointer handlers for all bars.
func (a *App) setupPointerHandlers() {
	a.pointer.SetEnterHandler(func(ev client.WlPointerEnterEvent) {
		a.mu.Lock()
		defer a.mu.Unlock()

		// Find which bar this surface belongs to
		for _, bar := range a.bars {
			if ev.Surface == bar.surface.WlSurface() {
				a.focusedBar = bar
				bar.setHovered(true)
				bar.setMousePos(ev.SurfaceX, ev.SurfaceY)
				break
			}
		}
	})

	a.pointer.SetLeaveHandler(func(ev client.WlPointerLeaveEvent) {
		a.mu.Lock()
		defer a.mu.Unlock()

		if a.focusedBar != nil && ev.Surface == a.focusedBar.surface.WlSurface() {
			a.focusedBar.setHovered(false)
			a.focusedBar = nil
		}
	})

	a.pointer.SetMotionHandler(func(ev client.WlPointerMotionEvent) {
		a.mu.Lock()
		defer a.mu.Unlock()

		if a.focusedBar != nil {
			a.focusedBar.setMousePos(ev.SurfaceX, ev.SurfaceY)
		}
	})
}
