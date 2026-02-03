package buffer

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// CreateAnonymousFile creates a temporary file backed by memory.
// Tries memfd_create first (Linux 3.17+), then falls back to temp file.
func CreateAnonymousFile(size int64) (*os.File, error) {
	// Try memfd_create first (Linux 3.17+)
	fd, err := unix.MemfdCreate("wayland-shm\000", 0)
	if err == nil {
		if err := unix.Ftruncate(fd, size); err != nil {
			unix.Close(fd)
			return nil, fmt.Errorf("ftruncate failed: %w", err)
		}
		return os.NewFile(uintptr(fd), "wayland-shm"), nil
	}

	// Fallback to creating a temp file
	file, err := os.CreateTemp("", "wayland-shm-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file failed: %w", err)
	}

	// Unlink immediately so it's automatically cleaned up
	if err := os.Remove(file.Name()); err != nil {
		file.Close()
		return nil, fmt.Errorf("unlink failed: %w", err)
	}

	if err := file.Truncate(size); err != nil {
		file.Close()
		return nil, fmt.Errorf("truncate failed: %w", err)
	}

	return file, nil
}

// Mmap memory-maps a file descriptor.
func Mmap(fd int, offset int64, size int, prot int, flags int) ([]byte, error) {
	return unix.Mmap(fd, offset, size, prot, flags)
}

// Munmap unmaps a memory region.
func Munmap(data []byte) error {
	return unix.Munmap(data)
}

// Memory protection flags for Mmap
const (
	ProtRead  = syscall.PROT_READ
	ProtWrite = syscall.PROT_WRITE
	MapShared = syscall.MAP_SHARED
)
