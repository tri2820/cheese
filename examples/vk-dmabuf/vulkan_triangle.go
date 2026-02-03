package main

import (
	_ "embed"
	"log"
	"os"
	"unsafe"

	vulkan "github.com/vulkan-go/vulkan"
)

//go:generate sh -c "glslc shader.vert -o vert.spv && glslc shader.frag -o frag.spv"

//go:embed vert.spv
var vertSpirv []byte

//go:embed frag.spv
var fragSpirv []byte

func loadVertShader() []uint32 {
	return unsafe.Slice((*uint32)(unsafe.Pointer(&vertSpirv[0])), len(vertSpirv)/4)
}

func loadFragShader() []uint32 {
	return unsafe.Slice((*uint32)(unsafe.Pointer(&fragSpirv[0])), len(fragSpirv)/4)
}

// TriangleRenderer holds persistent Vulkan resources for triangle rendering
type TriangleRenderer struct {
	initialized bool

	// Shaders
	vertShader vulkan.ShaderModule
	fragShader vulkan.ShaderModule

	// Pipeline resources
	renderPass     vulkan.RenderPass
	pipeline       vulkan.Pipeline
	pipelineLayout vulkan.PipelineLayout

	// Command buffer (reused and rerecorded each frame)
	commandBuffer vulkan.CommandBuffer

	// Dimensions
	width  int
	height int
}

var triangleRenderer TriangleRenderer

// InitTriangleRenderer initializes all persistent Vulkan resources for triangle rendering
func InitTriangleRenderer(width, height int) error {
	if triangleRenderer.initialized {
		return nil
	}

	device := globalVulkan.device
	triangleRenderer.width = width
	triangleRenderer.height = height

	// Create shaders (once)
	triangleRenderer.vertShader = createShaderModule(device, loadVertShader())
	triangleRenderer.fragShader = createShaderModule(device, loadFragShader())

	// Create render pass (once)
	triangleRenderer.renderPass = createRenderPass(device)

	// Create graphics pipeline (once)
	triangleRenderer.pipeline, triangleRenderer.pipelineLayout = createGraphicsPipeline(
		device, triangleRenderer.renderPass,
		triangleRenderer.vertShader, triangleRenderer.fragShader,
		width, height,
	)

	// Allocate command buffer (once, reused each frame)
	allocInfo := vulkan.CommandBufferAllocateInfo{
		SType:              vulkan.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        globalVulkan.commandPool,
		Level:              vulkan.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	}
	commandBuffers := make([]vulkan.CommandBuffer, 1)
	result := vulkan.AllocateCommandBuffers(device, &allocInfo, commandBuffers)
	if result != vulkan.Success {
		log.Printf("Failed to allocate command buffer: %v", result)
		return os.ErrNotExist
	}
	triangleRenderer.commandBuffer = commandBuffers[0]

	triangleRenderer.initialized = true
	log.Println("Triangle renderer initialized")
	return nil
}

// CleanupTriangleRenderer destroys all persistent resources
func CleanupTriangleRenderer() {
	if !triangleRenderer.initialized {
		return
	}

	device := globalVulkan.device

	if triangleRenderer.commandBuffer != nil {
		vulkan.FreeCommandBuffers(device, globalVulkan.commandPool, 1, []vulkan.CommandBuffer{triangleRenderer.commandBuffer})
	}
	if triangleRenderer.pipeline != nil {
		vulkan.DestroyPipeline(device, triangleRenderer.pipeline, nil)
	}
	if triangleRenderer.pipelineLayout != nil {
		vulkan.DestroyPipelineLayout(device, triangleRenderer.pipelineLayout, nil)
	}
	if triangleRenderer.renderPass != nil {
		vulkan.DestroyRenderPass(device, triangleRenderer.renderPass, nil)
	}
	if triangleRenderer.vertShader != nil {
		vulkan.DestroyShaderModule(device, triangleRenderer.vertShader, nil)
	}
	if triangleRenderer.fragShader != nil {
		vulkan.DestroyShaderModule(device, triangleRenderer.fragShader, nil)
	}

	triangleRenderer.initialized = false
	log.Println("Triangle renderer cleaned up")
}

