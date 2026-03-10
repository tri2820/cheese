package seat

import (
	"github.com/tri2820/cheese/protocols/client"
)

// Pointer represents a Wayland pointer input device.
type Pointer struct {
	wlPointer      *client.WlPointer
	enterHandlers  []func(ev client.WlPointerEnterEvent)
	leaveHandlers  []func(client.WlPointerLeaveEvent)
	motionHandlers []func(ev client.WlPointerMotionEvent)
	buttonHandlers []func(ev client.WlPointerButtonEvent)
	axisHandlers   []func(ev client.WlPointerAxisEvent)
	frameHandlers  []func(client.WlPointerFrameEvent)
}

// NewPointer creates a new Pointer from a wl_pointer.
func NewPointer(wlPointer *client.WlPointer) *Pointer {
	p := &Pointer{
		wlPointer: wlPointer,
	}

	// Set up event handlers
	wlPointer.SetEnterHandler(p.handleEnter)
	wlPointer.SetLeaveHandler(p.handleLeave)
	wlPointer.SetMotionHandler(p.handleMotion)
	wlPointer.SetButtonHandler(p.handleButton)
	wlPointer.SetAxisHandler(p.handleAxis)
	wlPointer.SetFrameHandler(p.handleFrame)

	return p
}

func (p *Pointer) handleEnter(ev client.WlPointerEnterEvent) {
	for _, fn := range append([]func(client.WlPointerEnterEvent){}, p.enterHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
}

func (p *Pointer) handleLeave(ev client.WlPointerLeaveEvent) {
	for _, fn := range append([]func(client.WlPointerLeaveEvent){}, p.leaveHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
}

func (p *Pointer) handleMotion(ev client.WlPointerMotionEvent) {
	for _, fn := range append([]func(client.WlPointerMotionEvent){}, p.motionHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
}

func (p *Pointer) handleButton(ev client.WlPointerButtonEvent) {
	for _, fn := range append([]func(client.WlPointerButtonEvent){}, p.buttonHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
}

func (p *Pointer) handleAxis(ev client.WlPointerAxisEvent) {
	for _, fn := range append([]func(client.WlPointerAxisEvent){}, p.axisHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
}

func (p *Pointer) handleFrame(ev client.WlPointerFrameEvent) {
	for _, fn := range append([]func(client.WlPointerFrameEvent){}, p.frameHandlers...) {
		if fn != nil {
			fn(ev)
		}
	}
}

// OnEnter registers a handler for pointer enter events.
// The event provides the surface and surface-local x/y coordinates.
func (p *Pointer) OnEnter(fn func(ev client.WlPointerEnterEvent)) {
	if fn == nil {
		return
	}
	p.enterHandlers = append(p.enterHandlers, fn)
}

// OnLeave registers a handler for pointer leave events.
func (p *Pointer) OnLeave(fn func(ev client.WlPointerLeaveEvent)) {
	if fn == nil {
		return
	}
	p.leaveHandlers = append(p.leaveHandlers, fn)
}

// OnMotion registers a handler for pointer motion events.
func (p *Pointer) OnMotion(fn func(ev client.WlPointerMotionEvent)) {
	if fn == nil {
		return
	}
	p.motionHandlers = append(p.motionHandlers, fn)
}

// OnButton registers a handler for pointer button events.
func (p *Pointer) OnButton(fn func(ev client.WlPointerButtonEvent)) {
	if fn == nil {
		return
	}
	p.buttonHandlers = append(p.buttonHandlers, fn)
}

// OnAxis registers a handler for pointer axis (scroll) events.
func (p *Pointer) OnAxis(fn func(ev client.WlPointerAxisEvent)) {
	if fn == nil {
		return
	}
	p.axisHandlers = append(p.axisHandlers, fn)
}

// OnFrame registers a handler for pointer frame events.
func (p *Pointer) OnFrame(fn func(ev client.WlPointerFrameEvent)) {
	if fn == nil {
		return
	}
	p.frameHandlers = append(p.frameHandlers, fn)
}

// WlPointer returns the underlying wl_pointer object.
func (p *Pointer) WlPointer() *client.WlPointer {
	return p.wlPointer
}

// Close destroys the pointer.
func (p *Pointer) Close() error {
	return p.wlPointer.Release()
}
