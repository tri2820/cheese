# UI Toolkit - Constraint-Based Reactive Layout for Wayland

A reactive UI toolkit for Go combining **signals** (fine-grained reactivity) with **constraint solving** (casso) for automatic layout.

## Core Philosophy

- **Constraint-based layout** - Declare relationships between elements, solver computes positions
- **Reactive signals** - Changes propagate automatically through dependency graph
- **Retained mode** - Objects exist in scene graph, update in place
- **Thread-safe** - RWMutex protects concurrent access
- **Explicit dependencies** - Compute/Effect take deps explicitly, no magic tracking

## Architecture

```
Signal[T] (reactive inputs)
         ↓
   Cassowary Solver (constraint solving)
         ↓
       Computed Layout (positions, sizes)
         ↓
       Scene (rendering, events)
```

## Signals (Reactivity)

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
func Effect(fn func(), deps ...Dep)
```

## Constraint Solver (Layout)

Uses [casso](https://github.com/lithdew/casso) - a Cassowary constraint solver:

```go
import "github.com/lithdew/casso"

solver := casso.NewSolver()

// Create variables for layout properties
width := casso.New()
height := casso.New()
x := casso.New()
y := casso.New()

// Add constraints:
// - Equality: width == 100
// - Relationship: x >= parent.X + 10
// - Ratios: width == 0.5 * parent.Width
// - Minimums: width >= 50 (with priority)

c1 := casso.NewConstraint(casso.EQ, 0, width.T(1.0), casso.T(-100))
c2 := casso.NewConstraint(casso.GTE, -50, width.T(1.0))

solver.AddConstraint(c1)
solver.AddConstraintWithPriority(casso.Weak, c2)

// Mark editable variables and suggest values
solver.Edit(width, casso.Strong)
solver.Suggest(width, 200)

// Read computed values
finalWidth := solver.Val(width)
```

## Combined: Reactive Constraints

```go
// LayoutState holds computed layout values
type LayoutState struct {
    X, Y, Width, Height float64
}

// Solver encapsulates constraint system
type Solver struct {
    solver *casso.Solver
    x, y, width, height casso.Symbol
}

// Create a signal for container width
containerWidth := signals.New(1024.0)

// Computed signal - auto-recalculates layout when containerWidth changes
layout := signals.Derive(func() LayoutState {
    w := containerWidth.Get()
    solver.Suggest(solver.width, w)
    return LayoutState{
        Width:  solver.Val(solver.width),
        X:      solver.Val(solver.x),
        // ...
    }
}, containerWidth)

// Subscribe to layout changes
layout.Subscribe(func(state LayoutState) {
    // Re-render with new layout
    scene.MarkDirty()
})

// Update container - layout automatically recomputes
containerWidth.Set(2048.0)
```

## Constraint Types

| Operator | Description | Example |
|----------|-------------|---------|
| `casso.EQ` | Equality | `width == 100` |
| `casso.GTE` | Greater than or equal | `width >= 50` |
| `casso.LTE` | Less than or equal | `width <= 500` |

## Constraint Priorities

- **Required** (`casso.Required`) - Must be satisfied
- **Strong** (`casso.Strong`) - High priority
- **Weak** (`casso.Weak`) - Low priority, can be violated

Used for constraints with trade-offs (e.g., "prefer this size but allow shrinking")

## Layout Examples

### Fixed Size
```go
width := casso.New()
height := casso.New()

solver.AddConstraint(
    casso.NewConstraint(casso.EQ, 0, width.T(1.0), casso.T(-100))
)
solver.AddConstraint(
    casso.NewConstraint(casso.EQ, 0, height.T(1.0), casso.T(-50))
)
```

### Relative to Parent
```go
// childWidth == 0.5 * parentWidth
childWidth.T(1.0), parentWidth.T(-0.5)

// childX == parentX + 10
childX.T(1.0), parentX.T(-1.0), casso.T(-10)
```

### Center Alignment
```go
// childX + childWidth/2 == parentX + parentWidth/2
casso.NewConstraint(
    casso.EQ, 0,
    childX.T(1.0), childWidth.T(0.5),
    parentX.T(-1.0), parentWidth.T(-0.5),
)
```

### Chaining Elements
```go
// nextX == prevX + prevWidth + gap
casso.NewConstraint(
    casso.EQ, -gap,
    nextX.T(1.0), prevX.T(-1.0), prevWidth.T(-1.0),
)
```

## File Structure

```
ui-kit/
├── kit.go                 # Package exports
├── go.mod
├── cmd/
│   ├── basic/             # Basic casso solver demo
│   │   └── main.go
│   └── reactive/          # Reactive + constraints demo
│       └── main.go
└── lib/                   # (future)
    ├── solver.go          # Constraint solver wrapper
    ├── signals.go         # Signal integration
    └── layout.go          # Layout abstractions
```

## Usage Examples

### Basic Constraint Solving
```bash
go run ./cmd/basic
```

Demonstrates pure casso constraint solving:
- Editable variable (`containerWidth`)
- Multiple constraints with different priorities
- Suggesting values and reading computed results

### Reactive Constraints
```bash
go run ./cmd/reactive
```

Demonstrates signals + constraints:
- Signal drives constraint solver
- `Compute()` creates reactive layout
- `Subscribe()` reacts to layout changes
- Derived computed signals

## Implementation Phases

1. **✓ Signals** - Signal[T], Dep interface, New, Compute, Effect (thread-safe)
2. **✓ Constraints** - Cassowary solver integration (casso)
3. **✓ Reactive Layout** - Signals driving constraint solver
4. **Scene** - Object interface, hit testing, rendering
5. **Objects** - Label, Button, Box with constraint-based layout
6. **Events** - Pointer handlers, dispatch
7. **Integration** - Wayland surface, render loop
