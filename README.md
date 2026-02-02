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
    └── bar/                # Simple bar example

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

## Current Status

✅ **Scanner implemented** - Generates clean Go code from Wayland XML
✅ **52 protocols generated** - All standard Wayland protocols
🚧 **Runtime implementation** - In progress (encoding/decoding)
⏳ **Toolkit** - Not started
⏳ **Layer shell** - Needs wlr-protocols

### Generated Protocols

- **Stable**: 4 protocols (xdg-shell, viewporter, presentation-time, tablet)
- **Staging**: 29 protocols (fractional-scale, cursor-shape, ext-session-lock, etc.)
- **Unstable**: 19 protocols (pointer-gestures, text-input, xdg-decoration, etc.)

See [protocols/README.md](protocols/README.md) for full list.
