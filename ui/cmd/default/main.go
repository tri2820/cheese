package main

import (
	"fmt"

	"github.com/lithdew/casso"
)

func main() {
	fmt.Println("=== Test: Default constraint vs Constraint.Strong ===")

	// Setup: two variables, both want different things
	// Default constraint: a >= 500
	// Strong constraint: b >= 500
	// Edit.Weak suggests both at 200
	// If they're equal, both should be 500

	solver := casso.NewSolver()
	a := casso.New()
	b := casso.New()

	// Default constraint on a
	solver.AddConstraint(casso.NewConstraint(casso.GTE, -500, a.T(1.0)))

	// Strong constraint on b
	solver.AddConstraintWithPriority(casso.Strong, casso.NewConstraint(casso.GTE, -500, b.T(1.0)))

	// Edit both with Weak, suggest 200
	solver.Edit(a, casso.Weak)
	solver.Edit(b, casso.Weak)
	solver.Suggest(a, 200)
	solver.Suggest(b, 200)

	fmt.Printf("Default constraint: a >= 500\n")
	fmt.Printf("Strong constraint:  b >= 500\n")
	fmt.Printf("Edit.Weak + Suggest: 200 for both\n\n")

	fmt.Printf("Result a (default): %v\n", solver.Val(a))
	fmt.Printf("Result b (strong):  %v\n", solver.Val(b))

	if solver.Val(a) == solver.Val(b) {
		fmt.Println("\nConclusion: Default constraint == Constraint.Strong")
	} else {
		fmt.Printf("\nConclusion: Default constraint != Constraint.Strong")
	}

	fmt.Println("\n=== Test: Default constraint priority value ===")
	// Check what casso uses as default priority
	fmt.Println("Can't inspect default priority directly from casso API")
	fmt.Println("But behavior suggests it's equivalent to Strong")
}
