# Widget Architecture

## Overview

This document describes the widget architecture for the Cheese UI framework, focusing on multi-monitor support in Wayland.

## Wayland Shell Comparison

### XDG Shell (Managed Windows)

- **Per widget:** 1 surface → 1 XDG surface → 1 frame
- **Placement:** Compositor decides which monitor and position
- **Use case:** Floating windows, popups, managed applications

**Example: 2 widgets, any number of monitors**
```
Widget 1: 1 surface (compositor handles placement)
Widget 2: 1 surface (compositor handles placement)
Total: 2 surfaces
```

### Layer Shell (Always-on-Top)

- **Per widget:** m surfaces → m layers → m frames (where m = number of monitors)
- **Placement:** You explicitly bind to each output
- **Use case:** Panels, bars, overlays, status widgets

**Example: 2 widgets across 3 monitors**
```
Widget 1: 3 surfaces (one per monitor)
Widget 2: 3 surfaces (one per monitor)
Total: 6 surfaces
```

**Why?** Layer shell surfaces are always-on-top and you must specify which output they appear on. The compositor doesn't automatically place them across monitors.

## Layer Shell Design Approaches

### 1. Shared Layout Viewport

If your intended item somehow accidentally concides with the global layout (e.g. see [ALIGNED_LEFT_BAR](../ui/cmd/left_bar)):

```
1 Layout (shared scene)
├── Layout Items (shared content)
│   └── Rectangle spanning virtual desktop
└── Frames (per-monitor viewports)
    ├── Frame 1: renders portion visible on Monitor 1
    ├── Frame 2: renders portion visible on Monitor 2
    └── Frame 3: renders portion visible on Monitor 3
```

**How it works:**
- Each frame is a "camera" viewing the same layout from different viewport positions
- Layout items outside a frame's viewport aren't rendered
- Surface size = bar size (e.g., 30px × monitor height)

**Best for:** Single bar/panel spanning multiple monitors

### 2. Per-Widget Surfaces

For scattered floating widgets:

```
Widget 1 (clock at top-left):
├── Monitor 1: surface (50×50)
├── Monitor 2: surface (50×50)
└── Monitor 3: surface (50×50)

Widget 2 (battery at bottom-right):
├── Monitor 1: surface (80×30)
├── Monitor 2: surface (80×30)
└── Monitor 3: surface (80×30)
```

