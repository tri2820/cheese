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
	geometryHandlers    []func(client.WlOutputGeometryEvent)
	modeHandlers        []func(client.WlOutputModeEvent)
	scaleHandlers       []func(client.WlOutputScaleEvent)
	nameHandlers        []func(client.WlOutputNameEvent)
	descriptionHandlers []func(client.WlOutputDescriptionEvent)
	doneHandlers        []func(client.WlOutputDoneEvent)
	readyHandlers       []func(*Output)
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
		o.OnReady(handler)
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
	for _, fn := range append([]func(client.WlOutputGeometryEvent){}, o.geometryHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
}

func (o *Output) handleMode(ev client.WlOutputModeEvent) {
	o.ModeWidth = ev.Width
	o.ModeHeight = ev.Height
	o.Refresh = ev.Refresh
	for _, fn := range append([]func(client.WlOutputModeEvent){}, o.modeHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
}

func (o *Output) handleScale(ev client.WlOutputScaleEvent) {
	o.Scale = ev.Factor
	for _, fn := range append([]func(client.WlOutputScaleEvent){}, o.scaleHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
}

func (o *Output) handleName(ev client.WlOutputNameEvent) {
	o.Name = ev.Name
	for _, fn := range append([]func(client.WlOutputNameEvent){}, o.nameHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
}

func (o *Output) handleDescription(ev client.WlOutputDescriptionEvent) {
	o.Description = ev.Description
	for _, fn := range append([]func(client.WlOutputDescriptionEvent){}, o.descriptionHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
}

func (o *Output) handleDone(ev client.WlOutputDoneEvent) {
	firstReady := !o.Ready
	o.Ready = true
	for _, fn := range append([]func(client.WlOutputDoneEvent){}, o.doneHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
	if firstReady {
		for _, fn := range append([]func(*Output){}, o.readyHandlers...) {
			if fn != nil {
				fn(o)
			}
		}
	}
}

// OnGeometry registers a geometry handler.
func (o *Output) OnGeometry(fn func(client.WlOutputGeometryEvent)) {
	if fn == nil {
		return
	}
	o.geometryHandlers = append(o.geometryHandlers, fn)
}

// OnMode registers a mode handler.
func (o *Output) OnMode(fn func(client.WlOutputModeEvent)) {
	if fn == nil {
		return
	}
	o.modeHandlers = append(o.modeHandlers, fn)
}

// OnScale registers a scale handler.
func (o *Output) OnScale(fn func(client.WlOutputScaleEvent)) {
	if fn == nil {
		return
	}
	o.scaleHandlers = append(o.scaleHandlers, fn)
}

// OnName registers a name handler.
func (o *Output) OnName(fn func(client.WlOutputNameEvent)) {
	if fn == nil {
		return
	}
	o.nameHandlers = append(o.nameHandlers, fn)
}

// OnDescription registers a description handler.
func (o *Output) OnDescription(fn func(client.WlOutputDescriptionEvent)) {
	if fn == nil {
		return
	}
	o.descriptionHandlers = append(o.descriptionHandlers, fn)
}

// OnDone registers a done handler.
func (o *Output) OnDone(fn func(client.WlOutputDoneEvent)) {
	if fn == nil {
		return
	}
	o.doneHandlers = append(o.doneHandlers, fn)
}

// OnReady registers a callback that runs only once, on the first done event.
func (o *Output) OnReady(fn func(*Output)) {
	if fn == nil {
		return
	}
	o.readyHandlers = append(o.readyHandlers, fn)
}
