package main

import (
	"fmt"

	"github.com/lithdew/casso"
	"github.com/tri2820/cheese/ui"
)

func main() {
	// Test with pure casso
	fmt.Println("=== Pure Casso Test ===")
	c := casso.NewSolver()
	sym1 := casso.New()
	sym2 := casso.New()

	// Constraint: sym1 == sym2 + 10
	// sym1 - sym2 - 10 == 0
	con := casso.NewConstraint(casso.EQ, -10, sym1.T(1.0), sym2.T(-1.0))
	c.AddConstraint(con)

	c.Edit(sym2, casso.Strong)
	c.Suggest(sym2, 100)

	fmt.Printf("sym2 (set to 100) = %v\n", c.Val(sym2))
	fmt.Printf("sym1 (should be 110) = %v\n", c.Val(sym1))

	// Now try with our solver
	fmt.Println("\n=== Our Solver Test ===")
	solver := ui.NewSolver()
	parent := solver.NewElement()
	child := solver.NewElement()

	solver.Add(ui.Eq(child.Left, parent.Left.Add(10)))

	parent.Left.Set(100)
	fmt.Printf("parent.Left = %v\n", parent.Left.Get())
	fmt.Printf("child.Left = %v (expected 110)\n", child.Left.Get())

	// Check casso state via Inner
	fmt.Println("\n=== Checking if casso has the constraint ===")
	// We can't access the symbol directly, but we can check if the solver has the constraint
	_ = solver.Inner()
}