// renderTriangleToImage renders a colored triangle to an image using the pre-initialized pipeline
// This is the fast path - only records command buffer and submits
func renderTriangleToImage(device vulkan.Device, image vulkan.Image, width, height int, time float32) {
	// Reinitialize if dimensions changed
	if !triangleRenderer.initialized || width != triangleRenderer.width || height != triangleRenderer.height {
		if triangleRenderer.initialized {
			CleanupTriangleRenderer()
		}
		if err := InitTriangleRenderer(width, height); err != nil {
			log.Printf("Failed to reinitialize triangle renderer: %v", err)
			return
		}
	}

	cmdBuf := triangleRenderer.commandBuffer

	// Reset and rerecord command buffer
	vulkan.ResetCommandBuffer(cmdBuf, 0)

	beginInfo := vulkan.CommandBufferBeginInfo{
		SType: vulkan.StructureTypeCommandBufferBeginInfo,
		Flags: vulkan.CommandBufferUsageFlags(vulkan.CommandBufferUsageOneTimeSubmitBit),
	}
	vulkan.BeginCommandBuffer(cmdBuf, &beginInfo)

	// Transition image layout to color attachment
	imageBarrier := vulkan.ImageMemoryBarrier{
		SType:               vulkan.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       0,
		DstAccessMask:       vulkan.AccessFlags(vulkan.AccessColorAttachmentWriteBit),
		OldLayout:           vulkan.ImageLayoutUndefined,
		NewLayout:           vulkan.ImageLayoutColorAttachmentOptimal,
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
		cmdBuf,
		vulkan.PipelineStageFlags(vulkan.PipelineStageTopOfPipeBit),
		vulkan.PipelineStageFlags(vulkan.PipelineStageColorAttachmentOutputBit),
		0, 0, nil, 0, nil, 1, []vulkan.ImageMemoryBarrier{imageBarrier},
	)

	// Create temporary image view for this frame's image
	imageView := createImageView(device, image)
	defer vulkan.DestroyImageView(device, imageView, nil)

	// Create temporary framebuffer for this frame's image
	framebuffer := createFramebuffer(device, triangleRenderer.renderPass, imageView, width, height)
	defer vulkan.DestroyFramebuffer(device, framebuffer, nil)

	// Begin render pass
	clearValue := vulkan.ClearValue{}
	clearValue.SetColor([]float32{0.0, 0.0, 0.0, 1.0})
	renderPassBegin := vulkan.RenderPassBeginInfo{
		SType:       vulkan.StructureTypeRenderPassBeginInfo,
		RenderPass:  triangleRenderer.renderPass,
		Framebuffer: framebuffer,
		RenderArea: vulkan.Rect2D{
			Offset: vulkan.Offset2D{X: 0, Y: 0},
			Extent: vulkan.Extent2D{Width: uint32(width), Height: uint32(height)},
		},
		ClearValueCount: 1,
		PClearValues:    []vulkan.ClearValue{clearValue},
	}
	vulkan.CmdBeginRenderPass(cmdBuf, &renderPassBegin, vulkan.SubpassContentsInline)

	// Bind pipeline (reused from initialization)
	vulkan.CmdBindPipeline(cmdBuf, vulkan.PipelineBindPointGraphics, triangleRenderer.pipeline)

	// Push time constant for animation
	vulkan.CmdPushConstants(cmdBuf, triangleRenderer.pipelineLayout, vulkan.ShaderStageFlags(vulkan.ShaderStageVertexBit), 0, 4, unsafe.Pointer(&time))

	// Set dynamic viewport and scissor
	viewport := vulkan.Viewport{
		X:        0.0,
		Y:        0.0,
		Width:    float32(width),
		Height:   float32(height),
		MinDepth: 0.0,
		MaxDepth: 1.0,
	}
	vulkan.CmdSetViewport(cmdBuf, 0, 1, []vulkan.Viewport{viewport})

	scissor := vulkan.Rect2D{
		Offset: vulkan.Offset2D{X: 0, Y: 0},
		Extent: vulkan.Extent2D{Width: uint32(width), Height: uint32(height)},
	}
	vulkan.CmdSetScissor(cmdBuf, 0, 1, []vulkan.Rect2D{scissor})

	// Draw
	vulkan.CmdDraw(cmdBuf, 3, 1, 0, 0)

	// End render pass
	vulkan.CmdEndRenderPass(cmdBuf)

	// Transition image layout back to general for copying
	imageBarrier.SrcAccessMask = vulkan.AccessFlags(vulkan.AccessColorAttachmentWriteBit)
	imageBarrier.DstAccessMask = vulkan.AccessFlags(vulkan.AccessMemoryReadBit)
	imageBarrier.OldLayout = vulkan.ImageLayoutColorAttachmentOptimal
	imageBarrier.NewLayout = vulkan.ImageLayoutGeneral
	vulkan.CmdPipelineBarrier(
		cmdBuf,
		vulkan.PipelineStageFlags(vulkan.PipelineStageColorAttachmentOutputBit),
		vulkan.PipelineStageFlags(vulkan.PipelineStageBottomOfPipeBit),
		0, 0, nil, 0, nil, 1, []vulkan.ImageMemoryBarrier{imageBarrier},
	)

	vulkan.EndCommandBuffer(cmdBuf)

	// Submit and wait with fence
	submitInfo := vulkan.SubmitInfo{
		SType:              vulkan.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    []vulkan.CommandBuffer{cmdBuf},
	}
	vulkan.ResetFences(globalVulkan.device, 1, []vulkan.Fence{globalVulkan.renderFence})
	vulkan.QueueSubmit(globalVulkan.graphicsQueue, 1, []vulkan.SubmitInfo{submitInfo}, globalVulkan.renderFence)
	vulkan.WaitForFences(globalVulkan.device, 1, []vulkan.Fence{globalVulkan.renderFence}, vulkan.True, 1000000000) // 1 second timeout
}

