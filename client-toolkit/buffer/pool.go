package buffer

import (
	"fmt"
	"os"

	"github.com/tri2820/cheese/protocols/client"
)

// Config configures a new SHM pool.
type Config struct {
	Width  int
	Height int
	Format client.WlShmFormat
	Size   int // 0 = auto-calculate from Width, Height, Format
}

// Pool is a Wayland SHM memory pool.
//
// The pool manages a shared memory region that can be divided into slots.
// Slots can be used to create wl_buffer objects.
type Pool struct {
	wlPool  *client.WlShmPool
	memFile *os.File
	data    []byte     // memory-mapped region
	size    int
	slots   []*Slot
}

// NewPool creates a new SHM pool.
func NewPool(shm *client.WlShm, config Config) (*Pool, error) {
	width := config.Width
	height := config.Height
	format := config.Format
	size := config.Size

	if size == 0 {
		size = width * height * bytesPerPixel(format)
	}

	// Create anonymous file
	memFile, err := CreateAnonymousFile(int64(size))
	if err != nil {
		return nil, fmt.Errorf("create anonymous file: %w", err)
	}

	// Create wl_shm_pool
	pool, err := shm.CreatePool(memFile.Fd(), int32(size))
	if err != nil {
		memFile.Close()
		return nil, fmt.Errorf("create shm pool: %w", err)
	}

	// Memory-map the file
	data, err := Mmap(int(memFile.Fd()), 0, size, ProtRead|ProtWrite, MapShared)
	if err != nil {
		pool.Destroy()
		memFile.Close()
		return nil, fmt.Errorf("mmap: %w", err)
	}

	return &Pool{
		wlPool:  pool,
		memFile: memFile,
		data:    data,
		size:    size,
		slots:   nil,
	}, nil
}

// Close destroys the pool and frees all resources.
func (p *Pool) Close() error {
	if p.data != nil {
		Munmap(p.data)
		p.data = nil
	}
	if p.wlPool != nil {
		if err := p.wlPool.Destroy(); err != nil {
			return err
		}
	}
	if p.memFile != nil {
		return p.memFile.Close()
	}
	return nil
}

// Slot is a region of the pool that can hold one buffer's worth of data.
type Slot struct {
	pool   *Pool
	offset int
	length int
	busy   bool
	buffer *Buffer
}

// NewSlot allocates a new slot in the pool.
// The size is rounded up to 64 bytes for alignment.
func (p *Pool) NewSlot(size int) (*Slot, error) {
	// Round up to 64-byte alignment
	aligned := (size + 63) &^ 63

	// Find free space at the end of existing slots
	offset := 0
	for _, slot := range p.slots {
		if slot != nil {
			end := slot.offset + slot.length
			if end > offset {
				offset = end
			}
		}
	}

	// Check if we need to resize
	if offset+aligned > p.size {
		return nil, fmt.Errorf("pool too small: need %d, have %d", offset+aligned, p.size)
	}

	slot := &Slot{
		pool:   p,
		offset: offset,
		length: aligned,
		busy:   false,
		buffer: nil,
	}

	p.slots = append(p.slots, slot)
	return slot, nil
}

// Mmap returns the memory region for this slot.
func (s *Slot) Mmap() []byte {
	start := s.offset
	end := s.offset + s.length
	return s.pool.data[start:end]
}

// Busy returns whether the slot is currently in use by the compositor.
func (s *Slot) Busy() bool {
	return s.busy
}

// Mark marks the slot as busy (attached to a surface).
func (s *Slot) Mark() {
	s.busy = true
}

// Release marks the slot as free (released by the compositor).
func (s *Slot) Release() {
	s.busy = false
}

// Buffer returns the buffer created from this slot, or nil if not created.
func (s *Slot) Buffer() *Buffer {
	return s.buffer
}

// FindFree returns the first non-busy slot, or nil if all are busy.
func (p *Pool) FindFree() *Slot {
	for _, slot := range p.slots {
		if slot != nil && !slot.busy {
			return slot
		}
	}
	return nil
}

// Buffer is a Wayland buffer created from a slot.
type Buffer struct {
	wlBuffer *client.WlBuffer
	slot     *Slot
	height   int
	stride   int
}

// NewBuffer creates a new wl_buffer from the slot.
func (s *Slot) NewBuffer(width, height, stride int, format client.WlShmFormat) (*Buffer, error) {
	if s.busy {
		return nil, fmt.Errorf("slot is busy")
	}

	if s.length < height*stride {
		return nil, fmt.Errorf("slot too small: need %d, have %d", height*stride, s.length)
	}

	buffer, err := s.pool.wlPool.CreateBuffer(
		int32(s.offset), int32(width), int32(height), int32(stride), format,
	)
	if err != nil {
		return nil, fmt.Errorf("create buffer: %w", err)
	}

	// Set release handler to mark slot as free when compositor is done
	slot := s
	buffer.SetReleaseHandler(func(client.WlBufferReleaseEvent) {
		slot.Release()
	})

	s.buffer = &Buffer{
		wlBuffer: buffer,
		slot:     s,
		height:   height,
		stride:   stride,
	}

	return s.buffer, nil
}

// bytesPerPixel returns the number of bytes per pixel for a Wayland format.
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
		return 4 // fallback
	}
}

// WlBuffer returns the underlying wl_buffer.
func (b *Buffer) WlBuffer() *client.WlBuffer {
	return b.wlBuffer
}

// Slot returns the slot this buffer was created from.
func (b *Buffer) Slot() *Slot {
	return b.slot
}

// Height returns the buffer height in pixels.
func (b *Buffer) Height() int {
	return b.height
}

// Stride returns the buffer stride in bytes.
func (b *Buffer) Stride() int {
	return b.stride
}

// Destroy destroys the wl_buffer.
func (b *Buffer) Destroy() error {
	return b.wlBuffer.Destroy()
}
