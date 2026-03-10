package ui

import (
	"fmt"
	"sync"

	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/shm"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/signals"
)

// Mask represents one frame on one output.
// It has two separate concerns:
//   - LayoutItem: positions the mask within the frame (global/frame-local space)
//   - Clip config: clips the content output (which portion of rendered content to show)
//
// ClipX/ClipY define the visible region origin in widget coordinates (96 DPI units).
// The visible region size is determined by the mask's Width()/Height() constraints.
// For example: ClipX=100, ClipY=50 with mask size 200x300 shows widget[100:300, 50:350].
// By default (zero values), shows widget starting from (0,0).
type Mask struct {
	*LayoutItem                     // Controls layer margin for global positioning (via reactive Effect)
	layer       *shell.LayerSurface // Layer surface reference for reactive margin updates
	frame       *shm.Frame          // Frame for this output
	widget      *Widget             // Parent widget (for accessing contents during render)
	output      *Output
	config      LayerConfig

	// Clip origin in widget coordinates (96 DPI units).
	// The visible region extends from (ClipX, ClipY) to (ClipX + maskWidth, ClipY + maskHeight).
	ClipX DesignUnit // X offset in widget coordinates
	ClipY DesignUnit

	disposers []signals.CancelFunc
	removed   bool
	mu        sync.RWMutex
}

// LayerConfig specifies how to create the layer surface
type LayerConfig struct {
	Layer         LayerPosition
	Name          string
	ExclusiveZone *int32 // nil = 0 (no exclusive zone), use ptr to distinguish 0 from unset
}

// getOrCreateFrame creates frame lazily when mask is first used
func (m *Mask) getOrCreateFrame() (*shm.Frame, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.frame != nil {
		return m.frame, nil
	}
	if m.output == nil {
		return nil, fmt.Errorf("mask has no output")
	}

	disp := m.output.displayHandle()
	if disp == nil {
		return nil, fmt.Errorf("mask output has no display")
	}

	// Create surface
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		return nil, err
	}

	// Create layer - always use AnchorTop|AnchorLeft for reactive margin positioning
	exclZone := int32(0)
	if m.config.ExclusiveZone != nil {
		exclZone = *m.config.ExclusiveZone
	}
	layer, err := shell.NewLayer(surf, disp.LayerShell(), shell.LayerConfig{
		Layer:         m.config.Layer,
		Name:          m.config.Name,
		Anchor:        shell.AnchorTop | shell.AnchorLeft,
		Width:         1, // Initial size - will be updated by constraints
		Height:        1, // Initial size - will be updated by constraints
		ExclusiveZone: exclZone,
		Output:        m.output.wlOutput(),
	})
	if err != nil {
		return nil, err
	}

	// Store layer reference for reactive margin updates
	m.layer = layer

	// Create frame
	frame, err := shm.NewFrame(disp.Shm(), layer, disp, shm.FrameConfig{
		Format:  client.WlShmFormatArgb8888,
		Buffers: 2,
	})
	if err != nil {
		return nil, err
	}

	frame.OnError(func(err error) {
		if m.widget != nil && m.widget.layout != nil {
			m.widget.layout.reportError(fmt.Errorf("mask frame %q: %w", m.config.Name, err))
		}
	})

	frame.SetManualMode(true)
	m.frame = frame

	return frame, nil
}

// Frame returns the frame for this mask, if one has been created.
func (m *Mask) Frame() *shm.Frame {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.frame
}

// SetClip updates the visible origin in widget coordinates and requests a render.
func (m *Mask) SetClip(x, y DesignUnit) {
	m.mu.Lock()
	m.ClipX = x
	m.ClipY = y
	widget := m.widget
	m.mu.Unlock()

	if widget != nil && widget.layout != nil {
		widget.layout.RequestRender()
	}
}

// Remove releases the mask's reactive hooks, owned constraints, solver vars, and Wayland resources.
func (m *Mask) Remove() error {
	m.mu.Lock()
	if m.removed {
		m.mu.Unlock()
		return nil
	}
	m.removed = true
	disposers := m.disposers
	m.disposers = nil
	frame := m.frame
	layer := m.layer
	widget := m.widget
	m.frame = nil
	m.layer = nil
	m.widget = nil
	m.mu.Unlock()

	for _, dispose := range disposers {
		if dispose != nil {
			dispose()
		}
	}

	if widget != nil {
		widget.removeMask(m)
	}

	if m.LayoutItem != nil {
		m.LayoutItem.releaseOwnedConstraints()
		if widget != nil && widget.layout != nil {
			widget.layout.removeLayoutItem(m.LayoutItem)
		}
	}

	if frame != nil && widget != nil && widget.layout != nil {
		widget.layout.RemoveFrame(frame)
		if err := frame.Close(); err != nil {
			return err
		}
	}
	if layer != nil {
		if err := layer.Close(); err != nil {
			return err
		}
		if surf := layer.Surface(); surf != nil {
			if err := surf.Close(); err != nil {
				return err
			}
		}
	}

	if widget != nil && widget.layout != nil {
		widget.layout.RequestRender()
	}

	return nil
}

// Close is an alias for Remove.
func (m *Mask) Close() error {
	return m.Remove()
}