func createShaderModule(device vulkan.Device, code []uint32) vulkan.ShaderModule {
	createInfo := vulkan.ShaderModuleCreateInfo{
		SType:    vulkan.StructureTypeShaderModuleCreateInfo,
		CodeSize: uint64(len(code) * 4),
		PCode:    code,
	}
	var shaderModule vulkan.ShaderModule
	result := vulkan.CreateShaderModule(device, &createInfo, nil, &shaderModule)
	if result != vulkan.Success {
		log.Panicf("Failed to create shader module: %v", result)
	}
	return shaderModule
}

func createRenderPass(device vulkan.Device) vulkan.RenderPass {
	colorAttachment := vulkan.AttachmentDescription{
		Format:         vulkan.FormatR8g8b8a8Unorm,
		Samples:        1,
		LoadOp:         vulkan.AttachmentLoadOpClear,
		StoreOp:        vulkan.AttachmentStoreOpStore,
		StencilLoadOp:  vulkan.AttachmentLoadOpDontCare,
		StencilStoreOp: vulkan.AttachmentStoreOpDontCare,
		InitialLayout:  vulkan.ImageLayoutUndefined,
		FinalLayout:    vulkan.ImageLayoutColorAttachmentOptimal,
	}

	colorAttachmentRef := vulkan.AttachmentReference{
		Attachment: 0,
		Layout:     vulkan.ImageLayoutColorAttachmentOptimal,
	}

	subpass := vulkan.SubpassDescription{
		PipelineBindPoint:    vulkan.PipelineBindPointGraphics,
		ColorAttachmentCount: 1,
		PColorAttachments:    []vulkan.AttachmentReference{colorAttachmentRef},
	}

	dependency := vulkan.SubpassDependency{
		SrcSubpass:    vulkan.SubpassExternal,
		DstSubpass:    0,
		SrcStageMask:  vulkan.PipelineStageFlags(vulkan.PipelineStageColorAttachmentOutputBit),
		SrcAccessMask: 0,
		DstStageMask:  vulkan.PipelineStageFlags(vulkan.PipelineStageColorAttachmentOutputBit),
		DstAccessMask: vulkan.AccessFlags(vulkan.AccessColorAttachmentWriteBit),
	}

	renderPassInfo := vulkan.RenderPassCreateInfo{
		SType:           vulkan.StructureTypeRenderPassCreateInfo,
		AttachmentCount: 1,
		PAttachments:    []vulkan.AttachmentDescription{colorAttachment},
		SubpassCount:    1,
		PSubpasses:      []vulkan.SubpassDescription{subpass},
		DependencyCount: 1,
		PDependencies:   []vulkan.SubpassDependency{dependency},
	}

	var renderPass vulkan.RenderPass
	result := vulkan.CreateRenderPass(device, &renderPassInfo, nil, &renderPass)
	if result != vulkan.Success {
		log.Panicf("Failed to create render pass: %v", result)
	}
	return renderPass
}

