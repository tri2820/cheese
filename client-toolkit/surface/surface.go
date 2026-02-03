package surface

import (
	"github.com/tri2820/cheese/protocols/client"
)

// Surface represents a Wayland surface.
type Surface struct {
	wlSurface   *client.WlSurface
	callback    *client.WlCallback
	onFrame     func(uint32)
	onEnter     func(*client.WlOutput)
	onLeave     func(*client.WlOutput)
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
	if s.onEnter != nil {
		s.onEnter(ev.Output)
	}
}

func (s *Surface) handleLeave(ev client.WlSurfaceLeaveEvent) {
	delete(s.currentOutputs, ev.Output)
	if s.onLeave != nil {
		s.onLeave(ev.Output)
	}
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
		if s.onFrame != nil {
			s.onFrame(ev.CallbackData)
		}
	})

	return nil
}

// SetFrameHandler sets a function to be called when the frame callback fires.
// The function receives the callback time in milliseconds.
func (s *Surface) SetFrameHandler(fn func(uint32)) {
	s.onFrame = fn
}

// HasFrameHandler returns true if a frame handler is set.
func (s *Surface) HasFrameHandler() bool {
	return s.onFrame != nil
}

// RequestFrame requests a frame callback with the given handler.
// This is a convenience method that combines SetFrameHandler and Frame.
func (s *Surface) RequestFrame(fn func(uint32)) error {
	s.SetFrameHandler(fn)
	return s.Frame()
}

// SetEnterHandler sets a function to be called when the surface enters an output.
func (s *Surface) SetEnterHandler(fn func(*client.WlOutput)) {
	s.onEnter = fn
}

// SetLeaveHandler sets a function to be called when the surface leaves an output.
func (s *Surface) SetLeaveHandler(fn func(*client.WlOutput)) {
	s.onLeave = fn
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
