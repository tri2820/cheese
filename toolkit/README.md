# Cheese Toolkit

High-level Wayland client toolkit for Go.

Built on top of `../protocols` (low-level Wayland bindings).

## Packages

| Package | Level | Purpose |
|---------|-------|---------|
| `display/` | Low | Display connection, registry, event loop |
| `surface/` | Low | Surface creation, frame callbacks |
| `shell/` | Low | xdg-shell toplevel windows |
| `buffer/` | Low | SHM pools, slots, buffers |
| `buffer/` | **High** | **Swapchain** (double buffering), **Renderer** (render loop) |
| `input/` | ❌ | Keyboard, pointer, touch handling |
| `render/` | ❌ | GPU rendering (Vulkan, DMA-BUF export) |

## Architecture

```
┌─────────────────────────────────────┐
│         Renderer (High-Level)       │
│  - swapchain + window + render loop │
│  - OnRender(time, pixels)           │
└─────────────────────────────────────┘
              │
              ├─→ Swapchain (Mid-Level)
              │   - Pool + slot management
              │   - Acquire/Present
              │
              └─→ Window (Low-Level)
                  - xdg-shell management
                  - Configure/close handlers
```

## Quick Example

```go
package main

import (
    "github.com/tri2820/cheese/toolkit/display"
    "github.com/tri2820/cheese/toolkit/shell"
    "github.com/tri2820/cheese/toolkit/surface"
    "github.com/tri2820/cheese/toolkit/buffer"
)

func main() {
    // Connect to display
    disp := display.MustConnect(display.Config{
        Required: display.RequiredGlobals{
            Compositor: true,
            Shm:        true,
            XdgWmBase:  true,
        },
    })
    defer disp.Close()

    // Create window
    surf, _ := surface.New(disp.Compositor())
    win, _ := shell.NewToplevel(surf, disp.XdgWmBase(), shell.Config{
        Title: "My App",
        AppId: "my-app",
    })

    // Create renderer - handles all the Wayland render complexity
    renderer, _ := buffer.NewRenderer(buffer.RendererConfig{
        Shm:     disp.Shm(),
        Window:  win,
        Width:   800,
        Height:  600,
        Format:  buffer.FormatXRGB8888,
        Buffers: 2,
    })

    // Just provide a draw function
    renderer.OnRender(func(time uint32, pixels []byte) {
        // Draw your frame here
        // pixels is a []byte slice of the buffer
    })

    // Run event loop
    disp.Run()
}
```

See `examples/with-toolkit/rainbow` for a complete working example.

## Level Design

- **Low-Level Primitives**: `display/`, `surface/`, `shell/`, `buffer/` (Pool, Slot, Buffer)
  - Full control, 1:1 with Wayland concepts
  - Use when you need custom behavior

- **High-Level Abstraction**: `Renderer`
  - Handles Wayland complexity (configure, frame callbacks, double buffering)
  - Simple `OnRender(time, pixels)` callback
  - Recommended for most applications

**Pure Go**, no CGO except for GPU rendering.
