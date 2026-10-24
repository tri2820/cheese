# Client Bar

Wayland status bar built on `client-toolkit` (layer-shell + SHM). One bar per
ready output; the dock is a separate surface.

## Layout

- `rightModules` (right-aligned): input-method, wifi, Control Center, and the clock
  (rightmost; click opens the notification flyout).
- **Control Center** (`module_control_center.go` + `control_center_flyout.go`) merges
  mic, volume and bluetooth into one dropdown:
  - a **circular Bluetooth button** (click = toggle power, animated on/off background);
    "Bluetooth ▾" on the right expands a full device list (Connected Devices / Devices
    sections with checkmarks, plus "Bluetooth Settings…"). Opening the flyout starts a
    BlueZ discovery scan (`StartDiscovery`, stopped on close) so connectable/unpaired
    devices show up;
  - a **circular Microphone button** (click = toggle mute, animated background);
  - a **volume slider** — speaker in a rounded-square button shown **on hover** (click =
    mute), a big rounded fill, and the thumb only shows on hover (animated);
  - a **brightness slider** per display: the **first** slider is always the monitor backing
that bar's output (built-in panel on the laptop, the external once the bar moves there). A
**chevron** at its right end expands the **other** monitors — one identical pill slider each;
hidden when there is only one display, so the list never repeats the first slider. Each
display's name is drawn dimmed *inside its own track* (right of the sun glyph): hover-only
when collapsed, and shown for **every** slider while the list is expanded. The sun glyph is
decorative — no hit target, no hover button chrome (only the volume pill's speaker glyph is a
button, mute) — and the whole track drags, chevron area aside.
  - a divider separates the Bluetooth segment from the mic/volume segment.
- Each flyout is its own layer-shell surface anchored under its module:
  `notification_flyout.go`, `wifi_flyout.go`, `control_center_flyout.go`.
- `dock.go`: magnification dock for the active workspace's windows. Auto-hide leaves a
  **full-width** 2px hover strip at the bottom screen edge (`dockTriggerRect`), so the dock
  reveals from anywhere along the bottom, not just above its own hidden pill. While the dock is
  visible that strip stays part of its input region (region = pill rect **+** strip), otherwise
  the region collapses to the pill and a pointer resting on the bottom edge leaves it
  immediately — hiding the dock the instant it appears.
- `module_mic.go`, `module_volume.go`, `module_bluetooth.go`, `volume_flyout.go`,
  `bluetooth_flyout.go` are now **unused** (superseded by the Control Center). Kept for
  now because shared helpers live inside them (`drawSmoothCircle` in `volume_flyout.go`,
  `drawDivider` in `wifi_flyout.go`) — prune once those move to a shared `draw_helpers.go`.

## Services

`*_service.go` expose `Subscribe(fn)` plus a cached `State()`/getter. State is
refreshed on the service's own goroutine (signals from D-Bus / PulseAudio /
`niri msg event-stream`), never on the caller's thread. `Subscribe` pushes the
current state immediately and on every change.

`BrightnessService` (`brightness_service.go`) is the exception that owns **hardware**
instead of D-Bus state: it enumerates the internal panel from
`/sys/class/backlight/*` and every connected external connector from
`/sys/class/drm/card*-*/ddc`, reads the external levels over raw DDC/CI
(VCP `0x10`, i2c slave ioctl — no `ddcutil` dependency) and writes them the same
way. Internal writes go through logind (`Session.SetBrightness` on
`/org/freedesktop/login1/session/auto`) because `/sys/class/backlight/*/brightness`
is root-only here; `brightnessctl` is only a fallback. Writes are coalesced on a
single writer goroutine (latest value per display wins) so dragging a slider never
floods D-Bus/i2c, and all i2c transactions are serialized behind one mutex. Labels
come from EDID (`AOC 24B2W1G5`); the panel is always "Built-in Display".

## Build & run (NixOS)

`go` is not on PATH here — use the flake dev shell.

```bash
cd ~/Projects/cheese/client-apps/bar
direnv exec . env CGO_ENABLED=0 go build -o ~/.local/bin/cheese-bar .
pkill -x cheese-bar   # the niri autostart loop relaunches it
```

The running binary is `~/.local/bin/cheese-bar`, spawned at startup from
`~/.config/niri/config.kdl` inside a `while true; do …; sleep 1; done` loop.

## Gotchas (learned the hard way)

- **Rebuild or it's stale.** Editing source changes nothing until you rebuild
  *and* restart — the bar you're looking at is a binary. Check its mtime against
  the source's before believing a fix works (cost a full "fix doesn't work" round).
- **Build static.** Use `CGO_ENABLED=0`; a plain `go build` produces a
  cgo/dynamic binary that mismatches the system glibc. Verify with
  `ldd ~/.local/bin/cheese-bar` → `not a dynamic executable`. (`file` isn't
  installed on this machine.)
- **Restart with `pkill -x cheese-bar`** (exact process name). `pkill -f
  cheese-bar` also matches the `sh -c while true …` wrapper and kills autostart.
- **Flyouts must fit content.** Create each flyout layer with the computed height
  (`computeContentAndSurfaceHeight`) and add a `SetSize`-on-change loop in the
  flyout goroutine — `notification_flyout.go` is the reference. Don't hardcode the
  max height. After `SetSize` + `Commit` the compositor sends a configure that
  re-renders the frame, so set `pending = false; continue` rather than rendering
  the stale buffer.
- **Never block the main thread.** Constructors and all seat/pointer callbacks run
  on the Wayland dispatch thread (blocking there freezes every output). Run the
  initial service `refresh()` inside the service goroutine, make flyout `Open`
  read cached state (do D-Bus/exec on the flyout goroutine), and wrap
  user-triggered `exec`/D-Bus work (bluetoothctl, nmcli, pulse) in `go func()`.

## Known limitations

- **Auto-opened notifications don't grab input.** A notification-triggered flyout (a
  new notification arriving on its own) has no click-catcher at all — a stray click
  can't dismiss it and it doesn't block the windows below; it auto-hides on its
  timer. A user-triggered flyout (clicking the clock) keeps click-outside-to-dismiss
  and the input grab below. The flyout is also right-clamped so it doesn't overflow
  under the right-side clock.
- **An open flyout blocks pointer input to the windows below** — you can't scroll
  the browser until you dismiss it. Each flyout creates a full-screen transparent
  layer-surface "backdrop" on the `top` layer to catch click-outside-to-dismiss;
  sitting above the windows with a full input region, it also swallows their
  scroll/clicks. This is a **Wayland constraint, not a fixable bug**: pointer focus
  is per-surface and delivered in full (motion + buttons + axis) to the single
  topmost surface under the cursor whose *input region* contains it; input regions
  are geometric (no per-event-type filter); and a client can't observe clicks on
  another client's surface. So "catch a click anywhere but let scroll pass
  through" is impossible.
- **Considered and rejected (2026-10-16):** drop the backdrop and dismiss only via
  the bar's own click handler (`Bar.HandlePointerButton` already closes an open
  flyout when you click the bar or another module) plus the module toggle. That
  keeps windows interactive but loses click-anywhere-to-dismiss. Decision: keep
  the backdrop and accept the limitation.
