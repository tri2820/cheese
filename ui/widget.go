package ui

import (
	"log"
	"math"
	"sync"

	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/signals"
)

// Widget manages a cohesive collection of contents that share N masks (one per output).
type Widget struct {
	mu       sync.RWMutex
	layout   *Layout
	contents []*Content // Coordinate-free visual elements (Rectangle, Label, etc.)
	masks    []*Mask    // Masks for rendering on different outputs

	// Width/ContentHeight_at96DPI define the logical content size
	// in pixels at 96 DPI baseline. The renderer computes the physical buffer as
	// Width * dpi/96, ensuring DPI-normalized rendering.
	// Zero means content renders at mask size (no DPI normalization).
	Width  float64
	Height float64
}

// NewWidget creates a new widget attached to the given layout.
func NewWidget(layout *Layout) *Widget {
	w := &Widget{
		layout:   layout,
		contents: make([]*Content, 0),
		masks:    make([]*Mask, 0),
	}
	layout.addWidget(w)
	return w
}

// NewMask creates a mask for a specific output/layer.
// The mask embeds a LayoutItem that positions the entire widget on that output.
func (w *Widget) NewMask(disp *display.Display, output *client.WlOutput, config LayerConfig) (*Mask, error) {
	mask := &Mask{
		LayoutItem: w.layout.NewLayoutItem(),
		widget:     w,
		display:    disp,
		output:     output,
		config:     config,
	}

	// Create frame and register with layout
	frame, err := mask.getOrCreateFrame()
	if err != nil {
		return nil, err
	}

	w.layout.AddFrame(frame, mask, func(width, height int) {
		// Frame configured callback
		if frame.Ready() {
			frame.ManualRender(0)
		}
	})

	// Reactive margin updates
	// Watch mask.Left/Top and update layer margin when they change
	mask.disposers = append(mask.disposers, signals.Effect(func() {
		if mask.layer == nil {
			return
		}

		// Get raw constraint solver values
		rawLeft := mask.LayoutItem.Left.Get()
		rawTop := mask.LayoutItem.Top.Get()
		rawRight := mask.LayoutItem.Right.Get()
		rawBottom := mask.LayoutItem.Bottom.Get()
		rawWidth := mask.LayoutItem.Width().Get()
		rawHeight := mask.LayoutItem.Height().Get()

		left := int32(math.Round(rawLeft))
		top := int32(math.Round(rawTop))
		width := uint32(math.Round(rawWidth))
		height := uint32(math.Round(rawHeight))

		// Get output info for logging
		output := mask.display.OutputByWlOutput(mask.output)
		var outputHeight int32 = 0
		if output != nil {
			outputHeight = int32(output.ModeHeight)
		}
		bottomMargin := outputHeight - top - int32(height)

		// Only log when we have valid dimensions (constraints resolved)
		if width > 0 && height > 0 && height < 100000 { // Filter out invalid intermediate states
			log.Printf("  %s: Left=%.1f Top=%.1f Right=%.1f Bottom=%.1f | Width=%.1f Height=%.1f",
				mask.config.Name, rawLeft, rawTop, rawRight, rawBottom, rawWidth, rawHeight)
			log.Printf("    surface at (%d, %d) size=%dx%d | top margin: %dpx, bottom margin: %dpx",
				left, top, width, height, top, bottomMargin)
		}

		// Update layer margin (anchor is always top-left)
		if err := mask.layer.SetMargin(top, 0, 0, left); err != nil {
			log.Printf("Failed to update mask margin: %v", err)
		}
	}, mask.LayoutItem.Left, mask.LayoutItem.Top, mask.LayoutItem.Right, mask.LayoutItem.Bottom))

	// Reactive size updates
	// Watch mask.Width()/Height() and update layer size when they change
	mask.disposers = append(mask.disposers, signals.Effect(func() {
		if mask.layer == nil {
			return
		}

		width := uint32(mask.LayoutItem.Width().Get())
		height := uint32(mask.LayoutItem.Height().Get())

		// Don't update if size is zero (constraints not resolved yet)
		if width == 0 || height == 0 {
			return
		}

		// Update layer size
		if err := mask.layer.SetSize(width, height); err != nil {
			log.Printf("Failed to update mask size: %v", err)
		}
		// Commit the change
		if err := mask.layer.Surface().Commit(); err != nil {
			log.Printf("Failed to commit size change: %v", err)
		}
	}, mask.LayoutItem.Width(), mask.LayoutItem.Height()))

	w.mu.Lock()
	w.masks = append(w.masks, mask)
	w.mu.Unlock()

	return mask, nil
}

// Remove removes this widget and all its resources.
func (w *Widget) Remove() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Clean up content LayoutItems (remove symbols from solver)
	for _, content := range w.contents {
		content.Cleanup(w.layout)
	}

	// Clean up masks (remove LayoutItem symbols, destroy frames)
	for _, mask := range w.masks {
		w.layout.removeVar(mask.LayoutItem.Left.state.symbol)
		w.layout.removeVar(mask.LayoutItem.Top.state.symbol)
		w.layout.removeVar(mask.LayoutItem.Right.state.symbol)
		w.layout.removeVar(mask.LayoutItem.Bottom.state.symbol)
		if err := mask.Close(); err != nil {
			return err
		}
	}

	// Remove this widget from layout's widget list
	w.layout.removeWidget(w)

	// Clear slices to allow GC
	w.contents = nil
	w.masks = nil

	// Request re-render
	w.layout.RequestRender()

	return nil
}
