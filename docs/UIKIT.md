# UI Toolkit - SolidJS-Style Retained Mode for Wayland

A reactive UI toolkit for Go with fine-grained reactivity, built on Wayland.

## Core Philosophy

- **Retained mode** - Objects exist in scene graph, update in place
- **Fine-grained reactivity** - Signals track dependencies, only affected parts re-render
- **Thread-safe** - RWMutex protects concurrent access
- **Explicit dependencies** - Derive/Effect take deps explicitly, no magic tracking
- **Performant** - Bounding box hit testing, dirty tracking

## Architecture

```
Signal[T] / Derive  (reactive core)
         ↓
       Object       (retained scene graph objects)
         ↓
       Scene        (hit testing, rendering, events)
```

## Object Interface

```go
type Object interface {
    // Rendering
    Draw(dst *image.RGBA)
    Bounds() image.Rectangle
    Children() []Object

    // Layout
    Measure(constraints Constraints) Size
    Layout(bounds Rect)

    // Events
    SetOnClick(func())
    SetOnEnter(func())
    SetOnLeave(func())
}
```

## Built-in Objects

- **Label** - Text display
- **Button** - Clickable with label
- **Box** - Container (row/column layout)
- **Image** - Image display

**Component pattern:** Objects accept `Dep` (signals) for reactive props, use `Derive()` for computed values.

```go
// Dep interface that all signals implement
type Dep interface {
    OnChange(fn func())
}

type Signal[T any] struct {
    value       T
    subscribers []func(T)
    mu          sync.RWMutex  // Thread-safe
}

func (s *Signal[T]) Get() T
func (s *Signal[T]) Set(v T)

// New creates a source signal with an initial value
func New[T any](value T) *Signal[T]

// Compute creates a signal computed from dependencies
func Compute[T any](fn func() T, deps ...Dep) *Signal[T]

// Effect runs a side effect when dependencies change
// Runs immediately, then on each dependency change
func Effect(fn func(), deps ...Dep)
```

## Layout

Simple constraint-based layout:

```go
type Constraints struct {
    MinWidth, MaxWidth   float64
    MinHeight, MaxHeight float64
}

type BoxProps struct {
    Direction Row | Column
    Gap       float64
    Align     Start | Center | End
    Children  []Object
}
```

## Styling

```go
type Style struct {
    Background   *Signal[color.Color]
    Foreground   *Signal[color.Color]
    Opacity      *Signal[float64]
    Padding      *Signal[Insets]
    Border       *Signal[Border]
    CornerRadius *Signal[float64]
    Width, Height *Signal[float64]
}
```

## Events

- **Pointer** - Click, enter/leave, motion
- **Keyboard** - Focus, key events
- **Drag** - Drag and drop

Hit testing: Bounding box (AABB), scene graph walk top-to-bottom.

## File Structure

```
client-toolkit/ui/
├── signals/
│   ├── signal.go      # Signal[T], Dep interface, New, Compute, Deps, Effect
├── scene/
│   ├── object.go      # Object interface, BaseObject
│   ├── scene.go       # Scene, hit testing
│   └── context.go     # Build context
├── objects/
│   ├── label.go
│   ├── button.go
│   ├── box.go
│   └── image.go
├── layout/
│   ├── constraints.go # Constraints, Size
│   └── box.go         # Box layout
├── style/
│   └── style.go       # Style struct
└── events/
    └── pointer.go     # Hit testing, dispatch
```

## Usage Example

```go
// Create source signals
count := New(0)
prefix := New("Count: ")

// Computed signal - auto-updates when dependencies change
labelText := Compute(func() string {
    return prefix.Get() + fmt.Sprint(count.Get())
}, count, prefix)

// Effect for side effects (logging, rendering, etc.)
Effect(func() {
    fmt.Println("Label changed:", labelText.Get())
    scene.MarkDirty()  // Request re-render
}, labelText)

// Subscribe to value changes
labelText.Subscribe(func(v string) {
    fmt.Println("Got:", v)
})

btn := NewButton(ButtonProps{
    Label: NewLabel(LabelProps{
        Text: labelText,
    }),
    OnClick: func() {
        count.Set(count.Get() + 1)
    },
})

scene.Add(btn)
```

**Patterns:**
- `New()` - Create source signals with initial values (type inferred)
- `Compute()` - Create computed signals that auto-update when dependencies change
- `Deps()` - Group multiple dependencies into a single signal for batch tracking
- `Effect()` - Run side effects when dependencies change (runs immediately + on changes)
- `Subscribe()` - Get notified of value changes, called immediately with current value

## Implementation Phases

1. **Signals** - Signal[T], Dep interface, New, Compute, Deps, Effect, Subscribe (thread-safe) ✓
2. **Scene** - Object interface, BaseObject, hit testing
3. **Objects** - Label, Button, Box
4. **Layout** - Constraints, box layout
5. **Events** - Pointer handlers, dispatch
6. **Integration** - Wayland surface, render loop
