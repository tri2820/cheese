package ui

import (
	"log"
	"sync"

	"github.com/tri2820/cheese/client-toolkit/display"
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
	display     *display.Display    // For frame creation
	output      *client.WlOutput
	config      LayerConfig

	// Clip origin in widget coordinates (96 DPI units).
	// The visible region extends from (ClipX, ClipY) to (ClipX + maskWidth, ClipY + maskHeight).
	ClipX float64 // X offset in widget coordinates
	ClipY float64 // Y offset in widget coordinates

	disposers []signals.CancelFunc
	mu        sync.RWMutex
}

// LayerConfig specifies how to create the layer surface
type LayerConfig struct {
	Layer         shell.LayerPosition
	Name          string
	Anchor        shell.LayerAnchor
	ExclusiveZone *int32 // nil = 0 (no exclusive zone), use ptr to distinguish 0 from unset
}

// getOrCreateFrame creates frame lazily when mask is first used
func (m *Mask) getOrCreateFrame() (*shm.Frame, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.frame != nil {
		return m.frame, nil
	}

	// Create surface
	surf, err := surface.New(m.display.Compositor())
	if err != nil {
		return nil, err
	}

	// Create layer - always use AnchorTop|AnchorLeft for reactive margin positioning
	exclZone := int32(0)
	if m.config.ExclusiveZone != nil {
		exclZone = *m.config.ExclusiveZone
	}
	layer, err := shell.NewLayer(surf, m.display.LayerShell(), shell.LayerConfig{
		Layer:         m.config.Layer,
		Name:          m.config.Name,
		Anchor:        shell.AnchorTop | shell.AnchorLeft,
		Width:         1, // Initial size - will be updated by constraints
		Height:        1, // Initial size - will be updated by constraints
		ExclusiveZone: exclZone,
		Output:        m.output,
	})
	if err != nil {
		return nil, err
	}

	// Store layer reference for reactive margin updates
	m.layer = layer

	// Create frame
	frame, err := shm.NewFrame(m.display.Shm(), layer, m.display, shm.FrameConfig{
		Format:  client.WlShmFormatArgb8888,
		Buffers: 2,
	})
	if err != nil {
		return nil, err
	}

	frame.OnError(func(err error) {
		log.Printf("mask frame error: %v", err)
	})

	frame.SetManualMode(true)
	m.frame = frame

	log.Printf("Created frame for mask on output %v", m.output)

	return frame, nil
}

// Frame returns the frame for this mask, if one has been created.
func (m *Mask) Frame() *shm.Frame {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.frame
}

// Close releases the mask's reactive hooks and Wayland resources.
func (m *Mask) Close() error {
	m.mu.Lock()
	disposers := m.disposers
	m.disposers = nil
	frame := m.frame
	layer := m.layer
	m.frame = nil
	m.layer = nil
	m.mu.Unlock()

	for _, dispose := range disposers {
		if dispose != nil {
			dispose()
		}
	}

	if frame != nil && m.widget != nil && m.widget.layout != nil {
		m.widget.layout.RemoveFrame(frame)
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

	return nil
}
