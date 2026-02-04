package seat

import (
	"github.com/tri2820/cheese/protocols/client"
)

// Pointer represents a Wayland pointer input device.
type Pointer struct {
	wlPointer *client.WlPointer
	onEnter   func(ev client.WlPointerEnterEvent)
	onLeave   func(client.WlPointerLeaveEvent)
	onMotion  func(ev client.WlPointerMotionEvent)
	onButton  func(ev client.WlPointerButtonEvent)
	onAxis    func(ev client.WlPointerAxisEvent)
	onFrame   func(client.WlPointerFrameEvent)
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
	if p.onEnter != nil {
		p.onEnter(ev)
	}
}

func (p *Pointer) handleLeave(ev client.WlPointerLeaveEvent) {
	if p.onLeave != nil {
		p.onLeave(ev)
	}
}

func (p *Pointer) handleMotion(ev client.WlPointerMotionEvent) {
	if p.onMotion != nil {
		p.onMotion(ev)
	}
}

func (p *Pointer) handleButton(ev client.WlPointerButtonEvent) {
	if p.onButton != nil {
		p.onButton(ev)
	}
}

func (p *Pointer) handleAxis(ev client.WlPointerAxisEvent) {
	if p.onAxis != nil {
		p.onAxis(ev)
	}
}

func (p *Pointer) handleFrame(ev client.WlPointerFrameEvent) {
	if p.onFrame != nil {
		p.onFrame(ev)
	}
}

// SetEnterHandler sets the handler for pointer enter events.
// The event provides the surface and surface-local x/y coordinates.
func (p *Pointer) SetEnterHandler(fn func(ev client.WlPointerEnterEvent)) {
	p.onEnter = fn
}

// SetLeaveHandler sets the handler for pointer leave events.
func (p *Pointer) SetLeaveHandler(fn func(ev client.WlPointerLeaveEvent)) {
	p.onLeave = fn
}

// SetMotionHandler sets the handler for pointer motion events.
func (p *Pointer) SetMotionHandler(fn func(ev client.WlPointerMotionEvent)) {
	p.onMotion = fn
}

// SetButtonHandler sets the handler for pointer button events.
func (p *Pointer) SetButtonHandler(fn func(ev client.WlPointerButtonEvent)) {
	p.onButton = fn
}

// SetAxisHandler sets the handler for pointer axis (scroll) events.
func (p *Pointer) SetAxisHandler(fn func(ev client.WlPointerAxisEvent)) {
	p.onAxis = fn
}

// SetFrameHandler sets the handler for pointer frame events.
func (p *Pointer) SetFrameHandler(fn func(ev client.WlPointerFrameEvent)) {
	p.onFrame = fn
}

// WlPointer returns the underlying wl_pointer object.
func (p *Pointer) WlPointer() *client.WlPointer {
	return p.wlPointer
}

// Close destroys the pointer.
func (p *Pointer) Close() error {
	return p.wlPointer.Release()
}
