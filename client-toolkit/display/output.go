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

	// Callbacks for output events
	onGeometry    func(client.WlOutputGeometryEvent)
	onMode        func(client.WlOutputModeEvent)
	onScale       func(client.WlOutputScaleEvent)
	onName        func(client.WlOutputNameEvent)
	onDescription func(client.WlOutputDescriptionEvent)
	onDone        func(client.WlOutputDoneEvent)
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

// DPIOrDefault returns the output's DPI, or 96.0 if not available.
func (o *Output) DPIOrDefault() float64 {
	dpi := o.DPI()
	if dpi == 0 {
		return 96.0
	}
	return dpi
}

// ScaleFrom96DPI converts a distance in 96 DPI pixels to physical pixels for this output.
// For example, if the output is 192 DPI and distance96 is 100, this returns 200.
func (o *Output) ScaleFrom96DPI(distance96 float64) int {
	return int(distance96 * o.DPIOrDefault() / 96.0)
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

// WlOutput returns the underlying wl_output proxy.
func (o *Output) WlOutput() *client.WlOutput {
	return o.wlOutput
}

// newOutput creates a new Output from a wl_output proxy.
func newOutput(wlOutput *client.WlOutput, handler func(*Output)) *Output {
	o := &Output{
		wlOutput: wlOutput,
		Scale:    1, // Default scale
	}
	if handler != nil {
		o.onDone = func(ev client.WlOutputDoneEvent) {
			handler(o)
		}
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
	if o.onGeometry != nil {
		o.onGeometry(ev)
	}
}

func (o *Output) handleMode(ev client.WlOutputModeEvent) {
	o.ModeWidth = ev.Width
	o.ModeHeight = ev.Height
	o.Refresh = ev.Refresh
	if o.onMode != nil {
		o.onMode(ev)
	}
}

func (o *Output) handleScale(ev client.WlOutputScaleEvent) {
	o.Scale = ev.Factor
	if o.onScale != nil {
		o.onScale(ev)
	}
}

func (o *Output) handleName(ev client.WlOutputNameEvent) {
	o.Name = ev.Name
	if o.onName != nil {
		o.onName(ev)
	}
}

func (o *Output) handleDescription(ev client.WlOutputDescriptionEvent) {
	o.Description = ev.Description
	if o.onDescription != nil {
		o.onDescription(ev)
	}
}

func (o *Output) handleDone(ev client.WlOutputDoneEvent) {
	o.Ready = true
	if o.onDone != nil {
		o.onDone(ev)
	}
}

// SetGeometryHandler sets a callback for geometry events.
func (o *Output) SetGeometryHandler(fn func(client.WlOutputGeometryEvent)) {
	o.onGeometry = fn
}

// SetModeHandler sets a callback for mode events.
func (o *Output) SetModeHandler(fn func(client.WlOutputModeEvent)) {
	o.onMode = fn
}

// SetScaleHandler sets a callback for scale events.
func (o *Output) SetScaleHandler(fn func(client.WlOutputScaleEvent)) {
	o.onScale = fn
}

// SetNameHandler sets a callback for name events.
func (o *Output) SetNameHandler(fn func(client.WlOutputNameEvent)) {
	o.onName = fn
}

// SetDescriptionHandler sets a callback for description events.
func (o *Output) SetDescriptionHandler(fn func(client.WlOutputDescriptionEvent)) {
	o.onDescription = fn
}

// SetDoneHandler sets a callback for done events.
func (o *Output) SetDoneHandler(fn func(client.WlOutputDoneEvent)) {
	o.onDone = fn
}
