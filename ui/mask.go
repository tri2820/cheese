package ui

import (
	"log"
	"sync"

	"github.com/tri2820/cheese/client-toolkit/buffer"
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
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
	frame       *buffer.Frame       // Frame for this output
	widget      *Widget             // Parent widget (for accessing contents during render)
	display     *display.Display    // For frame creation
	output      *client.WlOutput
	config      LayerConfig

	// Clip origin in widget coordinates (96 DPI units).
	// The visible region extends from (ClipX, ClipY) to (ClipX + maskWidth, ClipY + maskHeight).
	ClipX float64 // X offset in widget coordinates
	ClipY float64 // Y offset in widget coordinates

	mu sync.RWMutex
}

// LayerConfig specifies how to create the layer surface
type LayerConfig struct {
	Layer         shell.LayerPosition
	Name          string
	Anchor        shell.LayerAnchor
	ExclusiveZone *int32 // nil = 0 (no exclusive zone), use ptr to distinguish 0 from unset
}

// getOrCreateFrame creates frame lazily when mask is first used
func (m *Mask) getOrCreateFrame() (*buffer.Frame, error) {
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
	frame, err := buffer.NewFrame(m.display.Shm(), layer, m.display, buffer.FrameConfig{
		Format:  client.WlShmFormatArgb8888,
		Buffers: 2,
	})
	if err != nil {
		return nil, err
	}

	frame.SetManualMode(true)
	m.frame = frame

	log.Printf("Created frame for mask on output %v", m.output)

	return frame, nil
}

// Frame returns the frame for this mask (creates if needed)
func (m *Mask) Frame() *buffer.Frame {
	m.mu.RLock()
	if m.frame != nil {
		m.mu.RUnlock()
		return m.frame
	}
	m.mu.RUnlock()

	// Create frame if needed
	frame, err := m.getOrCreateFrame()
	if err != nil {
		log.Printf("Failed to create frame: %v", err)
		return nil
	}
	return frame
}
