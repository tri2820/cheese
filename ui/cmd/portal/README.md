# Portal Demo

This example demonstrates the "portal" capability enabled by the Mask+Content architecture - showing the same widget contents on multiple non-contiguous displays.

## What it does

The portal demo creates:
- **One widget** with contents (orange rectangle + "PORTAL" label)
- **Two masks** positioned at opposite edges of two monitors:
  - Mask 0: Right edge of first monitor
  - Mask 1: Left edge of second monitor

The same contents are rendered on both displays, making it appear as if the UI element spans across the gap between monitors.

## Key Architecture Features

### Before: Coupled positioning
```go
rect := widget.NewRectangle()
ui.Eq(rect.Left, outputX).Add()  // Global coordinates
ui.Eq(rect.Top, outputY).Add()
```
Problem: With monitors at (0,0) and (3000,0), creating a rectangle that spans both would create 1920px of wasted coordinate space in between.

### After: Mask+Content separation
```go
// Create contents (coordinate-free)
rect := widget.NewRectangle()
rect.Color.Set("#FF5500")

// Create masks for different outputs
mask0, err := widget.NewMask(output0, config)
if err != nil {
	panic(err)
}
mask0.Own(ui.Eq(mask0.Right, output0Width))  // Surface-local, owned by the mask

mask1, err := widget.NewMask(output1, config)
if err != nil {
	panic(err)
}
mask1.Own(ui.Eq(mask1.Left, 0))  // Surface-local, owned by the mask
```
Solution: Each mask uses surface-local coordinates (0,0 = top-left of that output). No coordinate gaps despite physical gaps.

## Benefits

1. **No coordinate gaps**: Each mask uses its own surface-local coordinate space
2. **Shared contents**: One widget definition, multiple rendering locations
3. **Independent positioning**: Each mask can be positioned differently
4. **Efficient**: Contents are defined once, rendered per-mask

## Running

```bash
cd /home/tri/cheese/ui/cmd/portal
go build .
./portal
```

Requires at least 2 monitors to demonstrate the portal effect.
