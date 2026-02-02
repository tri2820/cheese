# Cheese Examples

Complete, working examples demonstrating the cheese Wayland library.

## Examples

### 1. Rainbow Bar (`rainbow/`)
**An animated gradient window using xdg-shell**

- Standard desktop window
- Animated rainbow gradient
- Double-buffered rendering
- Frame callbacks
- ~360 lines of code

Perfect for learning:
- Window creation with xdg-shell
- Shared memory rendering
- Animation loops
- Event handling basics

### 2. Status Bar (`statusbar/`)
**A waybar-like bar using wlr-layer-shell**

- Layer shell surface (not a regular window!)
- Anchored to top of screen
- Exclusive zone (pushes windows down)
- Full screen width
- Animated rendering

Perfect for learning:
- wlr-layer-shell protocol
- Status bars / panels
- Desktop integration
- Layer management

## Quick Start

```bash
# Rainbow bar (xdg-shell window)
cd rainbow
go build && ./rainbow

# Status bar (layer-shell panel)
cd statusbar
go build && ./statusbar
```

## What You Can Build

With these examples as a base, you can create:

### Desktop Applications
- Regular GUI apps (use rainbow example)
- Multi-window applications
- Dialog boxes
- Popup menus

### Desktop Environment Components
- **Status bars** like waybar (use statusbar example)
- **Docks** like plank (layer-shell + bottom anchor)
- **App launchers** like rofi (layer-shell + overlay)
- **Widgets** (layer-shell + background)
- **Notifications** (layer-shell + overlay)
- **Screen lockers** (layer-shell + keyboard grab)

### System Tools
- Screen recorders (wlr-screencopy)
- Display configuration (wlr-output-management)
- Color temperature (wlr-gamma-control)
- Window managers
- Compositors (advanced)

## Protocol Coverage

The cheese library provides complete bindings for:
- ✅ 58 standard Wayland protocols
- ✅ wlroots protocols (layer-shell, etc.)
- ✅ All generated from XML (not hand-written)
- ✅ Type-safe event handlers
- ✅ Clean, go-wayland-like API

## Key Features Demonstrated

Both examples show:
- Connection management
- Registry binding
- Global discovery
- Shared memory
- Event handlers
- Frame callbacks
- Buffer management
- Error handling

**Rainbow** adds:
- xdg-shell protocol
- Window configuration
- Toplevel management

**Status bar** adds:
- wlr-layer-shell protocol
- Layer configuration
- Anchoring
- Exclusive zones

## Documentation

Each example includes:
- Detailed README.md
- Code comments
- Usage instructions
- Protocol explanations

## Complexity

| Example | Lines | Protocols | Level |
|---------|-------|-----------|-------|
| Rainbow | ~360 | xdg-shell | Beginner |
| Status bar | ~370 | wlr-layer-shell | Intermediate |

Both are simple enough to understand but complete enough to be useful!

## Next Steps

1. **Study the code** - Both examples are well-commented
2. **Modify and experiment** - Change colors, sizes, positions
3. **Add features** - Text rendering, input handling, modules
4. **Build something new** - Use these as templates

The cheese library gives you everything you need to build real Wayland applications!
