package ui

import (
	"log"
	"sync"

	"github.com/tri2820/cheese/protocols/client"
)

// Widget manages a cohesive collection of contents that share N masks (one per output).
type Widget struct {
	mu       sync.RWMutex
	layout   *Layout
	contents []Content // Coordinate-free visual elements (Rectangle, Label, etc.)
	masks    []*Mask   // Masks for rendering on different outputs

	// ContentWidth_at96DPI/ContentHeight_at96DPI define the logical content size
	// in pixels at 96 DPI baseline. The renderer computes the physical buffer as
	// ContentWidth_at96DPI * dpi/96, ensuring DPI-normalized rendering.
	// Zero means content renders at mask size (no DPI normalization).
	ContentWidth_at96DPI  float64
	ContentHeight_at96DPI float64
}

// NewWidget creates a new widget attached to the given layout.
func NewWidget(layout *Layout) *Widget {
	w := &Widget{
		layout:   layout,
		contents: make([]Content, 0),
		masks:    make([]*Mask, 0),
	}
	layout.addWidget(w)
	return w
}

// NewMask creates a mask for a specific output/layer.
// The mask embeds a LayoutItem that positions the entire widget on that output.
func (w *Widget) NewMask(output *client.WlOutput, config LayerConfig) *Mask {
	mask := &Mask{
		LayoutItem: w.layout.NewLayoutItem(),
		widget:     w,
		display:    w.layout.display,
		output:     output,
		config:     config,
	}

	w.mu.Lock()
	w.masks = append(w.masks, mask)
	w.mu.Unlock()

	// Create frame and register with layout
	frame, err := mask.getOrCreateFrame()
	if err != nil {
		log.Printf("Failed to create frame for mask: %v", err)
		return mask
	}

	w.layout.AddFrame(frame, mask, func(width, height int) {
		// Frame configured callback
		if frame.Ready() {
			frame.ManualRender(0)
		}
	})

	return mask
}

// Remove removes this widget and all its resources.
func (w *Widget) Remove() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Clean up masks (remove LayoutItem symbols, destroy frames)
	for _, mask := range w.masks {
		w.layout.removeVar(mask.LayoutItem.Left.state.symbol)
		w.layout.removeVar(mask.LayoutItem.Top.state.symbol)
		w.layout.removeVar(mask.LayoutItem.Right.state.symbol)
		w.layout.removeVar(mask.LayoutItem.Bottom.state.symbol)

		// TODO: Destroy frame - need RemoveFrame method
		// if mask.frame != nil {
		//     w.layout.RemoveFrame(mask.frame)
		// }
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
