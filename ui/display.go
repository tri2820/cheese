package ui

import (
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/signals"
)

// Display wraps display.Display with reactive outputs signal.
type Display struct {
	disp    *display.Display
	outputs signals.Signal[[]*display.Output]
}

// Connect wraps display.Connect, creating a new Display with reactive outputs.
func Connect(config display.Config) (*Display, error) {
	disp, err := display.Connect(config)
	if err != nil {
		return nil, err
	}

	d := &Display{
		disp:    disp,
		outputs: signals.New(disp.Outputs()),
	}

	// Set up handler to update signal on hotplug
	disp.SetOutputHandler(func(output *display.Output, added bool) {
		// Rebuild outputs list and update signal
		d.outputs.Set(disp.Outputs())
	})

	return d, nil
}

// Run runs the Wayland event loop.
func (d *Display) Run() error {
	return d.disp.Run()
}

// Close closes the display connection.
func (d *Display) Close() error {
	return d.disp.Close()
}

// Outputs returns a signal that emits the current list of outputs.
// The signal updates whenever outputs are added or removed via hotplug.
func (d *Display) Outputs() signals.Signal[[]*display.Output] {
	return d.outputs
}

// Display returns the underlying display.Display for direct access.
func (d *Display) Display() *display.Display {
	return d.disp
}
