package main

import (
	"log"
	"os"
	"unsafe"

	vulkan "github.com/vulkan-go/vulkan"
)

/*
#cgo linux CFLAGS: -I/usr/include/vulkan
#cgo linux LDFLAGS: -lvulkan
#include <vulkan/vulkan.h>
#include <stdlib.h>

// C helper to call the vkGetMemoryFdKHR function pointer
VkResult call_vkGetMemoryFdKHR(
    PFN_vkGetMemoryFdKHR fptr,
    VkDevice device,
    const VkMemoryGetFdInfoKHR* pGetFdInfo,
    int* pFd)
{
    return fptr(device, pGetFdInfo, pFd);
}

// C helper to call vkGetDeviceProcAddr
PFN_vkVoidFunction call_vkGetDeviceProcAddr(
    VkDevice device,
    const char* pName)
{
    return vkGetDeviceProcAddr(device, pName);
}

// C helper to get image memory requirements
typedef struct {
    VkDeviceSize size;
    VkDeviceSize alignment;
    uint32_t memoryTypeBits;
} VkMemoryRequirementsGo;

void getImageMemoryRequirements(VkDevice device, VkImage image, VkMemoryRequirementsGo* reqs) {
    VkMemoryRequirements cReqs;
    vkGetImageMemoryRequirements(device, image, &cReqs);
    reqs->size = cReqs.size;
    reqs->alignment = cReqs.alignment;
    reqs->memoryTypeBits = cReqs.memoryTypeBits;
}
*/
import "C"

// DmaBufBuffer holds a dmabuf-exportable Vulkan image and its resources
type DmaBufBuffer struct {
	image   vulkan.Image
	memory  vulkan.DeviceMemory
	fd      int
	stride  int
	busy    bool
	width   int
	height  int
}

var dmabufBuffers [2]DmaBufBuffer

// initDmaBufBuffers creates 2 dmabuf-exportable images upfront for double buffering
func initDmaBufBuffers(width, height int) error {
	if !globalVulkan.initialized {
		if !initVulkan() {
			return os.ErrNotExist
		}
	}

	for i := 0; i < 2; i++ {
		image, memory, fd, stride, err := createDmaBufImage(width, height)
		if err != nil {
			log.Printf("Failed to create dmabuf buffer %d: %v", i, err)
			return err
		}
		dmabufBuffers[i] = DmaBufBuffer{
			image:  image,
			memory: memory,
			fd:     fd,
			stride: stride,
			busy:   false,
			width:  width,
			height: height,
		}
		log.Printf("Created dmabuf buffer %d: fd=%d stride=%d", i, fd, stride)
	}

	return nil
}

