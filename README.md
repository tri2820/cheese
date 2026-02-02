# Cheese

Pure Go Wayland toolkit and protocol generator.

## Structure

```
./
├── cmd/
│   └── wayland-scanner/     # Protocol scanner (XML → Go)
├── protocols/               # Generated Wayland protocol bindings
│   ├── client/             # Core Wayland protocol
│   ├── xdg-shell/          # XDG shell (windows)
│   ├── layer-shell/        # Layer shell (bars, backgrounds)
│   └── ...
├── toolkit/                # High-level Wayland client toolkit
│   ├── display/
│   ├── surface/
│   ├── buffer/
│   └── shell/
└── examples/
    ├── rainbow/                # xdg shell example
    ├── statusbar/                # wlr shell example
    └── vk-dmabuf/                # GPU rendering

```

## Goals

- **Pure Go** - No CGO dependencies
- **Clean API** - Idiomatic Go interfaces
- **Layer Shell** - First-class support for bars/panels
- **Maintainable** - Simple, readable code

## Development

```bash
nix develop
go run ./cmd/wayland-scanner -i protocol.xml -o output.go
```

### Generated Protocols

See [protocols/README.md](protocols/README.md) for full list.
