# UI Concepts

## Widget
Virtual coordinate space for contents. Independent of any output.

```go
widget.Width = 600   // 96 DPI units (scales with display DPI)
widget.Height = 600
```

## Mask
A "portal" showing a portion of a widget on one output.

| Knob | Controls | Units |
|------|----------|-------|
| `ClipX/Y` | Visible region origin | Widget coordinates |
| `Width()/Height()` | Visible region size | Physical pixels |
| `Left/Top/Right/Bottom` | Position on output | Physical pixels |

**Example:** `ClipX=100, ClipY=50` with `Width()=200, Height()=300` shows `widget[100:300, 50:350]`.

## Affine Transform
Maps widget coordinates to framebuffer:

```
framebuffer = (widget × Scale) - Offset
```

Where `Scale = dpi/96.0` and `Offset = (ClipX, ClipY) × Scale`.

## Example: Portal
```go
// Widget: 600×600 virtual canvas
widget.Width = 600
widget.Height = 600

// Mask 1: Shows left 180 units at right edge
mask.ClipX = 0
mask.ClipY = 0
ui.Eq(mask.Width(), 180).Add()  // Physical pixels
ui.Eq(mask.Right, monitorWidth).Add()

// Mask 2: Shows right 420 units at left edge
mask.ClipX = 180
mask.ClipY = 0
ui.Eq(mask.Width(), 420).Add()
ui.Eq(mask.Left, 0).Add()
```
