package ui

import (
	"fmt"
	"math"
	"sync"

	"github.com/tri2820/cheese/signals"
)

// Widget manages a cohesive collection of contents that share N masks (one per output).
type Widget struct {
	mu       sync.RWMutex
	layout   *Layout
	contents []*Content // Coordinate-free visual elements (Rectangle, Label, etc.)
	masks    []*Mask    // Masks for rendering on different outputs

	// width/height define the logical content size
	// in pixels at 96 DPI baseline. The renderer computes the physical buffer as
	// Width * dpi/96, ensuring DPI-normalized rendering.
	// Zero means content renders at mask size (no DPI normalization).
	width  Expr
	height Expr
}

// NewWidget creates a new widget attached to the given layout.
func NewWidget(layout *Layout) *Widget {
	w := &Widget{
		layout:   layout,
		contents: make([]*Content, 0),
		masks:    make([]*Mask, 0),
		width:    layout.NewVar(),
		height:   layout.NewVar(),
	}
	layout.addWidget(w)
	return w
}

// Width returns the widget's logical width expression.
func (w *Widget) Width() Expr {
	return w.width
}

// Height returns the widget's logical height expression.
func (w *Widget) Height() Expr {
	return w.height
}

// ContentWidth returns the current logical content width.
func (w *Widget) ContentWidth() DesignUnit {
	return DesignUnit(w.width.Get())
}

// ContentHeight returns the current logical content height.
func (w *Widget) ContentHeight() DesignUnit {
	return DesignUnit(w.height.Get())
}

// SetSize updates the widget's logical content size.
func (w *Widget) SetSize(width, height DesignUnit) {
	w.width.Set(float64(width))
	w.height.Set(float64(height))
}

// NewMask creates a mask for a specific output/layer.
// The mask embeds a LayoutItem that positions the entire widget on that output.
func (w *Widget) NewMask(output *Output, config LayerConfig) (*Mask, error) {
	mask := &Mask{
		LayoutItem: w.layout.NewLayoutItem(),
		widget:     w,
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
		left := int32(math.Round(rawLeft))
		top := int32(math.Round(rawTop))

		// Update layer margin (anchor is always top-left)
		if err := mask.layer.SetMargin(top, 0, 0, left); err != nil {
			w.layout.reportError(fmt.Errorf("update mask margin for %q: %w", mask.config.Name, err))
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
			w.layout.reportError(fmt.Errorf("update mask size for %q: %w", mask.config.Name, err))
		}
		// Commit the change
		if err := mask.layer.Surface().Commit(); err != nil {
			w.layout.reportError(fmt.Errorf("commit mask size for %q: %w", mask.config.Name, err))
		}
	}, mask.LayoutItem.Width(), mask.LayoutItem.Height()))

	w.mu.Lock()
	w.masks = append(w.masks, mask)
	w.mu.Unlock()

	return mask, nil
}

func (w *Widget) removeMask(mask *Mask) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, candidate := range w.masks {
		if candidate == mask {
			w.masks = append(w.masks[:i], w.masks[i+1:]...)
			return
		}
	}
}

// Remove removes this widget and all its resources.
func (w *Widget) Remove() error {
	w.mu.Lock()
	contents := append([]*Content(nil), w.contents...)
	masks := append([]*Mask(nil), w.masks...)
	w.contents = nil
	w.masks = nil
	w.mu.Unlock()

	// Clean up content LayoutItems (remove symbols from solver)
	for _, content := range contents {
		content.Cleanup(w.layout)
	}

	// Clean up masks
	for _, mask := range masks {
		if err := mask.Remove(); err != nil {
			return err
		}
	}

	// Remove this widget from layout's widget list
	w.layout.removeWidget(w)

	// Clear slices to allow GC
	// Request re-render
	w.layout.RequestRender()

	return nil
}
