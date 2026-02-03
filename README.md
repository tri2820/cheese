# Cheese

Pure Go Wayland client toolkit and protocol generator.

## Installation

```bash
# Install the client toolkit
go get github.com/tri2820/cheese/client-toolkit

# Install protocol bindings only
go get github.com/tri2820/cheese/protocols
```

## Quick Start

```go
package main

import (
    "github.com/tri2820/cheese/client-toolkit/display"
    "github.com/tri2820/cheese/client-toolkit/shell"
    "github.com/tri2820/cheese/client-toolkit/surface"
)

func main() {
    // Connect to Wayland display
    disp := display.MustConnect(display.Config{
        Required: display.RequiredGlobals{
            Compositor: true,
            XdgWmBase:  true,
        },
    })
    defer disp.Close()

    // Create surface and window
    surf, _ := surface.New(disp.Compositor())
    win, _ := shell.NewToplevel(surf, disp.XdgWmBase(), shell.ToplevelConfig{
        Title: "Hello Wayland",
        Width: 400,
        Height: 300,
    })

    // Run event loop
    disp.Run()
}
```

## Structure

```
cheese/
├── cmd/
│   └── wayland-scanner/     # Protocol scanner (XML → Go)
├── protocols/               # Generated Wayland protocol bindings
│   ├── client/             # Core Wayland protocol
│   ├── xdg-shell/          # XDG shell (windows)
│   ├── linux_dmabuf_v1/    # DMA-BUF (GPU rendering)
│   └── ...
├── client-toolkit/         # High-level Wayland client toolkit
│   ├── display/            # Display connection
│   ├── surface/            # Surface management
│   ├── shell/              # XDG shell, layer shell
│   ├── buffer/             # SHM rendering
│   └── dmabuf/             # GPU rendering
├── examples/               # Example applications
└── apps/                   # Desktop applications (bar, launcher, etc.)
```

## Modules

| Module | Description | Dependencies |
|--------|-------------|--------------|
| `protocols` | Wayland protocol bindings | pure Go |
| `client-toolkit` | Client toolkit | pure Go |
| `examples` | Example apps | pure Go (except vk-dmabuf) |

## Development

```bash
nix develop
go run ./cmd/wayland-scanner -i protocol.xml -o output.go
```

## Goals

- **Pure Go** - No CGO dependencies in core libraries
- **Clean API** - Idiomatic Go interfaces
- **Layer Shell** - First-class support for bars/panels
- **GPU Ready** - DMA-BUF support for Vulkan/OpenGL rendering

## License

MIT
