# Cheese Toolkit

High-level Wayland client toolkit for Go.

Built on top of `../protocols` (low-level Wayland bindings).

## Packages

| Package | Status | Purpose |
|---------|--------|---------|
| `buffer/` | ✅ | SHM pool, slot, and buffer management |
| `display/` | ✅ | Display connection, registry, event loop |
| `surface/` | ✅ | Surface creation, frame callbacks |
| `shell/` | ✅ | xdg-shell toplevel windows |
| `input/` | ❌ | Keyboard, pointer, touch handling |
| `render/` | ❌ | GPU rendering (Vulkan, DMA-BUF export) |

## Example

```go
package main

import (
    "github.com/tri2820/cheese/toolkit/display"
    "github.com/tri2820/cheese/toolkit/surface"
    "github.com/tri2820/cheese/toolkit/shell"
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

    // Create surface and window
    surf, _ := surface.New(disp.Compositor())
    win, _ := shell.NewToplevel(surf, disp.XdgWmBase(), shell.Config{
        Title: "My App",
        AppId: "my-app",
    })

    // Create SHM pool for double buffering
    pool, _ := buffer.NewPool(disp.Shm(), buffer.Config{
        Width:  800,
        Height: 120,
        Format: buffer.FormatXRGB8888,
    })

    // Set up frame callback
    win.Surface().SetFrameHandler(func() {
        // Draw your frame here
        // ...

        // Request next frame
        win.Surface().RequestFrame(func() {
            // ...next frame
        })
        win.Surface().Commit()
    })

    // Run event loop
    disp.Run()
}
```

See `examples/with-toolkit/rainbow` for a complete working example.

**Pure Go**, no CGO except for GPU rendering.
