package shm

import (
	"errors"
	"fmt"

	"github.com/tri2820/cheese/client-toolkit/buffer"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
)

var (
	// ErrBufferAlreadyAcquired indicates Acquire was called twice without Present.
	ErrBufferAlreadyAcquired = errors.New("buffer already acquired")

	// ErrNoFreeBuffer indicates all swapchain buffers are still busy.
	ErrNoFreeBuffer = errors.New("no free buffer")
)

// Swapchain handles multi-buffered SHM rendering for one surface.
type Swapchain struct {
	pool     *buffer.Pool
	surface  *surface.Surface
	width    int
	height   int
	format   client.WlShmFormat
	stride   int
	acquired *buffer.Slot
}

// SwapchainConfig configures a new Swapchain.
type SwapchainConfig struct {
	// Shm is the wl_shm global.
	Shm *client.WlShm

	// Buffers is the number of buffers to allocate.
	Buffers int

	// Width and height of the buffers.
	Width  int
	Height int

	// Format is the pixel format.
	Format client.WlShmFormat
}

// NewSwapchain creates a new SHM swapchain.
func NewSwapchain(config SwapchainConfig) (*Swapchain, error) {
	if config.Buffers < 1 {
		return nil, fmt.Errorf("at least 1 buffer required")
	}

	stride := config.Width * bytesPerPixel(config.Format)
	slotSize := stride * config.Height
	alignedSlotSize := (slotSize + 63) &^ 63
	poolSize := alignedSlotSize * config.Buffers

	pool, err := buffer.NewPool(config.Shm, buffer.PoolConfig{
		Width:  config.Width,
		Height: config.Height * config.Buffers,
		Format: config.Format,
		Size:   poolSize,
	})
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	sc := &Swapchain{
		pool:   pool,
		width:  config.Width,
		height: config.Height,
		format: config.Format,
		stride: stride,
	}

	for i := 0; i < config.Buffers; i++ {
		if _, err := pool.NewSlot(slotSize); err != nil {
			pool.Close()
			return nil, fmt.Errorf("create slot %d: %w", i, err)
		}
	}

	return sc, nil
}

// SetSurface attaches the swapchain to a surface.
func (sc *Swapchain) SetSurface(surf *surface.Surface) {
	sc.surface = surf
}

// Surface returns the attached surface.
func (sc *Swapchain) Surface() *surface.Surface {
	return sc.surface
}

// Acquire returns a buffer ready for drawing.
func (sc *Swapchain) Acquire() ([]byte, error) {
	if sc.acquired != nil {
		return nil, fmt.Errorf("%w (call Present first)", ErrBufferAlreadyAcquired)
	}

	slot := sc.pool.FindFree()
	if slot == nil {
		return nil, ErrNoFreeBuffer
	}

	if slot.Buffer() == nil {
		if _, err := slot.NewBuffer(sc.width, sc.height, sc.stride, sc.format); err != nil {
			return nil, fmt.Errorf("create buffer: %w", err)
		}
	}

	sc.acquired = slot
	return slot.Mmap(), nil
}

// Present submits the last acquired buffer to the surface.
func (sc *Swapchain) Present() error {
	if sc.acquired == nil {
		return fmt.Errorf("no buffer acquired (call Acquire first)")
	}

	buf := sc.acquired.Buffer()

	if sc.surface != nil {
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

	sc.acquired.Mark()
	sc.acquired = nil

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
func (sc *Swapchain) Format() client.WlShmFormat {
	return sc.format
}

func bytesPerPixel(format client.WlShmFormat) int {
	switch format {
	case client.WlShmFormatXrgb8888, client.WlShmFormatArgb8888, client.WlShmFormatXbgr8888,
		client.WlShmFormatAbgr8888, client.WlShmFormatRgbx8888, client.WlShmFormatRgba8888,
		client.WlShmFormatBgrx8888, client.WlShmFormatBgra8888:
		return 4
	case client.WlShmFormatRgb888, client.WlShmFormatBgr888:
		return 3
	case client.WlShmFormatRgb565, client.WlShmFormatBgr565:
		return 2
	default:
		return 4
	}
}
