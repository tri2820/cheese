package display

import (
	"fmt"

	"github.com/tri2820/cheese/protocols/client"
)

// Output represents a monitor/display output.
type Output struct {
	wlOutput *client.WlOutput

	// Basic info
	Name        string
	Description string

	// Geometry (position in global compositor space)
	X int32
	Y int32

	// Physical dimensions (millimeters)
	PhysicalWidth  int32
	PhysicalHeight int32

	// Subpixel orientation
	Subpixel client.WlOutputSubpixel

	// Transform applied to the output
	Transform client.WlOutputTransform

	// Manufacturer and model
	Make  string
	Model string

	// Current mode (resolution and refresh rate)
	ModeWidth  int32
	ModeHeight int32
	Refresh    int32 // mHz

	// Scale factor for HiDPI
	Scale int32

	// Ready is set to true when all output events have been received
	Ready bool

	// doneHandler is called when the output becomes ready
	doneHandler func(*Output)
}

// DPI calculates the DPI based on physical size and current resolution.
// Returns 0 if physical dimensions are not available.
func (o *Output) DPI() float64 {
	if o.PhysicalWidth == 0 || o.PhysicalHeight == 0 {
		return 0
	}

	// Calculate DPI for both dimensions and average
	dpiX := float64(o.ModeWidth) * 25.4 / float64(o.PhysicalWidth)
	dpiY := float64(o.ModeHeight) * 25.4 / float64(o.PhysicalHeight)

	return (dpiX + dpiY) / 2
}

// String returns a human-readable description of the output.
func (o *Output) String() string {
	return fmt.Sprintf("%s (%s) - %dx%d @ %.1fHz, scale=%d, DPI=%.1f",
		o.Name, o.Description,
		o.ModeWidth, o.ModeHeight,
		float64(o.Refresh)/1000,
		o.Scale,
		o.DPI(),
	)
}

// newOutput creates a new Output from a wl_output proxy.
func newOutput(wlOutput *client.WlOutput, handler func(*Output)) *Output {
	o := &Output{
		wlOutput:    wlOutput,
		Scale:       1, // Default scale
		doneHandler: handler,
	}

	wlOutput.SetGeometryHandler(o.handleGeometry)
	wlOutput.SetModeHandler(o.handleMode)
	wlOutput.SetScaleHandler(o.handleScale)
	wlOutput.SetNameHandler(o.handleName)
	wlOutput.SetDescriptionHandler(o.handleDescription)
	wlOutput.SetDoneHandler(o.handleDone)

	return o
}

func (o *Output) handleGeometry(ev client.WlOutputGeometryEvent) {
	o.X = ev.X
	o.Y = ev.Y
	o.PhysicalWidth = ev.PhysicalWidth
	o.PhysicalHeight = ev.PhysicalHeight
	o.Subpixel = ev.Subpixel
	o.Transform = ev.Transform
	o.Make = ev.Make
	o.Model = ev.Model
}

func (o *Output) handleMode(ev client.WlOutputModeEvent) {
	// Only care about the current mode
	if ev.Flags&client.WlOutputModeCurrent != 0 {
		o.ModeWidth = ev.Width
		o.ModeHeight = ev.Height
		o.Refresh = ev.Refresh
	}
}

func (o *Output) handleScale(ev client.WlOutputScaleEvent) {
	o.Scale = ev.Factor
}

func (o *Output) handleName(ev client.WlOutputNameEvent) {
	o.Name = ev.Name
}

func (o *Output) handleDescription(ev client.WlOutputDescriptionEvent) {
	o.Description = ev.Description
}

func (o *Output) handleDone(client.WlOutputDoneEvent) {
	o.Ready = true
	if o.doneHandler != nil {
		o.doneHandler(o)
	}
}
