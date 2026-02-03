package surface

import (
	"github.com/tri2820/cheese/protocols/client"
)

// Surface represents a Wayland surface.
type Surface struct {
	wlSurface *client.WlSurface
	callback  *client.WlCallback
	onFrame   func(uint32)
}

// New creates a new surface from a wl_compositor.
func New(compositor *client.WlCompositor) (*Surface, error) {
	wlSurface, err := compositor.CreateSurface()
	if err != nil {
		return nil, err
	}

	s := &Surface{
		wlSurface: wlSurface,
	}

	return s, nil
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
