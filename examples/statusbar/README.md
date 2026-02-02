# Cheese Status Bar (Waybar-like)

A waybar-style status bar demonstrating the wlr-layer-shell protocol. This creates a bar that sticks to the top of your screen, just like waybar, polybar, or other Wayland status bars.

## What it does

This example creates a status bar using wlr-layer-shell that:
1. Connects to the Wayland compositor
2. Binds to `zwlr_layer_shell_v1` (wlroots layer shell protocol)
3. Creates a layer surface anchored to the top of the screen
4. Reserves exclusive space (pushes windows down)
5. Renders an animated gradient and placeholder for time display
6. Demonstrates proper layer shell configuration

## Key Difference from Regular Windows

**Regular windows (xdg-shell):**
- Float above other windows
- Can be moved, resized, minimized
- Don't reserve screen space

**Layer shell surfaces (wlr-layer-shell):**
- Can anchor to screen edges (top/bottom/left/right)
- Can reserve exclusive zones (push other windows away)
- Choose layer (background/bottom/top/overlay)
- Perfect for panels, bars, docks, and widgets

## Features Demonstrated

- **wlr-layer-shell Protocol**: The key protocol for status bars
- **Layer Configuration**: Setting layer (top), anchor (top+left+right), and exclusive zone
- **Full-width Bar**: Width=0 means "full screen width"
- **Exclusive Zone**: Prevents windows from overlapping the bar
- **Compositor Integration**: Works with Sway, Hyprland, River, etc.

## Requirements

**Compositor Support**: Your compositor must support `wlr-layer-shell-unstable-v1`:
- ✅ Sway
- ✅ Hyprland
- ✅ River
- ✅ Wayfire
- ✅ Most wlroots-based compositors
- ❌ Mutter/GNOME (doesn't support layer shell)
- ❌ KWin/KDE (partial support)

## Running

```bash
go build
./statusbar
```

You should see a 30-pixel tall bar appear at the top of your screen with an animated gradient.

## Expected Output

```
Starting Cheese Status Bar (waybar-like)...
Bound to wlr-layer-shell
Status bar is running! It should appear at the top of your screen.
Press Ctrl+C to exit.
Layer surface configure: serial=517691, width=1366, height=30
```

The bar will appear at the top of your screen, and other windows will be positioned below it.

## Code Structure

### Layer Shell Setup

```go
// Create layer surface (not xdg_surface!)
layerSurf, _ := layerShell.GetLayerSurface(
    surface,
    output,  // nil = any output
    uint32(ZwlrLayerShellV1LayerTop),
    "statusbar",  // namespace
)

// Configure anchoring
anchor := ZwlrLayerSurfaceV1AnchorTop |
          ZwlrLayerSurfaceV1AnchorLeft |
          ZwlrLayerSurfaceV1AnchorRight

layerSurf.SetAnchor(uint32(anchor))
layerSurf.SetSize(0, 30)  // width=0 means full screen width
layerSurf.SetExclusiveZone(30)  // Reserve 30 pixels
```

### Layer Options

**Layers (from bottom to top):**
- `LayerBackground` - Desktop wallpaper
- `LayerBottom` - Below windows
- `LayerTop` - Above windows (typical for bars)
- `LayerOverlay` - Above everything (notifications, OSD)

**Anchors:**
- `AnchorTop`, `AnchorBottom`, `AnchorLeft`, `AnchorRight`
- Combine with `|` for edges (e.g., Top+Left+Right = full-width top bar)

**Exclusive Zone:**
- Positive number: Reserve that many pixels (pushes windows away)
- 0: Don't reserve space (float over windows)
- -1: Auto-calculate based on size

## Comparison to Waybar

Waybar uses the exact same protocol! This example demonstrates all the core concepts needed to build a full-featured status bar like waybar:

| Feature | Waybar | Cheese Status Bar |
|---------|--------|-------------------|
| Protocol | wlr-layer-shell | ✅ wlr-layer-shell |
| Top anchoring | ✅ | ✅ |
| Exclusive zone | ✅ | ✅ |
| Multi-monitor | ✅ | ✅ (via output param) |
| Double buffering | ✅ | ✅ |
| Frame callbacks | ✅ | ✅ |

To build a full status bar like waybar, you'd add:
- Text rendering (freetype/harfbuzz)
- Module system (time, battery, CPU, etc.)
- Click handling (wl_seat/pointer events)
- Configuration file parsing
- Style/theme support

But the core Wayland integration is all here!

## Next Steps

From this base, you could build:
- **Full status bar** - Add modules for system info
- **Notification daemon** - Use LayerOverlay
- **Dock** - Anchor to bottom or sides
- **Desktop widgets** - Float anywhere with LayerBackground
- **Screen lock** - Use LayerOverlay with keyboard grab

All using the same wlr-layer-shell protocol!

## Protocol Information

- **Protocol**: `zwlr_layer_shell_unstable_v1`
- **XML**: `wlr-layer-shell-unstable-v1.xml`
- **Spec**: https://wayland.app/protocols/wlr-layer-shell-unstable-v1
- **wlroots**: Part of wlroots library, used by many compositors
