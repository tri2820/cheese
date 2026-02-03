# Cheese Vulkan DmaBuf Example

A minimal example demonstrating **Vulkan → dmabuf → Wayland** using the Cheese library.

## What It Does

1. **Vulkan**: Creates an image with external memory support
2. **Export**: Gets the dmabuf file descriptor via `VK_KHR_external_memory_fd`
3. **Import**: Creates a `wl_buffer` via `zwp_linux_dmabuf_v1`
4. **Present**: Displays the buffer on Wayland

## Why This Example?

Proves that GPU-accelerated rendering can work with the pure Go Cheese library through **dmabuf**:

```
Vulkan (GPU) → dmabuf FD → linux_dmabuf_v1 → Wayland compositor
```

CGO is needed for go-vulkan, which wrapping vulkan driver under the hood. CGO is NOT needed in the Cheese library!

## Prerequisites

### System Dependencies

```bash
# Nix (flake-based)
nix develop .  # The flake includes Vulkan
```

## Building

```bash
nix develop . --command go build -o ./examples/vk-dmabuf/vk-dmabuf ./examples/vk-dmabuf/
```

## Running

```bash
nix develop . --command ./examples/vk-dmabuf/vk-dmabuf
```

## File Structure

```
examples/vk-dmabuf/
├── main.go      # Wayland + dmabuf import
├── vulkan.go    # Vulkan + dmabuf export
└── README.md     # This file
```

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

## Examples

Run with NVIDIA GeForce MX250

```sh
tri@nixos ~/c/c/e/vk-dmabuf (main)> VK_ICD_FILENAMES="/run/opengl-driver/share/vulkan/icd.d/nvidia_icd.x86_64.json" LD_LIBRARY_PATH="/run/opengl-driver/lib:/nix/store/l0a355pbhjr49y6a33f3pag5cafi3cxs-nvidia-x11-580.119.02-6.18.7/lib:$LD_LIBRARY_PATH" ./vk-dmabuf
2026/02/03 11:07:28 Starting Cheese Vulkan DmaBuf Example...
2026/02/03 11:07:28 Bound to zwp_linux_dmabuf_v1
2026/02/03 11:07:28 Running! Close the window to exit.
2026/02/03 11:07:28 Initializing dmabuf buffers at 920x1018...
2026/02/03 11:07:28 Checking physical device 0: NVIDIA GeForce MX250
2026/02/03 11:07:28   Queue family 0: flags=0x0000000f
2026/02/03 11:07:28   Queue family 0 has graphics bit
2026/02/03 11:07:28   VK_KHR_external_memory: true
2026/02/03 11:07:28   VK_KHR_external_memory_fd: true
2026/02/03 11:07:28 Found Vulkan device with external memory support
2026/02/03 11:07:28 Created dmabuf buffer 0: fd=34 stride=3712
2026/02/03 11:07:28 Created dmabuf buffer 1: fd=35 stride=3712
2026/02/03 11:07:28 Created render image: 920x1018
2026/02/03 11:07:28 Triangle renderer initialized
```

Run with Intel(R) UHD Graphics 620 (WHL GT2)

```sh
tri@nixos ~/c/c/e/vk-dmabuf (main) [SIGINT]> VK_ICD_FILENAMES="/run/opengl-driver/share/vulkan/icd.d/intel_icd.x86_64.json" LD_LIBRARY_PATH="/run/opengl-driver/lib:$LD_LIBRARY_PATH/" ./vk-dmabuf
2026/02/03 11:09:53 Starting Cheese Vulkan DmaBuf Example...
2026/02/03 11:09:53 Bound to zwp_linux_dmabuf_v1
2026/02/03 11:09:53 Running! Close the window to exit.
2026/02/03 11:09:53 Initializing dmabuf buffers at 920x1018...
2026/02/03 11:09:53 Checking physical device 0: Intel(R) UHD Graphics 620 (WHL GT2)
2026/02/03 11:09:53   Queue family 0: flags=0x00000007
2026/02/03 11:09:53   Queue family 0 has graphics bit
2026/02/03 11:09:53   VK_KHR_external_memory: true
2026/02/03 11:09:53   VK_KHR_external_memory_fd: true
2026/02/03 11:09:53 Found Vulkan device with external memory support
2026/02/03 11:09:53 Created dmabuf buffer 0: fd=15 stride=3680
2026/02/03 11:09:53 Created dmabuf buffer 1: fd=16 stride=3680
2026/02/03 11:09:53 Created render image: 920x1018
2026/02/03 11:09:53 Triangle renderer initialized
```