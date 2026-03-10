package ui

import (
	"sort"

	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/signals"
)

type DisplayConfig = display.Config
type RequiredGlobals = display.RequiredGlobals
type DesignUnit = display.DesignUnit
type Pixel = display.Pixel

// Output is the ui-facing monitor abstraction.
// It hides the raw wl_output proxy while exposing live getters over the current output state.
type Output struct {
	display *ReactiveDisplay
	raw     *display.Output
}

func newOutput(display *ReactiveDisplay, raw *display.Output) *Output {
	return &Output{
		display: display,
		raw:     raw,
	}
}

func (o *Output) Name() string {
	if o == nil || o.raw == nil {
		return ""
	}
	return o.raw.Name
}

func (o *Output) Description() string {
	if o == nil || o.raw == nil {
		return ""
	}
	return o.raw.Description
}

func (o *Output) X() int32 {
	if o == nil || o.raw == nil {
		return 0
	}
	return o.raw.X
}

func (o *Output) Y() int32 {
	if o == nil || o.raw == nil {
		return 0
	}
	return o.raw.Y
}

func (o *Output) Width() int32 {
	if o == nil || o.raw == nil {
		return 0
	}
	return o.raw.ModeWidth
}

func (o *Output) Height() int32 {
	if o == nil || o.raw == nil {
		return 0
	}
	return o.raw.ModeHeight
}

func (o *Output) Refresh() int32 {
	if o == nil || o.raw == nil {
		return 0
	}
	return o.raw.Refresh
}

func (o *Output) Scale() int32 {
	if o == nil || o.raw == nil {
		return 0
	}
	return o.raw.Scale
}

func (o *Output) wlOutput() *client.WlOutput {
	if o == nil || o.raw == nil {
		return nil
	}
	return o.raw.WlOutput()
}

func (o *Output) displayHandle() *display.Display {
	if o == nil || o.display == nil {
		return nil
	}
	return o.display.disp
}

func (o *Output) DPI() float64 {
	if o == nil || o.raw == nil {
		return 0
	}
	return o.raw.DPI()
}

func (o *Output) DPIOrDefault() float64 {
	if o == nil || o.raw == nil {
		return 96.0
	}
	return o.raw.DPIOrDefault()
}

func (o *Output) ToPixels(distance DesignUnit) Pixel {
	if o == nil || o.raw == nil {
		return 0
	}
	return o.raw.ToPixels(distance)
}

func (o *Output) ToIntPixels(distance DesignUnit) int {
	if o == nil || o.raw == nil {
		return 0
	}
	return o.raw.ToIntPixels(distance)
}

// ReactiveDisplay wraps display.Display with a reactive outputs signal.
type ReactiveDisplay struct {
	disp    *display.Display
	outputs signals.Signal[[]*Output]
	known   map[*display.Output]*Output
}

// Connect wraps display.Connect, creating a ReactiveDisplay with reactive outputs.
func Connect(config DisplayConfig) (*ReactiveDisplay, error) {
	disp, err := display.Connect(config)
	if err != nil {
		return nil, err
	}

	d := &ReactiveDisplay{
		disp:  disp,
		known: make(map[*display.Output]*Output),
	}
	d.outputs = signals.New(d.snapshotOutputs())

	// Set up handler to update signal on hotplug
	disp.OnOutput(func(output *display.Output, added bool) {
		// Rebuild outputs list and update signal
		d.outputs.Set(d.snapshotOutputs())
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
func (d *ReactiveDisplay) OutputsSignal() signals.Signal[[]*Output] {
	return d.outputs
}

// Outputs returns the current snapshot of ready outputs.
func (d *ReactiveDisplay) Outputs() []*Output {
	return d.outputs.Get()
}

// WatchOutputs runs fn immediately for the current output topology and again whenever it changes.
// If fn returns a cleanup function, it runs before the next invocation and when the watch is canceled.
func (d *ReactiveDisplay) WatchOutputs(fn func([]*Output) func()) func() {
	if fn == nil {
		return nil
	}

	cancel := signals.Scope(func() signals.CancelFunc {
		return fn(d.outputs.Get())
	}, d.outputs)

	return func() {
		if cancel != nil {
			cancel()
		}
	}
}

// Display returns the underlying display.Display for direct access.
func (d *ReactiveDisplay) Display() *display.Display {
	return d.disp
}

func (d *ReactiveDisplay) snapshotOutputs() []*Output {
	rawOutputs := d.disp.ReadyOutputs()
	sort.Slice(rawOutputs, func(i, j int) bool {
		if rawOutputs[i].X != rawOutputs[j].X {
			return rawOutputs[i].X < rawOutputs[j].X
		}
		if rawOutputs[i].Y != rawOutputs[j].Y {
			return rawOutputs[i].Y < rawOutputs[j].Y
		}
		return rawOutputs[i].Name < rawOutputs[j].Name
	})
	outputs := make([]*Output, 0, len(rawOutputs))
	live := make(map[*display.Output]struct{}, len(rawOutputs))

	for _, raw := range rawOutputs {
		live[raw] = struct{}{}

		out, ok := d.known[raw]
		if !ok {
			out = newOutput(d, raw)
			d.known[raw] = out
			raw.OnDone(func(client.WlOutputDoneEvent) {
				d.outputs.Set(d.snapshotOutputs())
			})
		}
		outputs = append(outputs, out)
	}

	for raw := range d.known {
		if _, ok := live[raw]; !ok {
			delete(d.known, raw)
		}
	}

	return outputs
}
