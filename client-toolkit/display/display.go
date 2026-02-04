package display

import (
	"fmt"
	"log"

	"github.com/tri2820/cheese/client-toolkit/dmabuf"
	"github.com/tri2820/cheese/client-toolkit/seat"
	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/protocols/linux_dmabuf_unstable_v1"
	"github.com/tri2820/cheese/protocols/wlr_layer_shell_unstable_v1"
	"github.com/tri2820/cheese/protocols/xdg_shell"
)

// RequiredGlobals specifies which globals are required for the application.
type RequiredGlobals struct {
	Compositor bool
	Shm        bool
	XdgWmBase  bool
	LayerShell bool
	Dmabuf     bool
	Seat       bool
}

// Config configures a new Display connection.
type Config struct {
	// Name is the Wayland display name. Empty means auto-detect.
	Name string

	// Required specifies which globals must be present.
	Required RequiredGlobals

	// ErrorHandler is called on Wayland protocol errors.
	// If nil, a default handler that logs and exits is used.
	ErrorHandler func(client.WlDisplayErrorEvent)

	// OutputHandler is called when an output becomes ready or is removed.
	OutputHandler func(output *Output, added bool)
}

// Display represents a connection to a Wayland compositor.
type Display struct {
	wlDisplay   *client.WlDisplay
	registry    *client.WlRegistry
	compositor  *client.WlCompositor
	shm         *client.WlShm
	shell       *xdg_shell.XdgWmBase
	layerShell  *wlr_layer_shell_unstable_v1.ZwlrLayerShellV1
	dmabufState *dmabuf.State

	// SHM format tracking
	formats map[client.WlShmFormat]bool

	// Output tracking (global name -> Output)
	outputs       map[uint32]*Output
	outputHandler func(output *Output, added bool)

	// wl_output proxy -> Output mapping for surface enter/leave events
	outputsByProxy map[*client.WlOutput]*Output

	// Seat tracking (global name -> Seat)
	seats map[uint32]*seat.Seat

	required RequiredGlobals
}

// Connect connects to a Wayland compositor with the given configuration.
func Connect(config Config) (*Display, error) {
	d := &Display{
		formats:        make(map[client.WlShmFormat]bool),
		outputs:        make(map[uint32]*Output),
		outputsByProxy: make(map[*client.WlOutput]*Output),
		seats:          make(map[uint32]*seat.Seat),
		outputHandler:  config.OutputHandler,
		required:       config.Required,
	}

	// Connect to display
	display, err := client.Connect(config.Name)
	if err != nil {
		return nil, fmt.Errorf("connect to wayland: %w", err)
	}
	d.wlDisplay = display

	// Set error handler
	if config.ErrorHandler != nil {
		d.wlDisplay.SetErrorHandler(config.ErrorHandler)
	} else {
		d.wlDisplay.SetErrorHandler(func(ev client.WlDisplayErrorEvent) {
			log.Fatalf("Display error: object_id=%v code=%d message=%s\n",
				ev.ObjectId, ev.Code, ev.Message)
		})
	}

	// Get registry
	registry, err := d.wlDisplay.GetRegistry()
	if err != nil {
		d.wlDisplay.Context().Close()
		return nil, fmt.Errorf("get registry: %w", err)
	}
	d.registry = registry

	d.registry.SetGlobalHandler(d.handleGlobal)
	d.registry.SetGlobalRemoveHandler(d.handleGlobalRemove)

	// Roundtrip to get all globals
	if err := d.Roundtrip(); err != nil {
		d.wlDisplay.Context().Close()
		return nil, fmt.Errorf("roundtrip for globals: %w", err)
	}

	// Second roundtrip to get SHM formats (if shm was bound)
	if d.shm != nil {
		if err := d.Roundtrip(); err != nil {
			d.wlDisplay.Context().Close()
			return nil, fmt.Errorf("roundtrip for formats: %w", err)
		}
	}

	// Validate required globals
	if d.required.Compositor && d.compositor == nil {
		d.wlDisplay.Context().Close()
		return nil, fmt.Errorf("required global wl_compositor not available")
	}
	if d.required.Shm && d.shm == nil {
		d.wlDisplay.Context().Close()
		return nil, fmt.Errorf("required global wl_shm not available")
	}
	if d.required.XdgWmBase && d.shell == nil {
		d.wlDisplay.Context().Close()
		return nil, fmt.Errorf("required global xdg_wm_base not available")
	}
	if d.required.LayerShell && d.layerShell == nil {
		d.wlDisplay.Context().Close()
		return nil, fmt.Errorf("required global zwlr_layer_shell_v1 not available")
	}
	if d.required.Dmabuf && d.dmabufState == nil {
		d.wlDisplay.Context().Close()
		return nil, fmt.Errorf("required global zwp_linux_dmabuf_v1 not available")
	}
	if d.required.Seat && len(d.seats) == 0 {
		d.wlDisplay.Context().Close()
		return nil, fmt.Errorf("required global wl_seat not available")
	}

	return d, nil
}

