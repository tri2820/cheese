package surface

import (
	"testing"

	"github.com/tri2820/cheese/protocols/client"
)

func TestSurfaceHandlersCompose(t *testing.T) {
	s := &Surface{
		currentOutputs: make(map[*client.WlOutput]bool),
	}

	output := &client.WlOutput{}
	var events []string

	s.OnEnter(func(*client.WlOutput) {
		events = append(events, "enter-1")
	})
	s.OnEnter(func(*client.WlOutput) {
		events = append(events, "enter-2")
	})
	s.OnFrame(func(uint32) {
		events = append(events, "frame-1")
	})
	s.OnFrame(func(uint32) {
		events = append(events, "frame-2")
	})
	s.OnLeave(func(*client.WlOutput) {
		events = append(events, "leave-1")
	})
	s.OnLeave(func(*client.WlOutput) {
		events = append(events, "leave-2")
	})

	s.handleEnter(client.WlSurfaceEnterEvent{Output: output})
	s.emitFrame(123)
	s.handleLeave(client.WlSurfaceLeaveEvent{Output: output})

	if s.HasFrameHandler() != true {
		t.Fatalf("expected surface to report frame handlers")
	}
	if s.currentOutputs[output] {
		t.Fatalf("expected output to be removed after leave event")
	}

	want := []string{"enter-1", "enter-2", "frame-1", "frame-2", "leave-1", "leave-2"}
	if len(events) != len(want) {
		t.Fatalf("unexpected event count: got %d want %d (%#v)", len(events), len(want), events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("unexpected event order at %d: got %q want %q (%#v)", i, events[i], want[i], events)
		}
	}
}
