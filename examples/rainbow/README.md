# Cheese Rainbow Bar

An animated gradient bar demonstrating the cheese Wayland library with real window creation and rendering.

## What it does

This example creates a fully functional Wayland application that:
1. Connects to the Wayland compositor
2. Binds to required global interfaces (compositor, shm, xdg_wm_base)
3. Creates an 800x60 pixel bar window using xdg-shell
4. Sets up shared memory (SHM) buffers for rendering
5. Implements double-buffering for smooth animation
6. Renders an animated rainbow gradient that scrolls horizontally
7. Uses frame callbacks for synchronized rendering
8. Handles window close events properly

## Features Demonstrated

- **Connection Management**: `client.Connect()` and proper cleanup
- **Registry Binding**: Discovering and binding global interfaces
- **Event Handlers**: Setting up handlers for multiple event types
  - Display errors
  - WM base ping/pong
  - Surface configuration
  - Toplevel events (close, configure)
  - Frame callbacks for animation
  - Buffer release events
- **Shared Memory**: Creating anonymous files and memory-mapping
- **Double Buffering**: Switching between two buffers to avoid tearing
- **xdg-shell Protocol**: Creating proper desktop windows
- **Frame Synchronization**: Using frame callbacks for smooth animation

## Running

Make sure you're in a Wayland session (not X11):

```bash
go build
./rainbow
```

A colorful animated bar window will appear. Close it normally to exit.

## Expected Output

```
Starting Cheese Rainbow Bar...
Rainbow bar is running! Close the window to exit.
```

You should see an 800x60 pixel window with a scrolling rainbow gradient animation.

## Code Structure

### Display Setup
- `createDisplay()`: Establishes connection and binds global interfaces
- Uses sync callbacks for proper roundtrip handling
- Validates required interfaces (compositor, shm, xdg_wm_base)

### Window Creation
- `createWindow()`: Creates xdg-shell toplevel window
- Sets window title and app ID
- Waits for initial configure event before rendering

### Rendering
- `createShmBuffer()`: Allocates shared memory and creates wl_buffer
- `paintPixels()`: Renders animated gradient using time parameter
- `redraw()`: Frame callback handler that triggers next frame
- Double-buffering prevents tearing

### Event Loop
- Main loop calls `Dispatch()` to process all Wayland events
- Frame callbacks trigger redraw for smooth animation
- Handlers respond to window manager events

## Implementation Notes

This example closely mirrors the structure of a traditional Wayland C client:
- Manual buffer management with release events
- Proper configure/ack_configure protocol
- Frame callback chain for animation
- Explicit damage reporting

The code demonstrates that the cheese library provides a complete, production-ready Wayland client API comparable to go-wayland or wlroots clients.