// MustConnect connects to a Wayland compositor and panics on error.
func MustConnect(config Config) *Display {
	d, err := Connect(config)
	if err != nil {
		log.Fatal(err)
	}
	return d
}

// handleGlobal handles global object advertisements from the compositor.
func (d *Display) handleGlobal(ev client.WlRegistryGlobalEvent) {
	switch ev.Interface {
	case "wl_compositor":
		if ev.Version >= 1 && d.compositor == nil {
			comp := client.NewWlCompositor(d.wlDisplay.Context())
			if err := d.registry.Bind(ev.Name, "wl_compositor", 1, comp); err != nil {
				log.Printf("failed to bind wl_compositor: %v", err)
				return
			}
			d.compositor = comp
		}
	case "xdg_wm_base":
		if ev.Version >= 1 && d.shell == nil {
			shell := xdg_shell.NewXdgWmBase(d.wlDisplay.Context())
			if err := d.registry.Bind(ev.Name, "xdg_wm_base", 1, shell); err != nil {
				log.Printf("failed to bind xdg_wm_base: %v", err)
				return
			}
			d.shell = shell
			d.shell.SetPingHandler(d.handleWmBasePing)
		}
	case "wl_shm":
		if ev.Version >= 1 && d.shm == nil {
			shm := client.NewWlShm(d.wlDisplay.Context())
			if err := d.registry.Bind(ev.Name, "wl_shm", 1, shm); err != nil {
				log.Printf("failed to bind wl_shm: %v", err)
				return
			}
			d.shm = shm
			d.shm.SetFormatHandler(d.handleShmFormat)
		}
	case "zwlr_layer_shell_v1":
		if ev.Version >= 1 && d.layerShell == nil {
			shell := wlr_layer_shell_unstable_v1.NewZwlrLayerShellV1(d.wlDisplay.Context())
			if err := d.registry.Bind(ev.Name, "zwlr_layer_shell_v1", 1, shell); err != nil {
				log.Printf("failed to bind zwlr_layer_shell_v1: %v", err)
				return
			}
			d.layerShell = shell
		}
	case "zwp_linux_dmabuf_v1":
		if ev.Version >= 3 && d.dmabufState == nil {
			version := ev.Version
			if version > 4 {
				version = 4 // Cap at version 4 for now
			}
			wlDmabuf := linux_dmabuf_unstable_v1.NewZwpLinuxDmabufV1(d.wlDisplay.Context())
			if err := d.registry.Bind(ev.Name, "zwp_linux_dmabuf_v1", version, wlDmabuf); err != nil {
				log.Printf("failed to bind zwp_linux_dmabuf_v1: %v", err)
				return
			}
			d.dmabufState = dmabuf.NewState(wlDmabuf, version)
		}
	case "wl_output":
		// Bind to all outputs (version 2+ gives us scale events)
		version := ev.Version
		if version > 4 {
			version = 4
		}
		wlOutput := client.NewWlOutput(d.wlDisplay.Context())
		if err := d.registry.Bind(ev.Name, "wl_output", version, wlOutput); err != nil {
			log.Printf("failed to bind wl_output: %v", err)
			return
		}
		output := newOutput(wlOutput, func(o *Output) {
			if d.outputHandler != nil {
				d.outputHandler(o, true) // added = true
			}
		})
		d.outputs[ev.Name] = output
		d.outputsByProxy[wlOutput] = output
	case "wl_seat":
		// Bind to all seats (version 5+ gives name)
		version := ev.Version
		if version > 7 {
			version = 7
		}
		wlSeat := client.NewWlSeat(d.wlDisplay.Context())
		if err := d.registry.Bind(ev.Name, "wl_seat", uint32(version), wlSeat); err != nil {
			log.Printf("failed to bind wl_seat: %v", err)
			return
		}
		s, err := seat.New(wlSeat)
		if err != nil {
			log.Printf("failed to create seat: %v", err)
			return
		}
		d.seats[ev.Name] = s
	}
}

