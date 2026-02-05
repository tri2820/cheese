package main

import (
	"fmt"

	"github.com/lithdew/casso"
)

func main() {
	symbol := casso.New()
	solver := casso.NewSolver()

	fmt.Println("=== Test: Add Required constraint, then Weak Edit + Suggest ===")
	
	// Add constraint FIRST: width must be >= 500
	solver.AddConstraintWithPriority(casso.Required, 
		casso.NewConstraint(casso.GTE, -500, symbol.T(1.0)))
	
	// Edit with Weak, suggest 200
	solver.Edit(symbol, casso.Weak)
	solver.Suggest(symbol, 200)
	
	fmt.Printf("Result: %v (constraint wins - 500)\n", solver.Val(symbol))
	
	fmt.Println("\n=== Test: Suggest 1000 (satisfies constraint) ===")
	solver.Suggest(symbol, 1000)
	fmt.Printf("Result: %v (suggestion wins - satisfies constraint)\n", solver.Val(symbol))
}