func createImageView(device vulkan.Device, image vulkan.Image) vulkan.ImageView {
	viewInfo := vulkan.ImageViewCreateInfo{
		SType:    vulkan.StructureTypeImageViewCreateInfo,
		Image:    image,
		ViewType: vulkan.ImageViewType2d,
		Format:   vulkan.FormatR8g8b8a8Unorm,
		SubresourceRange: vulkan.ImageSubresourceRange{
			AspectMask:     vulkan.ImageAspectFlags(vulkan.ImageAspectColorBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}
	var imageView vulkan.ImageView
	result := vulkan.CreateImageView(device, &viewInfo, nil, &imageView)
	if result != vulkan.Success {
		log.Panicf("Failed to create image view: %v", result)
	}
	return imageView
}

func createFramebuffer(device vulkan.Device, renderPass vulkan.RenderPass, imageView vulkan.ImageView, width, height int) vulkan.Framebuffer {
	framebufferInfo := vulkan.FramebufferCreateInfo{
		SType:           vulkan.StructureTypeFramebufferCreateInfo,
		RenderPass:      renderPass,
		AttachmentCount: 1,
		PAttachments:    []vulkan.ImageView{imageView},
		Width:           uint32(width),
		Height:          uint32(height),
		Layers:          1,
	}
	var framebuffer vulkan.Framebuffer
	result := vulkan.CreateFramebuffer(device, &framebufferInfo, nil, &framebuffer)
	if result != vulkan.Success {
		log.Panicf("Failed to create framebuffer: %v", result)
	}
	return framebuffer
}

func createGraphicsPipeline(device vulkan.Device, renderPass vulkan.RenderPass, vertShader, fragShader vulkan.ShaderModule, width, height int) (vulkan.Pipeline, vulkan.PipelineLayout) {
	vertShaderStageInfo := vulkan.PipelineShaderStageCreateInfo{
		SType:  vulkan.StructureTypePipelineShaderStageCreateInfo,
		Stage:  vulkan.ShaderStageVertexBit,
		Module: vertShader,
		PName:  "main\x00",
	}

	fragShaderStageInfo := vulkan.PipelineShaderStageCreateInfo{
		SType:  vulkan.StructureTypePipelineShaderStageCreateInfo,
		Stage:  vulkan.ShaderStageFragmentBit,
		Module: fragShader,
		PName:  "main\x00",
	}

	shaderStages := []vulkan.PipelineShaderStageCreateInfo{vertShaderStageInfo, fragShaderStageInfo}

	vertexInputInfo := vulkan.PipelineVertexInputStateCreateInfo{
		SType: vulkan.StructureTypePipelineVertexInputStateCreateInfo,
	}

	inputAssembly := vulkan.PipelineInputAssemblyStateCreateInfo{
		SType:                  vulkan.StructureTypePipelineInputAssemblyStateCreateInfo,
		Topology:               vulkan.PrimitiveTopologyTriangleList,
		PrimitiveRestartEnable: vulkan.False,
	}

	viewportState := vulkan.PipelineViewportStateCreateInfo{
		SType:         vulkan.StructureTypePipelineViewportStateCreateInfo,
		ViewportCount: 1,
		ScissorCount:  1,
	}

	dynamicState := vulkan.PipelineDynamicStateCreateInfo{
		SType:             vulkan.StructureTypePipelineDynamicStateCreateInfo,
		DynamicStateCount: 2,
		PDynamicStates: []vulkan.DynamicState{
			vulkan.DynamicStateViewport,
			vulkan.DynamicStateScissor,
		},
	}

	rasterizer := vulkan.PipelineRasterizationStateCreateInfo{
		SType:                   vulkan.StructureTypePipelineRasterizationStateCreateInfo,
		DepthClampEnable:        vulkan.False,
		RasterizerDiscardEnable: vulkan.False,
		PolygonMode:             vulkan.PolygonModeFill,
		LineWidth:               1.0,
		CullMode:                vulkan.CullModeFlags(vulkan.CullModeBackBit),
		FrontFace:               vulkan.FrontFaceClockwise,
		DepthBiasEnable:         vulkan.False,
	}

	multisampling := vulkan.PipelineMultisampleStateCreateInfo{
		SType:                vulkan.StructureTypePipelineMultisampleStateCreateInfo,
		SampleShadingEnable:  vulkan.False,
		RasterizationSamples: 1,
	}

	colorBlendAttachment := vulkan.PipelineColorBlendAttachmentState{
		ColorWriteMask: vulkan.ColorComponentFlags(vulkan.ColorComponentRBit | vulkan.ColorComponentGBit | vulkan.ColorComponentBBit | vulkan.ColorComponentABit),
		BlendEnable:    vulkan.False,
	}

	colorBlending := vulkan.PipelineColorBlendStateCreateInfo{
		SType:           vulkan.StructureTypePipelineColorBlendStateCreateInfo,
		LogicOpEnable:   vulkan.False,
		LogicOp:         vulkan.LogicOpCopy,
		AttachmentCount: 1,
		PAttachments:    []vulkan.PipelineColorBlendAttachmentState{colorBlendAttachment},
	}

	// Push constant range for time (float32 = 4 bytes)
	pushConstantRange := vulkan.PushConstantRange{
		StageFlags: vulkan.ShaderStageFlags(vulkan.ShaderStageVertexBit),
		Offset:     0,
		Size:       4,
	}

	pipelineLayoutInfo := vulkan.PipelineLayoutCreateInfo{
		SType:                vulkan.StructureTypePipelineLayoutCreateInfo,
		PushConstantRangeCount: 1,
		PPushConstantRanges: []vulkan.PushConstantRange{pushConstantRange},
	}

	var pipelineLayout vulkan.PipelineLayout
	result := vulkan.CreatePipelineLayout(device, &pipelineLayoutInfo, nil, &pipelineLayout)
	if result != vulkan.Success {
		log.Panicf("Failed to create pipeline layout: %v", result)
	}

	depthStencil := vulkan.PipelineDepthStencilStateCreateInfo{
		SType: vulkan.StructureTypePipelineDepthStencilStateCreateInfo,
	}

	pipelineInfo := vulkan.GraphicsPipelineCreateInfo{
		SType:               vulkan.StructureTypeGraphicsPipelineCreateInfo,
		StageCount:          2,
		PStages:             shaderStages,
		PVertexInputState:   &vertexInputInfo,
		PInputAssemblyState: &inputAssembly,
		PViewportState:      &viewportState,
		PRasterizationState: &rasterizer,
		PMultisampleState:   &multisampling,
		PDepthStencilState:  &depthStencil,
		PColorBlendState:    &colorBlending,
		PDynamicState:       &dynamicState,
		Layout:              pipelineLayout,
		RenderPass:          renderPass,
		Subpass:             0,
		BasePipelineHandle:  vulkan.NullPipeline,
	}

	pipelines := make([]vulkan.Pipeline, 1)
	result = vulkan.CreateGraphicsPipelines(device, vulkan.NullPipelineCache, 1, []vulkan.GraphicsPipelineCreateInfo{pipelineInfo}, nil, pipelines)
	if result != vulkan.Success {
		log.Panicf("Failed to create graphics pipeline: %v", result)
	}

	return pipelines[0], pipelineLayout
}