// handleGlobalRemove handles global object removals.
func (d *Display) handleGlobalRemove(ev client.WlRegistryGlobalRemoveEvent) {
	if o, ok := d.outputs[ev.Name]; ok {
		delete(d.outputs, ev.Name)
		delete(d.outputsByProxy, o.WlOutput())
		if d.outputHandler != nil {
			d.outputHandler(o, false) // added = false (removed)
		}
	}
}

// handleShmFormat handles SHM format advertisements.
func (d *Display) handleShmFormat(ev client.WlShmFormatEvent) {
	d.formats[ev.Format] = true
}

// handleWmBasePing handles xdg_wm_base ping events.
func (d *Display) handleWmBasePing(ev xdg_shell.XdgWmBasePingEvent) {
	if err := d.shell.Pong(ev.Serial); err != nil {
		log.Printf("failed to pong: %v", err)
	}
}

// Roundtrip performs a synchronous roundtrip to the compositor.
func (d *Display) Roundtrip() error {
	done := false
	var roundtripErr error

	callback, err := d.wlDisplay.Sync()
	if err != nil {
		return err
	}

	callback.SetDoneHandler(func(client.WlCallbackDoneEvent) {
		done = true
	})

	for !done {
		if err := d.wlDisplay.Context().Dispatch(); err != nil {
			roundtripErr = err
			break
		}
	}

	return roundtripErr
}

// Dispatch reads and dispatches one event from the compositor.
func (d *Display) Dispatch() error {
	return d.wlDisplay.Context().Dispatch()
}

// Run starts the event loop and blocks until the display is closed.
func (d *Display) Run() error {
	for {
		if err := d.Dispatch(); err != nil {
			return err
		}
	}
}

// Close closes the connection to the Wayland compositor.
func (d *Display) Close() error {
	return d.wlDisplay.Context().Close()
}

// Context returns the underlying Wayland context.
func (d *Display) Context() *client.Context {
	return d.wlDisplay.Context()
}

// WlDisplay returns the underlying wl_display object.
func (d *Display) WlDisplay() *client.WlDisplay {
	return d.wlDisplay
}

// Compositor returns the wl_compositor global.
func (d *Display) Compositor() *client.WlCompositor {
	return d.compositor
}

// Shm returns the wl_shm global.
func (d *Display) Shm() *client.WlShm {
	return d.shm
}

// HasFormat returns true if the specified SHM format is available.
func (d *Display) HasFormat(format client.WlShmFormat) bool {
	return d.formats[format]
}

// XdgWmBase returns the xdg_wm_base global.
func (d *Display) XdgWmBase() *xdg_shell.XdgWmBase {
	return d.shell
}

// LayerShell returns the zwlr_layer_shell_v1 global.
func (d *Display) LayerShell() *wlr_layer_shell_unstable_v1.ZwlrLayerShellV1 {
	return d.layerShell
}

// Registry returns the wl_registry global for advanced use cases.
func (d *Display) Registry() *client.WlRegistry {
	return d.registry
}

// Dmabuf returns the DMA-BUF state, or nil if not available.
func (d *Display) Dmabuf() *dmabuf.State {
	return d.dmabufState
}

// Outputs returns all detected outputs.
func (d *Display) Outputs() []*Output {
	var outs []*Output
	for _, o := range d.outputs {
		outs = append(outs, o)
	}
	return outs
}

// ReadyOutputs returns all outputs that have finished receiving their information.
func (d *Display) ReadyOutputs() []*Output {
	var outs []*Output
	for _, o := range d.outputs {
		if o.Ready {
			outs = append(outs, o)
		}
	}
	return outs
}

// OutputByWlOutput returns an Output from its wl_output proxy.
// Returns nil if the output is not being tracked.
func (d *Display) OutputByWlOutput(wlOutput *client.WlOutput) *Output {
	return d.outputsByProxy[wlOutput]
}

// SetOutputHandler sets the handler for output add/remove events.
// This can be called after Connect to enable dynamic output handling.
func (d *Display) SetOutputHandler(handler func(output *Output, added bool)) {
	d.outputHandler = handler
}

// Seats returns all detected seats.
func (d *Display) Seats() []*seat.Seat {
	var seats []*seat.Seat
	for _, s := range d.seats {
		seats = append(seats, s)
	}
	return seats
}

// FirstSeat returns the first available seat, or nil if no seats exist.
// This is a convenience method for applications that only need one seat.
func (d *Display) FirstSeat() *seat.Seat {
	for _, s := range d.seats {
		return s
	}
	return nil
}
