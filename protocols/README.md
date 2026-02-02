# Cheese Desktop Environment

Generated Wayland protocol bindings in pure Go.

```go
import "github.com/tri2820/cheese/protocols/client"
import "github.com/tri2820/cheese/protocols/xdg_shell"

// Create context
ctx := client.NewContext(conn)

// Create XDG shell
wmBase := xdg_shell.NewXdgWmBase(ctx)
```

## Type Resolution

| Scenario | Type |
|----------|------|
| Core Wayland (wl_*) | `*client.WlDisplay`, `*client.WlSurface`, etc. |
| Same protocol | `*ZxdgSurfaceV6` (concrete type) |
| Different protocol | `client.Proxy` (interface) |

## Regenerating

```bash
./generate.sh
```

Requires: wayland-protocols, wayland.xml (from nix develop)

## Status

✅ **Working: All 58 protocols generated and compiling**

Each protocol is independently usable with no cross-package dependencies (except the client package for core types).
