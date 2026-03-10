package display

import (
	"testing"

	"github.com/tri2820/cheese/protocols/client"
)

func TestOutputOnReadyFiresOnce(t *testing.T) {
	output := &Output{}

	var readyCount int
	var doneCount int
	output.OnReady(func(*Output) {
		readyCount++
	})
	output.OnDone(func(client.WlOutputDoneEvent) {
		doneCount++
	})

	output.handleDone(client.WlOutputDoneEvent{})
	output.handleDone(client.WlOutputDoneEvent{})

	if !output.Ready {
		t.Fatalf("output should be marked ready after first done event")
	}
	if readyCount != 1 {
		t.Fatalf("expected ready handler to fire once, got %d", readyCount)
	}
	if doneCount != 2 {
		t.Fatalf("expected done handler to fire for every done event, got %d", doneCount)
	}
}

func TestOutputOnGeometryComposes(t *testing.T) {
	output := &Output{}

	var calls []int32
	output.OnGeometry(func(ev client.WlOutputGeometryEvent) {
		calls = append(calls, ev.X)
	})
	output.OnGeometry(func(ev client.WlOutputGeometryEvent) {
		calls = append(calls, ev.Y)
	})

	output.handleGeometry(client.WlOutputGeometryEvent{
		X:              10,
		Y:              20,
		PhysicalWidth:  300,
		PhysicalHeight: 200,
	})

	if output.X != 10 || output.Y != 20 {
		t.Fatalf("expected output coordinates to update, got (%d, %d)", output.X, output.Y)
	}
	if len(calls) != 2 || calls[0] != 10 || calls[1] != 20 {
		t.Fatalf("unexpected geometry callback sequence: %#v", calls)
	}
}
