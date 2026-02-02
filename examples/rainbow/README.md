# Rainbow Example

A minimal example demonstrating the cheese Wayland library.

## What it does

This example:
1. Connects to a Wayland compositor
2. Retrieves the global registry
3. Sets up event handlers for global interface announcements
4. Dispatches events to list all available Wayland interfaces
5. Cleanly disconnects

## Running

Make sure you're in a Wayland session:

```bash
go build
./rainbow
```

## Expected Output

```
Connected to Wayland compositor!

Available Wayland interfaces:
Global interface: wl_compositor (name=1, version=6)
Global interface: wl_subcompositor (name=2, version=1)
Global interface: xdg_wm_base (name=3, version=6)
...

Successfully listed Wayland interfaces!
```

## Code Highlights

The example demonstrates the key features of the cheese library:

- **Connection**: `client.Connect("")` establishes a connection
- **Event Handlers**: `registry.SetGlobalHandler(func(ev client.WlRegistryGlobalEvent) { ... })`
- **Event Loop**: `display.Context().Dispatch()` processes events
- **Cleanup**: `defer display.Context().Close()`
