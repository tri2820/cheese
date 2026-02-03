package buffer

import (
	"fmt"

	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/toolkit/surface"
)

// Swapchain handles double-buffered (or multi-buffered) rendering.
// It manages a pool of buffers and tracks which ones are busy.
//
// Frame callbacks should be managed separately (e.g., by the Renderer).
type Swapchain struct {
	pool    *Pool
	surface *surface.Surface
	width   int
	height  int
	format  Format
	stride  int
}

// Config configures a new Swapchain.
type SwapchainConfig struct {
	// Shm is the wl_shm global
	Shm *client.WlShm

	// Buffers is the number of buffers to allocate (typically 2 for double buffering)
	Buffers int

	// Width and height of the buffers
	Width  int
	Height int

	// Format is the pixel format
	Format Format
}

// NewSwapchain creates a new swapchain.
func NewSwapchain(config SwapchainConfig) (*Swapchain, error) {
	if config.Buffers < 1 {
		return nil, fmt.Errorf("at least 1 buffer required")
	}

	stride := config.Width * config.Format.BytesPerPixel()
	slotSize := stride * config.Height
	poolSize := slotSize * config.Buffers

	pool, err := NewPool(config.Shm, Config{
		Width:  config.Width,
		Height: config.Height * config.Buffers,
		Format: config.Format,
		Size:   poolSize,
	})
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	sc := &Swapchain{
		pool:    pool,
		width:   config.Width,
		height:  config.Height,
		format:  config.Format,
		stride:  stride,
	}

	// Pre-allocate slots
	for i := 0; i < config.Buffers; i++ {
		_, err := pool.NewSlot(slotSize)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("create slot %d: %w", i, err)
		}
	}

	return sc, nil
}

// SetSurface attaches the swapchain to a surface.
// After this, Present() will automatically attach/damage/commit.
func (sc *Swapchain) SetSurface(surf *surface.Surface) {
	sc.surface = surf
}

// Acquire returns a buffer ready for drawing.
// It returns the pixel data slice for drawing.
// Call Present() when done drawing to submit the frame.
func (sc *Swapchain) Acquire() ([]byte, error) {
	slot := sc.pool.FindFree()
	if slot == nil {
		return nil, fmt.Errorf("no free buffer")
	}

	// Create buffer if needed
	if slot.Buffer() == nil {
		_, err := slot.NewBuffer(sc.width, sc.height, sc.stride, sc.format)
		if err != nil {
			return nil, fmt.Errorf("create buffer: %w", err)
		}
	}

	return slot.Mmap(), nil
}

// Present submits the last acquired buffer to the surface.
// If a surface is attached, it handles attach/damage/commit automatically.
func (sc *Swapchain) Present() error {
	slot := sc.pool.FindFree()
	if slot == nil {
		return fmt.Errorf("no free buffer")
	}

	// Find the busy slot (the one we just drew to)
	// We need to find the slot that has a buffer but isn't free
	var drawnSlot *Slot
	for _, s := range sc.pool.slots {
		if s != nil && s.Buffer() != nil && !s.Busy() {
			// This is the slot we just acquired
			drawnSlot = s
			break
		}
	}

	if drawnSlot == nil {
		return fmt.Errorf("no acquired buffer to present")
	}

	buf := drawnSlot.Buffer()

	if sc.surface != nil {
		// Attach, damage, and commit
		if err := sc.surface.Attach(buf.WlBuffer(), 0, 0); err != nil {
			return err
		}
		if err := sc.surface.Damage(0, 0, int32(sc.width), int32(sc.height)); err != nil {
			return err
		}
		if err := sc.surface.Commit(); err != nil {
			return err
		}
	}

	// Mark slot as busy
	drawnSlot.Mark()

	return nil
}

// Close destroys the swapchain and frees resources.
func (sc *Swapchain) Close() error {
	return sc.pool.Close()
}

// Width returns the width of the swapchain buffers.
func (sc *Swapchain) Width() int {
	return sc.width
}

// Height returns the height of the swapchain buffers.
func (sc *Swapchain) Height() int {
	return sc.height
}

// Stride returns the stride of the swapchain buffers.
func (sc *Swapchain) Stride() int {
	return sc.stride
}

// Format returns the pixel format of the swapchain.
func (sc *Swapchain) Format() Format {
	return sc.format
}
