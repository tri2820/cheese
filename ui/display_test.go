package ui

import (
	"errors"
	"testing"

	ctdisplay "github.com/tri2820/cheese/client-toolkit/display"
)

func TestOutputReadsLiveState(t *testing.T) {
	raw := &ctdisplay.Output{
		Name:       "HDMI-A-1",
		X:          10,
		Y:          20,
		ModeWidth:  1920,
		ModeHeight: 1080,
		Refresh:    60000,
		Scale:      2,
	}

	out := newOutput(nil, raw)
	if got := out.Name(); got != "HDMI-A-1" {
		t.Fatalf("out.Name() = %q, want %q", got, "HDMI-A-1")
	}
	if got := out.Width(); got != 1920 {
		t.Fatalf("out.Width() = %d, want 1920", got)
	}

	raw.Name = "DP-1"
	raw.ModeWidth = 2560
	raw.ModeHeight = 1440

	if got := out.Name(); got != "DP-1" {
		t.Fatalf("updated out.Name() = %q, want %q", got, "DP-1")
	}
	if got := out.Width(); got != 2560 {
		t.Fatalf("updated out.Width() = %d, want 2560", got)
	}
	if got := out.Height(); got != 1440 {
		t.Fatalf("updated out.Height() = %d, want 1440", got)
	}

	if got := out.ToPixels(DesignUnit(100)); got <= 0 {
		t.Fatalf("out.ToPixels(100) = %v, want > 0", got)
	}
	if got := out.ToIntPixels(DesignUnit(100)); got != out.ToPixels(DesignUnit(100)).Int() {
		t.Fatalf("out.ToIntPixels(100) = %d, want %d", got, out.ToPixels(DesignUnit(100)).Int())
	}
}

func TestLayoutOnError(t *testing.T) {
	layout := NewLayout()

	var got error
	layout.OnError(func(err error) {
		got = err
	})

	want := errors.New("boom")
	layout.reportError(want)

	if !errors.Is(got, want) {
		t.Fatalf("layout.reportError() got %v, want %v", got, want)
	}
}