// createDmaBufImage creates a single dmabuf-exportable Vulkan image
func createDmaBufImage(width, height int) (vulkan.Image, vulkan.DeviceMemory, int, int, error) {
	device := globalVulkan.device

	// Create dmabuf-exportable linear image
	explicitInfo := vulkan.ExternalMemoryImageCreateInfo{
		SType:       vulkan.StructureTypeExternalMemoryImageCreateInfo,
		HandleTypes: 0x00000200, // VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT
	}

	imageCreateInfo := vulkan.ImageCreateInfo{
		SType:          vulkan.StructureTypeImageCreateInfo,
		PNext:          unsafe.Pointer(&explicitInfo),
		ImageType:      vulkan.ImageType2d,
		Extent:         vulkan.Extent3D{Width: uint32(width), Height: uint32(height), Depth: 1},
		MipLevels:      1,
		ArrayLayers:    1,
		Format:         vulkan.FormatR8g8b8a8Unorm,
		Tiling:         vulkan.ImageTilingLinear,
		InitialLayout:  vulkan.ImageLayoutUndefined,
		Usage:          vulkan.ImageUsageFlags(vulkan.ImageUsageTransferDstBit),
		Samples:        vulkan.SampleCount1Bit,
		SharingMode:    vulkan.SharingModeExclusive,
	}

	var image vulkan.Image
	result := vulkan.CreateImage(device, &imageCreateInfo, nil, &image)
	if result != vulkan.Success {
		log.Printf("Failed to create dmabuf image: %v", result)
		return nil, nil, 0, 0, os.ErrNotExist
	}

	// Get memory requirements
	var memReqsC C.VkMemoryRequirementsGo
	C.getImageMemoryRequirements(C.VkDevice(unsafe.Pointer(device)), C.VkImage(unsafe.Pointer(image)), &memReqsC)
	memTypeBits := uint32(memReqsC.memoryTypeBits)

	memTypeIndex := uint32(^uint32(0))
	for i := uint32(0); i < 32; i++ {
		if memTypeBits&(1<<i) != 0 {
			memTypeIndex = i
			break
		}
	}

	if memTypeIndex == ^uint32(0) {
		vulkan.DestroyImage(device, image, nil)
		log.Printf("Failed to find suitable memory type")
		return nil, nil, 0, 0, os.ErrNotExist
	}

	// Allocate memory with export info
	allocInfo := vulkan.MemoryAllocateInfo{
		SType:           vulkan.StructureTypeMemoryAllocateInfo,
		AllocationSize:  vulkan.DeviceSize(memReqsC.size),
		MemoryTypeIndex: memTypeIndex,
	}

	exportAllocInfo := vulkan.ExportMemoryAllocateInfo{
		SType:       vulkan.StructureTypeExportMemoryAllocateInfo,
		HandleTypes: 0x00000200, // VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT
	}
	allocInfo.PNext = unsafe.Pointer(&exportAllocInfo)

	var memory vulkan.DeviceMemory
	result = vulkan.AllocateMemory(device, &allocInfo, nil, &memory)
	if result != vulkan.Success {
		vulkan.DestroyImage(device, image, nil)
		log.Printf("Failed to allocate dmabuf memory: %v", result)
		return nil, nil, 0, 0, os.ErrNotExist
	}

	result = vulkan.BindImageMemory(device, image, memory, 0)
	if result != vulkan.Success {
		vulkan.FreeMemory(device, memory, nil)
		vulkan.DestroyImage(device, image, nil)
		log.Printf("Failed to bind dmabuf image memory: %v", result)
		return nil, nil, 0, 0, os.ErrNotExist
	}

	// Export as dmabuf fd
	fd := getMemoryFdKHR(device, memory)

	// Get stride
	var subresourceLayout C.VkSubresourceLayout
	var imageSubresource C.VkImageSubresource
	imageSubresource.aspectMask = C.VkImageAspectFlags(vulkan.ImageAspectColorBit)
	imageSubresource.mipLevel = 0
	imageSubresource.arrayLayer = 0

	C.vkGetImageSubresourceLayout(
		C.VkDevice(unsafe.Pointer(device)),
		C.VkImage(unsafe.Pointer(image)),
		&imageSubresource,
		&subresourceLayout,
	)

	stride := int(subresourceLayout.rowPitch)
	return image, memory, fd, stride, nil
}

