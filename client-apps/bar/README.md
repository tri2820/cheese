# Client Bar

Minimal Wayland layer-shell bar built on `client-toolkit`.

Current behavior:
- creates one bar instance per ready output
- binds each bar to its output with layer-shell
- renders a solid black bar with a centered white clock
- tracks output hotplug through `display.OnOutput(...)`

Run:

```bash
cd /home/tri/cheese/client-apps/bar
go run .
```

## TODO

Roadmap ideas for turning this into a more complete daily-driver bar:

- add a tray area
- add backlight status/control for laptop panels
- add temperature display
- add idle inhibitor toggle/status
- add bluetooth status
- add microphone mute indicator
- add keyboard/input-method indicator
- add battery status
- add network status
- add audio volume status
- add a real media/Spotify module
- add workspace and task/taskbar modules
- split the bar into left / center / right module regions instead of a single centered clock
- move styling and sizing into config instead of hardcoding the render layout
- add click handlers for interactive modules
- add font selection instead of relying on `basicfont`
- make per-output bar behavior configurable
