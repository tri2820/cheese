# Cheese Toolkit

Wayland client toolkit for Go, built on top of `../protocols`.

See [SEMANTICS.md](./SEMANTICS.md) for the API contract and naming rules.

## Design

`client-toolkit` is layered.

- `display/`, `surface/`, `shell/`, `seat/`
  Thin wrappers around core Wayland concepts.
- `buffer/`
  Raw SHM memory, pools, slots, and `wl_buffer` resources.
- `shm/`
  SHM-backed swapchains and `Frame` for SHM render lifecycle.
- `dmabuf/`
  Raw DMA-BUF protocol state, params, formats, and `wl_buffer` resources.
- `gpu/`
  GPU-backed render lifecycle using DMA-BUF for presentation.
- `render/`
  Shared render-target interface used by higher-level helpers.

The toolkit does not currently expose a generic high-level SHM `Renderer` type.
For shared-memory rendering, the intended helper is `shm.Frame`.

## Current Rendering APIs

- SHM: `shm.NewFrame(...)` plus `frame.SetRender(...)`
- DMA-BUF: `gpu.NewRenderer(...)` with `CreateBuffers`, `Render`, and `DestroyBuffers`

Both lifecycle helpers expose:

- `OnResize(...)` for future size changes
- `OnError(...)` for runtime lifecycle errors
- `Ready()` for current readiness
- `SetManualMode(...)` and `ManualRender(...)` for app-driven pacing

## Minimal SHM Example

```go
package main

import (
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/shm"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
)

func main() {
	disp := display.MustConnect(display.Config{
		Required: display.RequiredGlobals{
			Compositor: true,
			Shm:        true,
			XdgWmBase:  true,
		},
	})
	defer disp.Close()

	surf, _ := surface.New(disp.Compositor())
	win, _ := shell.NewToplevel(surf, disp.XdgWmBase(), shell.ToplevelConfig{
		Title: "Example",
	})

	frame, _ := shm.NewFrame(disp.Shm(), win, disp, shm.FrameConfig{
		Format:  client.WlShmFormatXrgb8888,
		Buffers: 2,
	})

	frame.SetRender(func(w, h int, time uint32, pixels []byte) {
		_ = w
		_ = h
		_ = time
		_ = pixels
	})

	_ = disp.Run()
}
```

## Real References

- [monitor-info](/home/tri/cheese/examples/with-toolkit/monitor-info) for output discovery and hotplug
- [vk-dmabuf](/home/tri/cheese/examples/with-toolkit/vk-dmabuf) for the current toolkit-backed GPU path
- [cheesebar](/home/tri/cheese/apps/cheesebar) for a real application using the toolkit
