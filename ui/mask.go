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
// ClipRelX/ClipRelY (0.0 to 1.0) select the start position as a fraction of content.
// ClipRelW/ClipRelH (0.0 to 1.0) select the visible portion size as a fraction of content.
//
// When Widget.ContentWidth/ContentHeight are set, content buffer size is computed as
// ContentWidth * dpi/96 (DPI-normalized), ensuring ClipRel fractions refer to the
// same logical position across outputs with different DPIs.
// When not set, falls back to visW/ClipRelW (only correct for single-DPI setups).
// By default (zero values), no clipping - content renders at mask's visible size.
type Mask struct {
	*LayoutItem                // Controls layer margin for global positioning (via reactive Effect)
	layer   *shell.LayerSurface // Layer surface reference for reactive margin updates
	frame   *buffer.Frame       // Frame for this output
	widget  *Widget             // Parent widget (for accessing contents during render)
	display *display.Display    // For frame creation
	output  *client.WlOutput
	config  LayerConfig

	// Clip config - separate from LayoutItem positioning.
	// When Widget.ContentWidth is set, content buffer = ContentWidth * dpi/96.
	// ClipRelX selects the start position, ClipRelW selects the visible fraction.
	// Zero/default values = no clipping, content renders at visible size directly.
	ClipRelX float64 // X offset as fraction of content (0.0 to 1.0)
	ClipRelY float64 // Y offset as fraction of content (0.0 to 1.0)
	ClipRelW float64 // Visible width as fraction of content (0.0 to 1.0, 0 = full width)
	ClipRelH float64 // Visible height as fraction of content (0.0 to 1.0, 0 = full height)

	mu sync.RWMutex
}

// LayerConfig specifies how to create the layer surface
type LayerConfig struct {
	Layer         shell.LayerPosition
	Name          string
	Anchor        shell.LayerAnchor
	Width         uint32
	Height        uint32
	ExclusiveZone int32
	Margin        struct{ Top, Right, Bottom, Left int32 }
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
	layer, err := shell.NewLayer(surf, m.display.LayerShell(), shell.LayerConfig{
		Layer:         m.config.Layer,
		Name:          m.config.Name,
		Anchor:        shell.AnchorTop | shell.AnchorLeft,
		Width:         m.config.Width,
		Height:        m.config.Height,
		ExclusiveZone: m.config.ExclusiveZone,
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
