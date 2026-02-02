package main

/*
#cgo linux CFLAGS: -I/usr/include/vulkan
#cgo linux LDFLAGS: -lvulkan
#include <vulkan/vulkan.h>
#include <stdlib.h>
#include <string.h>

// Forward declarations for C types used in Go
typedef struct {
    VkMemoryPropertyFlags propertyFlags;
    uint32_t heapIndex;
} VkMemoryType_;

typedef struct {
    VkDeviceSize size;
    VkMemoryHeapFlags flags;
} VkMemoryHeap_;

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

// C helper to get physical device properties - returns device name as C string
const char* getPhysicalDeviceName(VkPhysicalDevice device) {
    static VkPhysicalDeviceProperties props;
    vkGetPhysicalDeviceProperties(device, &props);
    return props.deviceName;
}

// C helper to get queue family properties count and flags
void getQueueFamilyProperties(VkPhysicalDevice device, uint32_t* count, VkQueueFamilyProperties** props) {
    vkGetPhysicalDeviceQueueFamilyProperties(device, count, NULL);
    if (*count > 0) {
        *props = (VkQueueFamilyProperties*)calloc(*count, sizeof(VkQueueFamilyProperties));
        vkGetPhysicalDeviceQueueFamilyProperties(device, count, *props);
    }
}

// Free queue family properties
void freeQueueFamilyProperties(VkQueueFamilyProperties* props) {
    free(props);
}

// C helper to get device extension properties
int hasExtensionKHR(VkPhysicalDevice device, const char* extName) {
    uint32_t extCount = 0;
    vkEnumerateDeviceExtensionProperties(device, NULL, &extCount, NULL);
    VkExtensionProperties* exts = (VkExtensionProperties*)calloc(extCount, sizeof(VkExtensionProperties));
    vkEnumerateDeviceExtensionProperties(device, NULL, &extCount, exts);

    int found = 0;
    for (uint32_t i = 0; i < extCount; i++) {
        if (strcmp(exts[i].extensionName, extName) == 0) {
            found = 1;
            break;
        }
    }
    free(exts);
    return found;
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
import (
	"log"
	"os"
	"unsafe"

	vulkan "github.com/vulkan-go/vulkan"
)

func getPhysicalDeviceName(device C.VkPhysicalDevice) *C.char

func getQueueFamilyProperties(device C.VkPhysicalDevice, count *C.uint32_t, props **C.VkQueueFamilyProperties)

func freeQueueFamilyProperties(props *C.VkQueueFamilyProperties)

func hasExtensionKHR(device C.VkPhysicalDevice, extName *C.char) C.int

type C_VkMemoryRequirements struct {
    size            C.VkDeviceSize
    alignment       C.VkDeviceSize
    memoryTypeBits  C.uint32_t
}

// DmaBufImage holds the dmabuf-exported Vulkan image
type DmaBufImage struct {
	image    vulkan.Image
	memory   vulkan.DeviceMemory
	memoryFD int
	width    int
	height   int
	stride   int
	format   uint32 // DRM fourcc
}

var globalVulkan struct {
	instance            vulkan.Instance
	physicalDevice      vulkan.PhysicalDevice
	device              vulkan.Device
	graphicsQueueIndex  uint32
	initialized         bool
}

func initVulkan() bool {
	vulkan.SetDefaultGetInstanceProcAddr()
	if err := vulkan.Init(); err != nil {
		log.Printf("Failed to initialize Vulkan: %v", err)
		return false
	}

	// Create instance with external memory support
	appInfo := vulkan.ApplicationInfo{
		SType:           vulkan.StructureTypeApplicationInfo,
		ApiVersion:      vulkan.MakeVersion(1, 1, 0),
		PApplicationName: "Cheese DmaBuf\x00",
	}

	createInfo := vulkan.InstanceCreateInfo{
		SType:            vulkan.StructureTypeInstanceCreateInfo,
		PApplicationInfo: &appInfo,
	}

	var instance vulkan.Instance
	result := vulkan.CreateInstance(&createInfo, nil, &instance)
	if result != vulkan.Success {
		log.Printf("Failed to create Vulkan instance: %v", result)
		return false
	}
	globalVulkan.instance = instance

	// Initialize instance-level function pointers
	if err := vulkan.InitInstance(instance); err != nil {
		log.Printf("Failed to initialize instance: %v", err)
		return false
	}

	// Find physical device
	var deviceCount uint32
	result = vulkan.EnumeratePhysicalDevices(instance, &deviceCount, nil)
	if result != vulkan.Success || deviceCount == 0 {
		log.Println("No Vulkan physical devices found")
		return false
	}

	devices := make([]vulkan.PhysicalDevice, deviceCount)
	result = vulkan.EnumeratePhysicalDevices(instance, &deviceCount, devices)
	if result != vulkan.Success {
		log.Printf("Failed to enumerate physical devices: %v", result)
		return false
	}

	// Find first device with graphics queue
	for deviceIdx, device := range devices {
		// Get device name using C helper - PhysicalDevice is a pointer, get its value
		deviceNameC := C.getPhysicalDeviceName(C.VkPhysicalDevice(unsafe.Pointer(device)))
		deviceName := C.GoString(deviceNameC)
		log.Printf("Checking physical device %d: %s", deviceIdx, deviceName)

		// Get queue family properties using C helper
		var queueCount C.uint32_t
		var queueProps *C.VkQueueFamilyProperties
		C.getQueueFamilyProperties(C.VkPhysicalDevice(unsafe.Pointer(device)), &queueCount, &queueProps)
		defer C.freeQueueFamilyProperties(queueProps)

		// Convert C array to Go slice for iteration
		qProps := (*[100]C.VkQueueFamilyProperties)(unsafe.Pointer(queueProps))[:queueCount:queueCount]

		for i := C.uint32_t(0); i < queueCount; i++ {
			flags := uint32(qProps[i].queueFlags)
			log.Printf("  Queue family %d: flags=0x%08x", i, flags)
			if flags&0x00000001 != 0 { // VK_QUEUE_GRAPHICS_BIT
				log.Printf("  Queue family %d has graphics bit", i)
				globalVulkan.physicalDevice = device
				globalVulkan.graphicsQueueIndex = uint32(i)

				// Check for external memory extensions using C helper
				extMem := C.CString("VK_KHR_external_memory")
				defer C.free(unsafe.Pointer(extMem))
				extMemFD := C.CString("VK_KHR_external_memory_fd")
				defer C.free(unsafe.Pointer(extMemFD))
				hasExternalMemory := C.hasExtensionKHR(C.VkPhysicalDevice(unsafe.Pointer(device)), extMem) != 0
				hasExternalMemoryFD := C.hasExtensionKHR(C.VkPhysicalDevice(unsafe.Pointer(device)), extMemFD) != 0

				log.Printf("  VK_KHR_external_memory: %v", hasExternalMemory)
				log.Printf("  VK_KHR_external_memory_fd: %v", hasExternalMemoryFD)

				if hasExternalMemory && hasExternalMemoryFD {
					log.Println("Found Vulkan device with external memory support")
					goto foundDevice
				}
			}
		}
	}

	log.Println("No Vulkan device with external memory support found")
	return false

foundDevice:
	// Create logical device
	queuePriority := float32(1.0)
	queueCreateInfo := vulkan.DeviceQueueCreateInfo{
		SType:            vulkan.StructureTypeDeviceQueueCreateInfo,
		QueueFamilyIndex: globalVulkan.graphicsQueueIndex,
		QueueCount:       1,
		PQueuePriorities: []float32{queuePriority},
	}

	deviceExtensions := []string{"VK_KHR_external_memory\x00", "VK_KHR_external_memory_fd\x00"}

	deviceCreateInfo := vulkan.DeviceCreateInfo{
		SType:                 vulkan.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount:  1,
		PQueueCreateInfos:     []vulkan.DeviceQueueCreateInfo{queueCreateInfo},
		EnabledExtensionCount: uint32(len(deviceExtensions)),
		PpEnabledExtensionNames: deviceExtensions,
	}

	var device vulkan.Device
	result = vulkan.CreateDevice(globalVulkan.physicalDevice, &deviceCreateInfo, nil, &device)
	if result != vulkan.Success {
		log.Printf("Failed to create logical device: %v", result)
		return false
	}
	globalVulkan.device = device

	globalVulkan.initialized = true
	return true
}

func renderToDmaBuf(width, height int) (fd int, stride int, err error) {
	if !globalVulkan.initialized {
		if !initVulkan() {
			return -1, 0, os.ErrNotExist
		}
	}

	device := globalVulkan.device

	// Create image with external memory
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
		Usage:          0x00000010, // VK_IMAGE_USAGE_TRANSFER_DST_BIT
		Samples:        vulkan.SampleCount1Bit,
		SharingMode:    vulkan.SharingModeExclusive,
	}

	var image vulkan.Image
	result := vulkan.CreateImage(device, &imageCreateInfo, nil, &image)
	if result != vulkan.Success {
		log.Printf("Failed to create image: %v", result)
		return -1, 0, err
	}

	// Get memory requirements using inline CGO
	var memReqsC C.VkMemoryRequirements
	C.vkGetImageMemoryRequirements(C.VkDevice(unsafe.Pointer(device)), C.VkImage(unsafe.Pointer(image)), &memReqsC)
	memTypeBits := uint32(memReqsC.memoryTypeBits)
	log.Printf("Memory requirements: size=%d, alignment=%d, bits=0x%08x", memReqsC.size, memReqsC.alignment, memTypeBits)

	// Find memory type - simple approach: use first compatible type
	memTypeIndex := uint32(^uint32(0))
	for i := uint32(0); i < 32; i++ {
		if memTypeBits&(1<<i) != 0 {
			memTypeIndex = i
			log.Printf("Using memory type index: %d", i)
			break
		}
	}

	if memTypeIndex == ^uint32(0) {
		log.Println("Failed to find suitable memory type - no bits set in MemoryTypeBits")
		vulkan.DestroyImage(device, image, nil)
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
		log.Printf("Failed to allocate memory: %v", result)
		vulkan.DestroyImage(device, image, nil)
		return -1, 0, err
	}

	// Bind image memory
	result = vulkan.BindImageMemory(device, image, memory, 0)
	if result != vulkan.Success {
		log.Printf("Failed to bind image memory: %v", result)
		vulkan.FreeMemory(device, memory, nil)
		vulkan.DestroyImage(device, image, nil)
		return -1, 0, err
	}

	// Get the dmabuf file descriptor using vkGetMemoryFdKHR
	dmabufFD := getMemoryFdKHR(device, memory)

	// Calculate stride (row pitch)
	// For linear tiling with R8G8B8A8, each pixel is 4 bytes
	stride = width * 4

	// TODO: Actually fill the image with a color using a command buffer
	// For now, we've proven the dmabuf export works

	// Clean up
	vulkan.FreeMemory(device, memory, nil)
	vulkan.DestroyImage(device, image, nil)

	log.Printf("Exported dmabuf: fd=%d size=%d stride=%d", dmabufFD, memReqsC.size, stride)

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

func cleanupVulkan() {
	if globalVulkan.device != nil {
		vulkan.DestroyDevice(globalVulkan.device, nil)
	}
	if globalVulkan.instance != nil {
		vulkan.DestroyInstance(globalVulkan.instance, nil)
	}
}
