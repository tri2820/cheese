package ui

import (
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/signals"
)

// ReactiveDisplay wraps display.Display with a reactive outputs signal.
type ReactiveDisplay struct {
	disp    *display.Display
	outputs signals.Signal[[]*display.Output]
}

// Connect wraps display.Connect, creating a ReactiveDisplay with reactive outputs.
func Connect(config display.Config) (*ReactiveDisplay, error) {
	disp, err := display.Connect(config)
	if err != nil {
		return nil, err
	}

	d := &ReactiveDisplay{
		disp:    disp,
		outputs: signals.New(disp.Outputs()),
	}

	// Set up handler to update signal on hotplug
	disp.OnOutput(func(output *display.Output, added bool) {
		// Rebuild outputs list and update signal
		d.outputs.Set(disp.Outputs())
	})

	return d, nil
}

// Run runs the Wayland event loop.
func (d *ReactiveDisplay) Run() error {
	return d.disp.Run()
}

// Close closes the display connection.
func (d *ReactiveDisplay) Close() error {
	return d.disp.Close()
}

// OutputsSignal returns a signal that emits the current list of outputs.
// The signal updates whenever outputs are added or removed via hotplug.
func (d *ReactiveDisplay) OutputsSignal() signals.Signal[[]*display.Output] {
	return d.outputs
}

// Display returns the underlying display.Display for direct access.
func (d *ReactiveDisplay) Display() *display.Display {
	return d.disp
}
