# Cheese Vulkan DmaBuf Example

A minimal example demonstrating **Vulkan → dmabuf → Wayland** using the pure Go cheese library.

## What It Does

1. **Vulkan**: Creates an image with external memory support
2. **Export**: Gets the dmabuf file descriptor via `VK_KHR_external_memory_fd`
3. **Import**: Creates a `wl_buffer` via `zwp_linux_dmabuf_v1`
4. **Present**: Displays the buffer on Wayland

## Why This Example?

Proves that GPU-accelerated rendering can work with the pure Go cheese library through **dmabuf**:

```
Vulkan (GPU) → dmabuf FD → linux_dmabuf_v1 → Wayland compositor
```

No CGO needed in the cheese library!

## Prerequisites

### System Dependencies

```bash
# Ubuntu/Debian
sudo apt install vulkan-headers vulkan-loader libvulkan1 mesa-vulkan-drivers

# Fedora
sudo dnf install vulkan-headers vulkan-loader mesa-vulkan-drivers

# Nix (flake-based)
nix develop .  # The flake includes Vulkan
```

### Go Dependencies

```bash
go get github.com/vulkan-go/vulkan
```

## Building

```bash
nix develop . --command go build -o ./examples/vk-dmabuf/vk-dmabuf ./examples/vk-dmabuf/
```

## Running

```bash
nix develop . --command ./examples/vk-dmabuf/vk-dmabuf
```

## Expected Output

```
Starting Cheese Vulkan DmaBuf Example...
Bound to zwp_linux_dmabuf_v1
Found Vulkan device with external memory support
Running! Close the window to exit.
Rendering 400x300 frame
Got dmabuf fd=XX stride=1600
```

## How It Works

### 1. Vulkan Side (`vulkan.go`)

```go
// Create image with external memory
externalInfo := vulkan.ExternalMemoryImageCreateInfo{
    HandleTypes: VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT
}

// Allocate with export capability
exportInfo := vulkan.ExportMemoryAllocateInfo{
    HandleTypes: VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT
}

// Get the dmabuf file descriptor
vulkan.GetMemoryFdKHR(device, &getFDInfo, &dmabufFD)
```

### 2. Wayland Side (`main.go`)

```go
// Create params for dmabuf import
params := dmabuf.CreateParams()

// Add the dmabuf plane
params.Add(fd, plane_idx, offset, stride, modifier_hi, modifier_lo)

// Create wl_buffer immediately
buffer := params.CreateImmed(width, height, format, flags)
```

## File Structure

```
examples/vk-dmabuf/
├── main.go      # Wayland + dmabuf import
├── vulkan.go    # Vulkan + dmabuf export
└── README.md     # This file
```

## Next Steps

To add actual GPU rendering:

1. **Add shaders** (SPIR-V embedded)
2. **Create pipeline** (vertex + fragment)
3. **Add vertex/index buffers**
4. **Use command buffers** to draw

See `examples/vk-cube/` for a full 3D rotating cube example.

## Troubleshooting

### "No zwp_linux_dmabuf_v1 global"

Your compositor doesn't support dmabuf. Try:
- Weston
- wlroots-based compositors (sway, river, etc.)
- GNOME with Wayland

### "No Vulkan device with external memory support"

Your GPU driver doesn't support dmabuf export. Install:
- Mesa Vulkan drivers (Intel/AMD)
- NVIDIA proprietary drivers

### Black window

The dmabuf was exported but not filled with content. This is expected for this minimal example.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  Cheese Library (Pure Go, No CGO)                   │
│  ┌─────────────────────────────────────────────────┐ │
│  │  main.go                                        │ │
│  │  - Wayland connection (xdg_shell)               │ │
│  │  - zwp_linux_dmabuf_v1 import                   │ │
│  └─────────────────────────────────────────────────┘ │
│                         ↓                             │
│  ┌─────────────────────────────────────────────────┐ │
│  │  vulkan.go (via vulkan-go, uses CGO)            │ │
│  │  - Vulkan image creation                        │ │
│  │  - dmabuf export (VK_KHR_external_memory_fd)    │ │
│  └─────────────────────────────────────────────────┘ │
│                         ↓ dmabuf FD                  │
│  ┌─────────────────────────────────────────────────┐ │
│  │  Compositor (Wayland)                            │ │
│  │  - GPU scanout of dmabuf                         │ │
│  └─────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

## Key Point

**The cheese library remains pure Go.** Only the rendering layer (vulkan-go) uses CGO.


## Examples

```sh
VK_ICD_FILENAMES="/run/opengl-driver/share/vulkan/icd.d/intel_icd.x86_64.json" LD_LIBRARY_PATH="/run/opengl-driver/lib:$LD_LIBRARY_PATH/" ./vk-dmabuf
```

```sh
VK_ICD_FILENAMES="/run/opengl-driver/share/vulkan/icd.d/nvidia_icd.x86_64.json" LD_LIBRARY_PATH="/run/opengl-driver/lib:/nix/store/l0a355pbhjr49y6a33f3pag5cafi3cxs-nvidia-x11-580.119.02-6.18.7/lib:$LD_LIBRARY_PATH" ./vk-dmabuf
```