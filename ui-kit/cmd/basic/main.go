package main

import (
	"fmt"

	"github.com/lithdew/casso"
)

func main() {
	s := casso.NewSolver()

	containerWidth := casso.New()

	childX := casso.New()
	childCompWidth := casso.New()

	child2X := casso.New()
	child2CompWidth := casso.New()

	// c1: childX == (50.0 / 1024) * containerWidth
	// c2: childCompWidth == (200.0 / 1024) * containerWidth
	// c3: childCompWidth >= 200.0
	// c4: child2X - childX - childCompWidth == 50
	// c5: child2CompWidth == 50 + containerWidth + child2X

	c1 := casso.NewConstraint(casso.EQ, 0, childX.T(1.0), containerWidth.T(-50.0/1024))
	c2 := casso.NewConstraint(casso.EQ, 0, childCompWidth.T(1.0), containerWidth.T(-200.0/1024))
	c3 := casso.NewConstraint(casso.GTE, -200, childCompWidth.T(1.0))
	c4 := casso.NewConstraint(casso.EQ, -50, child2X.T(1.0), childX.T(-1.0), childCompWidth.T(-1.0))
	c5 := casso.NewConstraint(casso.EQ, 50, child2CompWidth.T(1.0), containerWidth.T(-1.0), child2X.T(1.0))

	// Mark 'containerWidth' as an editable variable with strong precedence.
	// Suggest 'containerWidth' to take on the value 2048.

	if err := s.Edit(containerWidth, casso.Strong); err != nil {
		panic(err)
	}
	if err := s.Suggest(containerWidth, 2048); err != nil {
		panic(err)
	}

	// Add constraints to the solver.

	if _, err := s.AddConstraint(c1); err != nil {
		panic(err)
	}

	if _, err := s.AddConstraintWithPriority(casso.Weak, c2); err != nil {
		panic(err)
	}

	if _, err := s.AddConstraintWithPriority(casso.Strong, c3); err != nil {
		panic(err)
	}

	if _, err := s.AddConstraint(c4); err != nil {
		panic(err)
	}

	if _, err := s.AddConstraint(c5); err != nil {
		panic(err)
	}

	// Grab computed values.

	fmt.Println("=== Initial state (containerWidth suggested as 2048) ===")
	fmt.Printf("containerWidth:    %v (expected: 2048)\n", s.Val(containerWidth))
	fmt.Printf("childCompWidth:    %v (expected: 400)\n", s.Val(childCompWidth))
	fmt.Printf("child2CompWidth:   %v (expected: 1448)\n", s.Val(child2CompWidth))
	fmt.Printf("childX:            %v\n", s.Val(childX))
	fmt.Printf("child2X:           %v\n", s.Val(child2X))

	// Suggest 'containerWidth' to take on the value 500.

	if err := s.Suggest(containerWidth, 500); err != nil {
		panic(err)
	}

	// Grab computed values.

	fmt.Println("\n=== After suggesting containerWidth as 500 ===")
	fmt.Printf("containerWidth:    %v (expected: 500)\n", s.Val(containerWidth))
	fmt.Printf("childCompWidth:    %v (expected: 200)\n", s.Val(childCompWidth))
	fmt.Printf("child2CompWidth:   %v (expected: 175.5859375)\n", s.Val(child2CompWidth))
	fmt.Printf("childX:            %v\n", s.Val(childX))
	fmt.Printf("child2X:           %v\n", s.Val(child2X))
}
