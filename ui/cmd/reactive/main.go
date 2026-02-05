package main

import (
	"fmt"

	"github.com/lithdew/casso"
	"github.com/tri2820/cheese/signals"
)

// LayoutState holds the computed layout values from the constraint solver
type LayoutState struct {
	ContainerWidth  float64
	ChildX          float64
	ChildCompWidth  float64
	Child2X         float64
	Child2CompWidth float64
}

// Solver encapsulates the constraint solver and casso variables
type Solver struct {
	solver          *casso.Solver
	containerWidth  casso.Symbol
	childX          casso.Symbol
	childCompWidth  casso.Symbol
	child2X         casso.Symbol
	child2CompWidth casso.Symbol
}

// NewSolver creates a new constraint solver with layout constraints
func NewSolver() *Solver {
	s := &Solver{
		solver:          casso.NewSolver(),
		containerWidth:  casso.New(),
		childX:          casso.New(),
		childCompWidth:  casso.New(),
		child2X:         casso.New(),
		child2CompWidth: casso.New(),
	}

	// Constraint formula: Constant + (Symbol1 × Coeff1) + (Symbol2 × Coeff2) ... [Relation] 0
	//
	// c1: childX == (50.0 / 1024) * containerWidth
	//     → 0 + (childX × 1.0) + (containerWidth × -50.0/1024) == 0
	c1 := casso.NewConstraint(casso.EQ, 0, s.childX.T(1.0), s.containerWidth.T(-50.0/1024))

	// c2: childCompWidth == (200.0 / 1024) * containerWidth
	//     → 0 + (childCompWidth × 1.0) + (containerWidth × -200.0/1024) == 0
	c2 := casso.NewConstraint(casso.EQ, 0, s.childCompWidth.T(1.0), s.containerWidth.T(-200.0/1024))

	// c3: childCompWidth >= 200.0
	//     → -200 + (childCompWidth × 1.0) >= 0
	c3 := casso.NewConstraint(casso.GTE, -200, s.childCompWidth.T(1.0))

	// c4: child2X - childX - childCompWidth == 50
	//     → -50 + (child2X × 1.0) + (childX × -1.0) + (childCompWidth × -1.0) == 0
	c4 := casso.NewConstraint(casso.EQ, -50, s.child2X.T(1.0), s.childX.T(-1.0), s.childCompWidth.T(-1.0))

	// c5: child2CompWidth == 50 + containerWidth + child2X
	//     → 50 + (child2CompWidth × 1.0) + (containerWidth × -1.0) + (child2X × 1.0) == 0
	c5 := casso.NewConstraint(casso.EQ, 50, s.child2CompWidth.T(1.0), s.containerWidth.T(-1.0), s.child2X.T(1.0))

	// Add constraints to the solver
	s.solver.AddConstraint(c1)
	s.solver.AddConstraintWithPriority(casso.Weak, c2)
	s.solver.AddConstraintWithPriority(casso.Strong, c3)
	s.solver.AddConstraint(c4)
	s.solver.AddConstraint(c5)

	// Mark containerWidth as editable
	s.solver.Edit(s.containerWidth, casso.Strong)

	return s
}

// Solve sets the container width and returns the computed layout state
func (s *Solver) Solve(containerWidth float64) LayoutState {
	s.solver.Suggest(s.containerWidth, containerWidth)

	return LayoutState{
		ContainerWidth:  s.solver.Val(s.containerWidth),
		ChildX:          s.solver.Val(s.childX),
		ChildCompWidth:  s.solver.Val(s.childCompWidth),
		Child2X:         s.solver.Val(s.child2X),
		Child2CompWidth: s.solver.Val(s.child2CompWidth),
	}
}

func main() {
	solver := NewSolver()

	// Create a source signal for container width
	containerWidth := signals.New(2048.0)

	// Create a computed signal that derives layout from container width
	// When containerWidth changes, the layout automatically recomputes
	layout := signals.Compute(func() LayoutState {
		width := containerWidth.Get()
		fmt.Printf("[Recompute] Solving layout for containerWidth=%v...\n", width)
		return solver.Solve(width)
	}, containerWidth)

	// Subscribe to layout changes
	layout.Subscribe(func(state LayoutState) {
		fmt.Println("=== Layout Updated ===")
		fmt.Printf("Container Width:  %v\n", state.ContainerWidth)
		fmt.Printf("Child X:          %v\n", state.ChildX)
		fmt.Printf("Child Comp Width: %v\n", state.ChildCompWidth)
		fmt.Printf("Child2 X:         %v\n", state.Child2X)
		fmt.Printf("Child2 Comp Width:%v\n", state.Child2CompWidth)
		fmt.Println()
	})

	// Initial state is computed automatically
	fmt.Println("=== Initial State (containerWidth = 2048) ===")
	fmt.Printf("Container: %v, ChildWidth: %v, Child2Width: %v\n\n",
		layout.Get().ContainerWidth,
		layout.Get().ChildCompWidth,
		layout.Get().Child2CompWidth)

	// Update container width - layout automatically recomputes!
	fmt.Println("=== Updating containerWidth to 500 ===")
	containerWidth.Set(500)

	// Let's change it again
	fmt.Println("=== Updating containerWidth to 1024 ===")
	containerWidth.Set(1024)

	// And one more time
	fmt.Println("=== Updating containerWidth to 4000 ===")
	containerWidth.Set(4000)

	// Demonstrate that we can also create derived computed signals
	minChildWidth := signals.Compute(func() float64 {
		// Child width is at minimum 200
		if layout.Get().ChildCompWidth < 200.1 {
			return 200
		}
		return layout.Get().ChildCompWidth
	}, layout)

	fmt.Println("=== Final computed values ===")
	fmt.Printf("Container Width:  %v\n", layout.Get().ContainerWidth)
	fmt.Printf("Child Comp Width: %v\n", layout.Get().ChildCompWidth)
	fmt.Printf("Min Child Width:  %v (derived computed signal)\n", minChildWidth.Get())
	fmt.Printf("Child2 Comp Width:%v\n", layout.Get().Child2CompWidth)
}
