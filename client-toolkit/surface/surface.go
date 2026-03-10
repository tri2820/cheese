package surface

import (
	"github.com/tri2820/cheese/protocols/client"
)

// Surface represents a Wayland surface.
type Surface struct {
	wlSurface      *client.WlSurface
	callback       *client.WlCallback
	frameHandlers  []func(uint32)
	enterHandlers  []func(*client.WlOutput)
	leaveHandlers  []func(*client.WlOutput)
	currentOutputs map[*client.WlOutput]bool
}

// New creates a new surface from a wl_compositor.
func New(compositor *client.WlCompositor) (*Surface, error) {
	wlSurface, err := compositor.CreateSurface()
	if err != nil {
		return nil, err
	}

	s := &Surface{
		wlSurface:      wlSurface,
		currentOutputs: make(map[*client.WlOutput]bool),
	}

	// Set up enter/leave handlers
	wlSurface.SetEnterHandler(s.handleEnter)
	wlSurface.SetLeaveHandler(s.handleLeave)

	return s, nil
}

func (s *Surface) handleEnter(ev client.WlSurfaceEnterEvent) {
	s.currentOutputs[ev.Output] = true
	s.emitEnter(ev.Output)
}

func (s *Surface) handleLeave(ev client.WlSurfaceLeaveEvent) {
	delete(s.currentOutputs, ev.Output)
	s.emitLeave(ev.Output)
}

// WlSurface returns the underlying wl_surface.
func (s *Surface) WlSurface() *client.WlSurface {
	return s.wlSurface
}

// Commit commits the pending surface state.
func (s *Surface) Commit() error {
	return s.wlSurface.Commit()
}

// Damage requests a damage update for the surface.
func (s *Surface) Damage(x, y, width, height int32) error {
	return s.wlSurface.Damage(x, y, width, height)
}

// Attach attaches a buffer to the surface.
func (s *Surface) Attach(buffer *client.WlBuffer, x, y int32) error {
	return s.wlSurface.Attach(buffer, x, y)
}

// Frame sets a frame callback and returns immediately.
// The callback will be invoked when the compositor has finished
// processing the frame.
func (s *Surface) Frame() error {
	callback, err := s.wlSurface.Frame()
	if err != nil {
		return err
	}
	s.callback = callback

	callback.SetDoneHandler(func(ev client.WlCallbackDoneEvent) {
		s.emitFrame(ev.CallbackData)
	})

	return nil
}

func (s *Surface) emitFrame(time uint32) {
	for _, fn := range append([]func(uint32){}, s.frameHandlers...) {
		if fn != nil {
			fn(time)
		}
	}
}

func (s *Surface) emitEnter(output *client.WlOutput) {
	for _, fn := range append([]func(*client.WlOutput){}, s.enterHandlers...) {
		if fn != nil {
			fn(output)
		}
	}
}

func (s *Surface) emitLeave(output *client.WlOutput) {
	for _, fn := range append([]func(*client.WlOutput){}, s.leaveHandlers...) {
		if fn != nil {
			fn(output)
		}
	}
}

// OnFrame registers a function to be called when the frame callback fires.
// The function receives the callback time in milliseconds.
func (s *Surface) OnFrame(fn func(uint32)) {
	if fn == nil {
		return
	}
	s.frameHandlers = append(s.frameHandlers, fn)
}

// HasFrameHandler returns true if a frame handler is set.
func (s *Surface) HasFrameHandler() bool {
	return len(s.frameHandlers) > 0
}

// OnEnter registers a function to be called when the surface enters an output.
func (s *Surface) OnEnter(fn func(*client.WlOutput)) {
	if fn == nil {
		return
	}
	s.enterHandlers = append(s.enterHandlers, fn)
}

// OnLeave registers a function to be called when the surface leaves an output.
func (s *Surface) OnLeave(fn func(*client.WlOutput)) {
	if fn == nil {
		return
	}
	s.leaveHandlers = append(s.leaveHandlers, fn)
}

// Outputs returns all outputs the surface is currently on.
func (s *Surface) Outputs() []*client.WlOutput {
	var outs []*client.WlOutput
	for o := range s.currentOutputs {
		outs = append(outs, o)
	}
	return outs
}

// Close destroys the surface.
func (s *Surface) Close() error {
	return s.wlSurface.Destroy()
}
