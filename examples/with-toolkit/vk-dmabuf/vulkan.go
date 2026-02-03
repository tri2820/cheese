package main

// This file contains only the minimal C forward declarations needed
// The actual Vulkan implementation is split across:
// - vulkan_init.go: Initialization and cleanup
// - vulkan_render.go: Rendering functions (clear, copy)
// - vulkan_dmabuf.go: DmaBuf export functionality

/*
#cgo linux CFLAGS: -I/usr/include/vulkan
#cgo linux LDFLAGS: -lvulkan
#include <vulkan/vulkan.h>

// Forward declarations for C types used in Go
typedef struct {
    VkMemoryPropertyFlags propertyFlags;
    uint32_t heapIndex;
} VkMemoryType_;

typedef struct {
    VkDeviceSize size;
    VkMemoryHeapFlags flags;
} VkMemoryHeap_;
*/
import "C"

// DmaBufImage holds the dmabuf-exported Vulkan image
type DmaBufImage struct {
	image    C.VkImage
	memory   C.VkDeviceMemory
	memoryFD int
	width    int
	height   int
	stride   int
	format   uint32 // DRM fourcc
}
