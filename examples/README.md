# Cheese Examples

Examples are split by abstraction level.

## With Toolkit

These examples use `client-toolkit`.

- `with-toolkit/monitor-info`
  Output discovery and hotplug tracking.
- `with-toolkit/vk-dmabuf`
  Toolkit-backed GPU rendering using DMA-BUF.
  Requires `glslc` at runtime to compile `shader.vert` / `shader.frag`.

## Without Toolkit

These examples use raw generated protocol bindings directly.

- `without-toolkit/rainbow`
  XDG toplevel window with SHM rendering.
- `without-toolkit/statusbar`
  Layer-shell panel with SHM rendering.
- `without-toolkit/vk-dmabuf`
  Raw DMA-BUF import path.
  Requires `glslc` at runtime to compile `shader.vert` / `shader.frag`.

## Notes

- The old toolkit-backed SHM `rainbow` and `statusbar` examples were removed because
  they referenced an API that no longer exists.
- If a toolkit example is added back, it should be built on the current APIs:
  `shm.Frame` for SHM or `gpu.Renderer` for GPU.
- The Vulkan examples no longer commit generated `.spv` shader blobs. They compile
  shader source at startup instead, so `glslc` must be available in `PATH`.