// renderToDmaBuf renders to a specific dmabuf buffer
func renderToDmaBuf(bufferIndex int, time float32) error {
	buf := &dmabufBuffers[bufferIndex]
	device := globalVulkan.device

	// Create temporary render target image with optimal tiling for rendering
	renderImageCreateInfo := vulkan.ImageCreateInfo{
		SType:          vulkan.StructureTypeImageCreateInfo,
		ImageType:      vulkan.ImageType2d,
		Extent:         vulkan.Extent3D{Width: uint32(buf.width), Height: uint32(buf.height), Depth: 1},
		MipLevels:      1,
		ArrayLayers:    1,
		Format:         vulkan.FormatR8g8b8a8Unorm,
		Tiling:         vulkan.ImageTilingOptimal,
		InitialLayout:  vulkan.ImageLayoutUndefined,
		Usage:          vulkan.ImageUsageFlags(vulkan.ImageUsageColorAttachmentBit | vulkan.ImageUsageTransferSrcBit),
		Samples:        vulkan.SampleCount1Bit,
		SharingMode:    vulkan.SharingModeExclusive,
	}

	var renderImage vulkan.Image
	result := vulkan.CreateImage(device, &renderImageCreateInfo, nil, &renderImage)
	if result != vulkan.Success {
		log.Printf("Failed to create render image: %v", result)
		return os.ErrNotExist
	}
	defer vulkan.DestroyImage(device, renderImage, nil)

	// Allocate memory for render image
	var renderMemReqs C.VkMemoryRequirementsGo
	C.getImageMemoryRequirements(C.VkDevice(unsafe.Pointer(device)), C.VkImage(unsafe.Pointer(renderImage)), &renderMemReqs)

	renderMemTypeIndex := uint32(0)
	for i := uint32(0); i < 32; i++ {
		if uint32(renderMemReqs.memoryTypeBits)&(1<<i) != 0 {
			renderMemTypeIndex = i
			break
		}
	}

	renderAllocInfo := vulkan.MemoryAllocateInfo{
		SType:           vulkan.StructureTypeMemoryAllocateInfo,
		AllocationSize:  vulkan.DeviceSize(renderMemReqs.size),
		MemoryTypeIndex: renderMemTypeIndex,
	}

	var renderMemory vulkan.DeviceMemory
	result = vulkan.AllocateMemory(device, &renderAllocInfo, nil, &renderMemory)
	if result != vulkan.Success {
		log.Printf("Failed to allocate render memory: %v", result)
		return os.ErrNotExist
	}
	defer vulkan.FreeMemory(device, renderMemory, nil)

	result = vulkan.BindImageMemory(device, renderImage, renderMemory, 0)
	if result != vulkan.Success {
		log.Printf("Failed to bind render image memory: %v", result)
		return os.ErrNotExist
	}

	// Render triangle to the optimal image
	renderTriangleToImage(device, renderImage, buf.width, buf.height, time)

	// Copy from render image to dmabuf image
	copyImageToImage(device, renderImage, buf.image, buf.width, buf.height)

	return nil
}

// cleanupDmaBufBuffers cleans up the dmabuf buffers
func cleanupDmaBufBuffers() {
	for i := 0; i < 2; i++ {
		if dmabufBuffers[i].memory != nil {
			vulkan.FreeMemory(globalVulkan.device, dmabufBuffers[i].memory, nil)
		}
		if dmabufBuffers[i].image != nil {
			vulkan.DestroyImage(globalVulkan.device, dmabufBuffers[i].image, nil)
		}
	}
}

// getMemoryFdKHR exports Vulkan memory as a dmabuf file descriptor
func getMemoryFdKHR(device vulkan.Device, memory vulkan.DeviceMemory) int {
	// 1. Get the function pointer via vkGetDeviceProcAddr
	funcName := C.CString("vkGetMemoryFdKHR")
	defer C.free(unsafe.Pointer(funcName))

	procAddr := C.call_vkGetDeviceProcAddr(
		C.VkDevice(unsafe.Pointer(device)),
		funcName,
	)
	if procAddr == nil {
		log.Panic("Failed to get vkGetMemoryFdKHR proc address")
	}

	// 2. Prepare the info struct
	var getFDInfo C.VkMemoryGetFdInfoKHR
	getFDInfo.sType = C.VkStructureType(vulkan.StructureTypeMemoryGetFdInfo)
	getFDInfo.pNext = nil
	getFDInfo.memory = C.VkDeviceMemory(unsafe.Pointer(memory))
	getFDInfo.handleType = C.VkExternalMemoryHandleTypeFlagBits(0x00000200) // VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT

	// 3. Call the function through our C helper
	var fd C.int
	result := C.call_vkGetMemoryFdKHR(
		(C.PFN_vkGetMemoryFdKHR)(procAddr),
		C.VkDevice(unsafe.Pointer(device)),
		&getFDInfo,
		&fd,
	)

	if result != C.VK_SUCCESS {
		log.Panicf("vkGetMemoryFdKHR failed: %d", result)
	}

	return int(fd)
}
