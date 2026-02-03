package main

import (
	"log"
	"unsafe"

	vulkan "github.com/vulkan-go/vulkan"
)

/*
#cgo linux CFLAGS: -I/usr/include/vulkan
#cgo linux LDFLAGS: -lvulkan
#include <vulkan/vulkan.h>
#include <stdlib.h>
#include <string.h>

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
*/
import "C"

var globalVulkan struct {
	instance            vulkan.Instance
	physicalDevice      vulkan.PhysicalDevice
	device              vulkan.Device
	graphicsQueue       vulkan.Queue
	graphicsQueueIndex  uint32
	commandPool         vulkan.CommandPool
	renderFence         vulkan.Fence
	initialized         bool
}

func initVulkan() bool {
	vulkan.SetDefaultGetInstanceProcAddr()
	if err := vulkan.Init(); err != nil {
		log.Printf("Failed to initialize Vulkan: %v", err)
		return false
	}

	// Create instance with external memory support and validation layers
	appInfo := vulkan.ApplicationInfo{
		SType:           vulkan.StructureTypeApplicationInfo,
		ApiVersion:      vulkan.MakeVersion(1, 1, 0),
		PApplicationName: "Cheese DmaBuf\x00",
	}

	validationLayers := []string{
		"VK_LAYER_KHRONOS_validation\x00",
	}

	createInfo := vulkan.InstanceCreateInfo{
		SType:                   vulkan.StructureTypeInstanceCreateInfo,
		PApplicationInfo:        &appInfo,
		EnabledLayerCount:       uint32(len(validationLayers)),
		PpEnabledLayerNames:     validationLayers,
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

	deviceExtensions := []string{
		"VK_KHR_external_memory\x00",
		"VK_KHR_external_memory_fd\x00",
		"VK_EXT_external_memory_dma_buf\x00",
	}

	deviceCreateInfo := vulkan.DeviceCreateInfo{
		SType:                   vulkan.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount:    1,
		PQueueCreateInfos:       []vulkan.DeviceQueueCreateInfo{queueCreateInfo},
		EnabledExtensionCount:   uint32(len(deviceExtensions)),
		PpEnabledExtensionNames: deviceExtensions,
		EnabledLayerCount:       uint32(len(validationLayers)),
		PpEnabledLayerNames:     validationLayers,
	}

	var device vulkan.Device
	result = vulkan.CreateDevice(globalVulkan.physicalDevice, &deviceCreateInfo, nil, &device)
	if result != vulkan.Success {
		log.Printf("Failed to create logical device: %v", result)
		return false
	}
	globalVulkan.device = device

	// Get the graphics queue
	var queue vulkan.Queue
	vulkan.GetDeviceQueue(device, globalVulkan.graphicsQueueIndex, 0, &queue)
	globalVulkan.graphicsQueue = queue

	// Create command pool
	poolInfo := vulkan.CommandPoolCreateInfo{
		SType:            vulkan.StructureTypeCommandPoolCreateInfo,
		QueueFamilyIndex: globalVulkan.graphicsQueueIndex,
		Flags:            vulkan.CommandPoolCreateFlags(vulkan.CommandPoolCreateResetCommandBufferBit),
	}
	var commandPool vulkan.CommandPool
	result = vulkan.CreateCommandPool(device, &poolInfo, nil, &commandPool)
	if result != vulkan.Success {
		log.Printf("Failed to create command pool: %v", result)
		return false
	}
	globalVulkan.commandPool = commandPool

	// Create fence for synchronization
	fenceInfo := vulkan.FenceCreateInfo{
		SType: vulkan.StructureTypeFenceCreateInfo,
		Flags: vulkan.FenceCreateFlags(vulkan.FenceCreateSignaledBit),
	}
	var fence vulkan.Fence
	result = vulkan.CreateFence(device, &fenceInfo, nil, &fence)
	if result != vulkan.Success {
		log.Printf("Failed to create fence: %v", result)
		return false
	}
	globalVulkan.renderFence = fence

	globalVulkan.initialized = true
	return true
}

func cleanupVulkan() {
	if globalVulkan.renderFence != nil {
		vulkan.DestroyFence(globalVulkan.device, globalVulkan.renderFence, nil)
	}
	if globalVulkan.commandPool != nil {
		vulkan.DestroyCommandPool(globalVulkan.device, globalVulkan.commandPool, nil)
	}
	if globalVulkan.device != nil {
		vulkan.DestroyDevice(globalVulkan.device, nil)
	}
	if globalVulkan.instance != nil {
		vulkan.DestroyInstance(globalVulkan.instance, nil)
	}
}
