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

// renderToDmaBuf creates a rendered image and exports it as a dmabuf file descriptor
func renderToDmaBuf(width, height int) (fd int, stride int, err error) {
	if !globalVulkan.initialized {
		if !initVulkan() {
			return -1, 0, os.ErrNotExist
		}
	}

	device := globalVulkan.device

	// Create render target image with optimal tiling for rendering
	renderImageCreateInfo := vulkan.ImageCreateInfo{
		SType:          vulkan.StructureTypeImageCreateInfo,
		ImageType:      vulkan.ImageType2d,
		Extent:         vulkan.Extent3D{Width: uint32(width), Height: uint32(height), Depth: 1},
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
		return -1, 0, err
	}
	defer vulkan.DestroyImage(device, renderImage, nil)

	// Allocate memory for render image
	var renderMemReqs C.VkMemoryRequirements
	C.vkGetImageMemoryRequirements(C.VkDevice(unsafe.Pointer(device)), C.VkImage(unsafe.Pointer(renderImage)), &renderMemReqs)

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
		return -1, 0, err
	}
	defer vulkan.FreeMemory(device, renderMemory, nil)

	result = vulkan.BindImageMemory(device, renderImage, renderMemory, 0)
	if result != vulkan.Success {
		log.Printf("Failed to bind render image memory: %v", result)
		return -1, 0, err
	}

	// Render triangle to the optimal image (validation layers enabled for debugging)
	renderTriangleToImage(device, renderImage, width, height)

	// Create dmabuf-exportable linear image
	explicitInfo := vulkan.ExternalMemoryImageCreateInfo{
		SType:       vulkan.StructureTypeExternalMemoryImageCreateInfo,
		HandleTypes: 0x00000200, // VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT
	}

	dmabufImageCreateInfo := vulkan.ImageCreateInfo{
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

	var dmabufImage vulkan.Image
	result = vulkan.CreateImage(device, &dmabufImageCreateInfo, nil, &dmabufImage)
	if result != vulkan.Success {
		log.Printf("Failed to create dmabuf image: %v", result)
		return -1, 0, err
	}

	// Get memory requirements for dmabuf image
	var memReqsC C.VkMemoryRequirements
	C.vkGetImageMemoryRequirements(C.VkDevice(unsafe.Pointer(device)), C.VkImage(unsafe.Pointer(dmabufImage)), &memReqsC)
	memTypeBits := uint32(memReqsC.memoryTypeBits)
	log.Printf("DmaBuf memory requirements: size=%d, alignment=%d, bits=0x%08x", memReqsC.size, memReqsC.alignment, memTypeBits)

	memTypeIndex := uint32(^uint32(0))
	for i := uint32(0); i < 32; i++ {
		if memTypeBits&(1<<i) != 0 {
			memTypeIndex = i
			log.Printf("Using memory type index: %d", i)
			break
		}
	}

	if memTypeIndex == ^uint32(0) {
		log.Println("Failed to find suitable memory type")
		vulkan.DestroyImage(device, dmabufImage, nil)
		return -1, 0, os.ErrNotExist
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
		log.Printf("Failed to allocate dmabuf memory: %v", result)
		vulkan.DestroyImage(device, dmabufImage, nil)
		return -1, 0, err
	}

	result = vulkan.BindImageMemory(device, dmabufImage, memory, 0)
	if result != vulkan.Success {
		log.Printf("Failed to bind dmabuf image memory: %v", result)
		vulkan.FreeMemory(device, memory, nil)
		vulkan.DestroyImage(device, dmabufImage, nil)
		return -1, 0, err
	}

	// Copy from render image to dmabuf image
	copyImageToImage(device, renderImage, dmabufImage, width, height)

	// Get the dmabuf file descriptor
	dmabufFD := getMemoryFdKHR(device, memory)

	// Get the actual stride from the image subresource layout
	var subresourceLayout C.VkSubresourceLayout
	var imageSubresource C.VkImageSubresource
	imageSubresource.aspectMask = C.VkImageAspectFlags(vulkan.ImageAspectColorBit)
	imageSubresource.mipLevel = 0
	imageSubresource.arrayLayer = 0

	C.vkGetImageSubresourceLayout(
		C.VkDevice(unsafe.Pointer(device)),
		C.VkImage(unsafe.Pointer(dmabufImage)),
		&imageSubresource,
		&subresourceLayout,
	)

	stride = int(subresourceLayout.rowPitch)
	log.Printf("Exported dmabuf: fd=%d size=%d stride=%d (offset=%d)", dmabufFD, memReqsC.size, stride, subresourceLayout.offset)

	return dmabufFD, stride, nil
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
