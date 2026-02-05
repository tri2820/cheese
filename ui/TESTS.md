# UI Kit Test Scripts

This directory contains test scripts demonstrating various features of the UI constraint layout system.

## Test Scripts

### 1. test_fluent
**Tests the fluent `.Add()` API for constraint addition.**

```bash
go run cmd/test_fluent/main.go
```

Demonstrates:
- Standalone constraint functions: `ui.Eq(...).Add()`
- Element methods: `child.Inside(parent).Add()`
- Priority chaining: `ui.Between(...).IsWeak().Add()`

### 2. test_priority
**Tests constraint priority levels and conflict resolution.**

```bash
go run cmd/test_priority/main.go
```

Demonstrates:
- Required priority (must be satisfied)
- Strong priority (default)
- Weak priority (suggestion)
- How Strong overrides Weak in conflicts

### 3. test_effect
**Tests reactive effects and change notifications.**

```bash
go run cmd/test_effect/main.go
```

Demonstrates:
- `Effect(fn, deps...)` running on dependency changes
- Effects triggering during constraint resolution
- Effects with computed expressions (Width, Height)
- Both `Signal[T]` and `Expr` as dependencies

### 4. test_computed
**Tests computed expressions on elements.**

```bash
go run cmd/test_computed/main.go
```

Demonstrates:
- `Width()` = Right - Left
- `Height()` = Bottom - Top
- `CenterX()` = Left + Width/2
- `CenterY()` = Top + Height/2
- `Position()`, `Size()`, `Center()`
- Constraints using computed expressions

### 5. test_relations
**Tests relational constraints between elements.**

```bash
go run cmd/test_relations/main.go
```

Demonstrates:
- `Inside(parent)` - containment
- `RightOf/LeftOf` - horizontal positioning with gaps
- `Above/Below` - vertical positioning with gaps
- `NearX/NearY/Near` - center proximity constraints
- `Eq(Point, Point)` - aligning positions

## Running All Tests

```bash
# Run each test
go run cmd/test_fluent/main.go
go run cmd/test_priority/main.go
go run cmd/test_effect/main.go
go run cmd/test_computed/main.go
go run cmd/test_relations/main.go

# Or build all
go build ./cmd/...
```

## API Quick Reference

### Constraint Addition (Fluent API)
```go
// Standalone functions
ui.Eq(child.Left, parent.Left.Add(10)).Add()
ui.Between(child.Width(), 50, 200).IsWeak().Add()

// Element methods
child.Inside(parent).Add()
child.RightOf(parent, 20).Add()
```

### Priorities
```go
constraint.IsRequired()  // Required (1e9) - must satisfy
constraint               // Strong (1e6) - default
constraint.IsWeak()      // Weak (1) - suggestion
```

### Effects
```go
signals.Effect(func() {
    x := child.Left.Get()
    y := child.Top.Get()
    fmt.Printf("Child at: (%.0f, %.0f)\n", x, y)
}, child.Left, child.Top)
```

### Element Properties
```go
box.Width()     // Computed: Right - Left
box.Height()    // Computed: Bottom - Top
box.CenterX()   // Computed: Left + Width/2
box.CenterY()   // Computed: Top + Height/2
box.Position()  // Point{Left, Top}
box.Size()      // Point{Width, Height}
box.Center()    // Point{CenterX, CenterY}
```
