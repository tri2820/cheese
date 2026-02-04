package seat

import (
	"log"

	"github.com/tri2820/cheese/protocols/client"
)

// Seat represents a Wayland seat (a group of input devices).
type Seat struct {
	wlSeat  *client.WlSeat
	pointer *Pointer
	keyboard interface{} // TODO: implement keyboard wrapper
	touch    interface{} // TODO: implement touch wrapper
}

// New creates a new Seat from a wl_seat.
func New(wlSeat *client.WlSeat) (*Seat, error) {
	s := &Seat{
		wlSeat: wlSeat,
	}

	// Set up capabilities handler
	wlSeat.SetCapabilitiesHandler(s.handleCapabilities)

	return s, nil
}

// handleCapabilities handles seat capability changes.
func (s *Seat) handleCapabilities(ev client.WlSeatCapabilitiesEvent) {
	// Check for pointer capability
	hasPointer := ev.Capabilities&client.WlSeatCapabilityPointer != 0

	if hasPointer && s.pointer == nil {
		// Pointer became available
		pointer, err := s.wlSeat.GetPointer()
		if err != nil {
			log.Printf("failed to get pointer: %v", err)
			return
		}
		s.pointer = NewPointer(pointer)
		log.Printf("seat: pointer acquired")
	} else if !hasPointer && s.pointer != nil {
		// Pointer became unavailable
		s.pointer.Close()
		s.pointer = nil
		log.Printf("seat: pointer released")
	}

	// TODO: handle keyboard and touch capabilities
}

// Pointer returns the pointer for this seat, or nil if not available.
func (s *Seat) Pointer() *Pointer {
	return s.pointer
}

// WlSeat returns the underlying wl_seat object.
func (s *Seat) WlSeat() *client.WlSeat {
	return s.wlSeat
}

// Close destroys the seat and its resources.
func (s *Seat) Close() error {
	if s.pointer != nil {
		s.pointer.Close()
	}
	return s.wlSeat.Release()
}
