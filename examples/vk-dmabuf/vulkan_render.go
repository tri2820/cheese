package main

import (
	"unsafe"

	vulkan "github.com/vulkan-go/vulkan"
)

// clearImageToColor fills an image with a solid color using Vulkan transfer commands
func clearImageToColor(device vulkan.Device, image vulkan.Image, width, height int) {
	// Allocate command buffer
	allocInfo := vulkan.CommandBufferAllocateInfo{
		SType:              vulkan.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        globalVulkan.commandPool,
		Level:              vulkan.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	}
	commandBuffers := make([]vulkan.CommandBuffer, 1)
	vulkan.AllocateCommandBuffers(device, &allocInfo, commandBuffers)
	commandBuffer := commandBuffers[0]
	defer vulkan.FreeCommandBuffers(device, globalVulkan.commandPool, 1, commandBuffers)

	// Record command buffer
	beginInfo := vulkan.CommandBufferBeginInfo{
		SType: vulkan.StructureTypeCommandBufferBeginInfo,
		Flags: vulkan.CommandBufferUsageFlags(vulkan.CommandBufferUsageOneTimeSubmitBit),
	}
	vulkan.BeginCommandBuffer(commandBuffer, &beginInfo)

	// Transition image to transfer dst
	barrier := vulkan.ImageMemoryBarrier{
		SType:               vulkan.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       0,
		DstAccessMask:       vulkan.AccessFlags(vulkan.AccessTransferWriteBit),
		OldLayout:           vulkan.ImageLayoutUndefined,
		NewLayout:           vulkan.ImageLayoutTransferDstOptimal,
		SrcQueueFamilyIndex: vulkan.QueueFamilyIgnored,
		DstQueueFamilyIndex: vulkan.QueueFamilyIgnored,
		Image:               image,
		SubresourceRange: vulkan.ImageSubresourceRange{
			AspectMask:     vulkan.ImageAspectFlags(vulkan.ImageAspectColorBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}
	vulkan.CmdPipelineBarrier(
		commandBuffer,
		vulkan.PipelineStageFlags(vulkan.PipelineStageTopOfPipeBit),
		vulkan.PipelineStageFlags(vulkan.PipelineStageTransferBit),
		0, 0, nil, 0, nil, 1, []vulkan.ImageMemoryBarrier{barrier},
	)

	// Clear to an orange color
	// Note: swap R and B because Vulkan uses R8G8B8A8 but Wayland expects XRGB8888 (BGRX)
	colorFloats := [4]float32{0.2, 0.5, 1.0, 1.0} // BGR order: orange becomes (0.2, 0.5, 1.0)
	clearColor := (*vulkan.ClearColorValue)(unsafe.Pointer(&colorFloats[0]))

	subresourceRange := vulkan.ImageSubresourceRange{
		AspectMask:     vulkan.ImageAspectFlags(vulkan.ImageAspectColorBit),
		BaseMipLevel:   0,
		LevelCount:     1,
		BaseArrayLayer: 0,
		LayerCount:     1,
	}
	vulkan.CmdClearColorImage(commandBuffer, image, vulkan.ImageLayoutTransferDstOptimal, clearColor, 1, []vulkan.ImageSubresourceRange{subresourceRange})

	// Transition to general layout
	barrier.SrcAccessMask = vulkan.AccessFlags(vulkan.AccessTransferWriteBit)
	barrier.DstAccessMask = vulkan.AccessFlags(vulkan.AccessMemoryReadBit)
	barrier.OldLayout = vulkan.ImageLayoutTransferDstOptimal
	barrier.NewLayout = vulkan.ImageLayoutGeneral

	vulkan.CmdPipelineBarrier(
		commandBuffer,
		vulkan.PipelineStageFlags(vulkan.PipelineStageTransferBit),
		vulkan.PipelineStageFlags(vulkan.PipelineStageBottomOfPipeBit),
		0, 0, nil, 0, nil, 1, []vulkan.ImageMemoryBarrier{barrier},
	)

	vulkan.EndCommandBuffer(commandBuffer)

	// Submit and wait
	submitInfo := vulkan.SubmitInfo{
		SType:              vulkan.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    commandBuffers,
	}
	vulkan.QueueSubmit(globalVulkan.graphicsQueue, 1, []vulkan.SubmitInfo{submitInfo}, vulkan.NullFence)
	vulkan.QueueWaitIdle(globalVulkan.graphicsQueue)
}

// copyImageToImage copies content from one image to another using Vulkan transfer commands
func copyImageToImage(device vulkan.Device, srcImage, dstImage vulkan.Image, width, height int) {
	// Allocate command buffer
	allocInfo := vulkan.CommandBufferAllocateInfo{
		SType:              vulkan.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        globalVulkan.commandPool,
		Level:              vulkan.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	}
	commandBuffers := make([]vulkan.CommandBuffer, 1)
	vulkan.AllocateCommandBuffers(device, &allocInfo, commandBuffers)
	commandBuffer := commandBuffers[0]
	defer vulkan.FreeCommandBuffers(device, globalVulkan.commandPool, 1, commandBuffers)

	// Record command buffer
	beginInfo := vulkan.CommandBufferBeginInfo{
		SType: vulkan.StructureTypeCommandBufferBeginInfo,
		Flags: vulkan.CommandBufferUsageFlags(vulkan.CommandBufferUsageOneTimeSubmitBit),
	}
	vulkan.BeginCommandBuffer(commandBuffer, &beginInfo)

	// Transition src image to transfer src
	srcBarrier := vulkan.ImageMemoryBarrier{
		SType:               vulkan.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       vulkan.AccessFlags(vulkan.AccessColorAttachmentWriteBit),
		DstAccessMask:       vulkan.AccessFlags(vulkan.AccessTransferReadBit),
		OldLayout:           vulkan.ImageLayoutGeneral,
		NewLayout:           vulkan.ImageLayoutTransferSrcOptimal,
		SrcQueueFamilyIndex: vulkan.QueueFamilyIgnored,
		DstQueueFamilyIndex: vulkan.QueueFamilyIgnored,
		Image:               srcImage,
		SubresourceRange: vulkan.ImageSubresourceRange{
			AspectMask:     vulkan.ImageAspectFlags(vulkan.ImageAspectColorBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}

	// Transition dst image to transfer dst
	dstBarrier := vulkan.ImageMemoryBarrier{
		SType:               vulkan.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       0,
		DstAccessMask:       vulkan.AccessFlags(vulkan.AccessTransferWriteBit),
		OldLayout:           vulkan.ImageLayoutUndefined,
		NewLayout:           vulkan.ImageLayoutTransferDstOptimal,
		SrcQueueFamilyIndex: vulkan.QueueFamilyIgnored,
		DstQueueFamilyIndex: vulkan.QueueFamilyIgnored,
		Image:               dstImage,
		SubresourceRange: vulkan.ImageSubresourceRange{
			AspectMask:     vulkan.ImageAspectFlags(vulkan.ImageAspectColorBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}

	vulkan.CmdPipelineBarrier(
		commandBuffer,
		vulkan.PipelineStageFlags(vulkan.PipelineStageColorAttachmentOutputBit),
		vulkan.PipelineStageFlags(vulkan.PipelineStageTransferBit),
		0, 0, nil, 0, nil, 2, []vulkan.ImageMemoryBarrier{srcBarrier, dstBarrier},
	)

	// Copy image
	region := vulkan.ImageCopy{
		SrcSubresource: vulkan.ImageSubresourceLayers{
			AspectMask:     vulkan.ImageAspectFlags(vulkan.ImageAspectColorBit),
			MipLevel:       0,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
		SrcOffset: vulkan.Offset3D{X: 0, Y: 0, Z: 0},
		DstSubresource: vulkan.ImageSubresourceLayers{
			AspectMask:     vulkan.ImageAspectFlags(vulkan.ImageAspectColorBit),
			MipLevel:       0,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
		DstOffset: vulkan.Offset3D{X: 0, Y: 0, Z: 0},
		Extent:    vulkan.Extent3D{Width: uint32(width), Height: uint32(height), Depth: 1},
	}
	vulkan.CmdCopyImage(commandBuffer, srcImage, vulkan.ImageLayoutTransferSrcOptimal,
		dstImage, vulkan.ImageLayoutTransferDstOptimal, 1, []vulkan.ImageCopy{region})

	// Transition dst image to general layout
	dstBarrier.SrcAccessMask = vulkan.AccessFlags(vulkan.AccessTransferWriteBit)
	dstBarrier.DstAccessMask = vulkan.AccessFlags(vulkan.AccessMemoryReadBit)
	dstBarrier.OldLayout = vulkan.ImageLayoutTransferDstOptimal
	dstBarrier.NewLayout = vulkan.ImageLayoutGeneral

	vulkan.CmdPipelineBarrier(
		commandBuffer,
		vulkan.PipelineStageFlags(vulkan.PipelineStageTransferBit),
		vulkan.PipelineStageFlags(vulkan.PipelineStageBottomOfPipeBit),
		0, 0, nil, 0, nil, 1, []vulkan.ImageMemoryBarrier{dstBarrier},
	)

	vulkan.EndCommandBuffer(commandBuffer)

	// Submit and wait
	submitInfo := vulkan.SubmitInfo{
		SType:              vulkan.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    commandBuffers,
	}
	vulkan.QueueSubmit(globalVulkan.graphicsQueue, 1, []vulkan.SubmitInfo{submitInfo}, vulkan.NullFence)
	vulkan.QueueWaitIdle(globalVulkan.graphicsQueue)
}
