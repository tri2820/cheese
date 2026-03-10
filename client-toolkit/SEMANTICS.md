# Client Toolkit Semantics

This document defines the intended semantics and conventions for `client-toolkit`.

It is normative. New APIs should fit this model. Existing APIs that do not fit it
should be treated as design debt and cleaned up, not copied.

## Goal

`client-toolkit` should make Wayland easier to use without becoming a second,
more confusing protocol.

The design target is:

- thin where Wayland is already clear
- explicit where lifecycle is tricky
- predictable in naming and behavior
- composable, so helpers do not fight each other

## Core Perspective

The toolkit is not "one app = one window".

An app owns a Wayland connection and may create many surfaces. A surface is just
a compositor-visible drawing target. Shell objects assign roles to surfaces.
Higher-level helpers may manage rendering for one surface, but they do not define
the shape of the whole app.

The intended mental model is:

1. `display.Display` is the connection, registry, globals, and hotplug view.
2. `surface.Surface` is a bare `wl_surface`.
3. `shell.ToplevelSurface` and `shell.LayerSurface` assign roles to a surface.
4. `shm.Frame` and `gpu.Renderer` manage rendering lifecycle for one render target.
5. Applications compose these pieces and may own multiple render targets.

## Abstraction Levels

The toolkit is layered on purpose. It should not pretend every package is at the
same abstraction level.

### Thin wrappers

These packages stay close to Wayland concepts:

- `display/`
- `surface/`
- `shell/`
- `seat/`

Rules:

- expose Wayland concepts with minimal policy
- keep ownership clear
- do not hide important protocol facts
- do not decide app shutdown or process lifetime

### Resource helpers

These packages manage rendering resources but should still be fairly mechanical:

- `buffer.Pool`
- `buffer.Slot`
- `buffer.Buffer`
- `shm.Swapchain`
- `dmabuf.State`
- `dmabuf.Params`
- `dmabuf.Buffer`
- `dmabuf.BufferInfo`

Rules:

- own buffers and memory
- hide repetitive setup
- avoid unrelated policy

### Lifecycle helpers

These packages are allowed to own more behavior:

- `shm.Frame`
- `gpu.Renderer`

Rules:

- may subscribe to configure/frame events
- may manage render pacing and resize handling
- must still be explicit about what they own
- must compose with app code and other helpers

If an API mixes two abstraction levels, split it instead of making it "smart".

## State vs Events

This toolkit follows a strict split:

- state is pulled
- events are pushed

### State

Current state should come from getters or fields that describe the object now.

Examples:

- `Display.Outputs()`
- `Display.ReadyOutputs()`
- `Frame.Width()`
- `Frame.Height()`
- `Output.Ready`

State access should be side-effect free.

### Events

Event APIs use `OnX`.

`OnX` means all of the following:

- additive: registering one handler does not replace another
- future-only: it does not replay past events
- non-owning: registration does not transfer lifecycle ownership
- explicit: if the event can happen many times, handlers may run many times

`OnX` must not mean "set the one handler". That pattern is banned in toolkit wrappers.

## Replay Rule

Replay semantics must never be hidden behind `OnX`.

If an API needs to immediately deliver current state on subscription, it must use
a different name and document that behavior explicitly.

Allowed examples for future APIs:

- `CurrentX`
- `SnapshotX`
- `WhenReady`
- `SubscribeX` with explicit replay docs

Disallowed:

- `OnX` that sometimes replays and sometimes does not

## Naming Conventions

### Constructors

- use `NewX` for normal constructors
- use `MustX` only for convenience constructors that fail fast during setup
- do not add `MustX` forms casually

### Events

- use `OnX` for additive event subscription
- never use `SetXHandler` in wrapper APIs
- avoid `AddXHandler`; `OnX` is the standard spelling

### Delegates

- use `SetX` for single-owner delegates or callbacks
- `SetX` replaces the previous delegate
- do not use `OnX` for single render or single handler slots

### Lifecycle

- use `Close()` for wrapper-owned cleanup, even if the Wayland protocol verb is
  `destroy` or `release`
- `Close()` should release this object's resources, not terminate the process

### Accessors

- use nouns or clear getters for current state
- avoid ambiguous verbs that hide work or policy

## Ownership Rules

The toolkit must not hide ownership boundaries.

### Process ownership

Toolkit wrappers must not:

- call `os.Exit`
- unilaterally terminate the app on runtime events
- silently swallow close requests that matter to the caller

A close request from the compositor is a signal to the application, not a command
for the toolkit to kill the process.

### Callback ownership

Helpers must never steal callbacks from callers.

If a helper needs to observe events internally, it must subscribe through the
same additive `OnX` APIs available to the caller. Internal subscriptions and app
subscriptions must compose.

### Goroutine ownership

If an API starts goroutines, timers, or background loops, that must be obvious
from the type's role and documentation.

Thin wrappers should generally not start background work.

### Error ownership

Lifecycle helpers should surface runtime failures through explicit APIs such as
`OnError(...)`.

Toolkit helpers should not print errors directly to stdout or stderr as their
primary error-reporting mechanism.

## Hotplug and Output Semantics

Output handling must follow these rules:

- output "added" means the output became ready for the first time
- later `wl_output.done` events are updates, not re-adds
- output "removed" means the registry global was removed
- current output state is available via `Outputs()` and `ReadyOutputs()`

`Output` itself may emit update events like geometry, scale, or done repeatedly.
That does not change the meaning of the add/remove stream.

## Render Helper Semantics

`shm.Frame` and `gpu.Renderer` are lifecycle helpers, not passive wrappers.

That means they may:

- react to configure events
- request frame callbacks
- own buffer recreation on resize

That also means they must not:

- override app event handlers
- hide resize or pacing behavior behind ambiguous names
- pretend to be thin wrappers

If a helper owns a render loop, its API should read like a lifecycle helper.

## Package Boundary Rule

If a type mainly exists to mirror a protocol object, keep it thin.

If a type mainly exists to own policy, buffering, scheduling, or coordination,
move it to a higher-level package and make that ownership explicit.

Do not make low-level packages "magical" just to save a few lines in examples.

## API Review Checklist

When adding or changing an API, ask:

1. What abstraction level is this type at?
2. Is this method exposing state, or subscribing to events?
3. If it is an event API, is it additive and future-only?
4. Does this helper compose with other helpers and app code?
5. Does any runtime event here incorrectly control process lifetime?
6. Is the naming obvious without reading the implementation?
7. Would a developer guess the behavior correctly from the name alone?

If the answer to 3, 4, 5, or 7 is no, redesign it before merging.

## Current Direction

The current preferred style is:

- `Display.OnOutput(...)`
- `Display.OnError(...)`
- `Output.OnGeometry(...)`
- `Output.OnDone(...)`
- `Output.OnReady(...)`
- `Surface.OnFrame(...)`
- `Surface.OnEnter(...)`
- `Surface.OnLeave(...)`
- `Frame.OnResize(...)`
- `Frame.OnError(...)`
- `Frame.SetRender(...)`
- `Frame.SetManualMode(...)`
- `Frame.ManualRender(...)`
- `ToplevelSurface.OnConfigure(...)`
- `ToplevelSurface.OnClose(...)`
- `LayerSurface.OnConfigure(...)`
- `LayerSurface.OnClose(...)`
- `Renderer.OnResize(...)`
- `Renderer.OnError(...)`
- `Renderer.SetManualMode(...)`
- `Renderer.ManualRender(...)`
- `Pointer.OnEnter(...)`
- `Pointer.OnLeave(...)`

This is the model future cleanup should reinforce.
