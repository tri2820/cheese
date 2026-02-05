package main

import (
	"fmt"

	"github.com/lithdew/casso"
)

func main() {
	fmt.Println("=== Testing Priority Levels ===")
	fmt.Println()

	// Test 1: Constraint.Required vs Edit.Strong
	fmt.Println("Test 1: Constraint.Required vs Edit.Strong")
	solver1 := casso.NewSolver()
	symbol1 := casso.New()
	solver1.AddConstraintWithPriority(casso.Required, casso.NewConstraint(casso.GTE, -500, symbol1.T(1.0)))
	solver1.Edit(symbol1, casso.Strong)
	solver1.Suggest(symbol1, 200)
	fmt.Printf("  Constraint: width >= 500 (Required)\n")
	fmt.Printf("  Edit.Strong + Suggest: 200\n")
	fmt.Printf("  Result: %v\n", solver1.Val(symbol1))
	fmt.Printf("  Winner: %v\n\n", map[bool]string{true: "Constraint.Required", false: "Edit.Strong"}[solver1.Val(symbol1) == 500])

	// Test 2: Constraint.Required vs Constraint.Strong
	fmt.Println("Test 2: Constraint.Required vs Constraint.Strong")
	solver2 := casso.NewSolver()
	symbol2a := casso.New()
	symbol2b := casso.New()
	solver2.AddConstraintWithPriority(casso.Required, casso.NewConstraint(casso.GTE, -100, symbol2a.T(1.0)))
	solver2.AddConstraintWithPriority(casso.Strong, casso.NewConstraint(casso.GTE, -200, symbol2b.T(1.0)))
	solver2.Edit(symbol2a, casso.Weak)
	solver2.Edit(symbol2b, casso.Weak)
	solver2.Suggest(symbol2a, 0)
	solver2.Suggest(symbol2b, 0)
	fmt.Printf("  Constraint.Required: >= 100\n")
	fmt.Printf("  Constraint.Strong: >= 200\n")
	fmt.Printf("  Edit.Weak + Suggest: 0 for both\n")
	fmt.Printf("  Required result: %v (wants >= 100)\n", solver2.Val(symbol2a))
	fmt.Printf("  Strong result: %v (wants >= 200)\n", solver2.Val(symbol2b))

	// Test 3: Constraint.Strong vs Edit.Strong
	fmt.Println("\nTest 3: Constraint.Strong vs Edit.Strong")
	solver3 := casso.NewSolver()
	symbol3 := casso.New()
	solver3.AddConstraintWithPriority(casso.Strong, casso.NewConstraint(casso.GTE, -500, symbol3.T(1.0)))
	solver3.Edit(symbol3, casso.Strong)
	solver3.Suggest(symbol3, 200)
	fmt.Printf("  Constraint: width >= 500 (Strong)\n")
	fmt.Printf("  Edit.Strong + Suggest: 200\n")
	fmt.Printf("  Result: %v\n", solver3.Val(symbol3))
	fmt.Printf("  Winner: %v\n\n", map[bool]string{true: "Constraint.Strong", false: "Edit.Strong"}[solver3.Val(symbol3) == 500])

	// Test 4: Constraint.Weak vs Edit.Strong
	fmt.Println("Test 4: Constraint.Weak vs Edit.Strong")
	solver4 := casso.NewSolver()
	symbol4 := casso.New()
	solver4.AddConstraintWithPriority(casso.Weak, casso.NewConstraint(casso.GTE, -500, symbol4.T(1.0)))
	solver4.Edit(symbol4, casso.Strong)
	solver4.Suggest(symbol4, 200)
	fmt.Printf("  Constraint: width >= 500 (Weak)\n")
	fmt.Printf("  Edit.Strong + Suggest: 200\n")
	fmt.Printf("  Result: %v\n", solver4.Val(symbol4))
	fmt.Printf("  Winner: %v\n\n", map[bool]string{true: "Constraint.Weak", false: "Edit.Strong"}[solver4.Val(symbol4) == 500])

	// Test 5: Constraint.Strong vs Edit.Weak
	fmt.Println("Test 5: Constraint.Strong vs Edit.Weak")
	solver5 := casso.NewSolver()
	symbol5 := casso.New()
	solver5.AddConstraintWithPriority(casso.Strong, casso.NewConstraint(casso.GTE, -500, symbol5.T(1.0)))
	solver5.Edit(symbol5, casso.Weak)
	solver5.Suggest(symbol5, 200)
	fmt.Printf("  Constraint: width >= 500 (Strong)\n")
	fmt.Printf("  Edit.Weak + Suggest: 200\n")
	fmt.Printf("  Result: %v\n", solver5.Val(symbol5))
	fmt.Printf("  Winner: %v\n\n", map[bool]string{true: "Constraint.Strong", false: "Edit.Weak"}[solver5.Val(symbol5) == 500])

	// Test 6: Edit.Strong vs Edit.Weak (same variable - last one wins?)
	fmt.Println("Test 6: What if we have two Edits?")
	solver6 := casso.NewSolver()
	symbol6a := casso.New()
	symbol6b := casso.New()
	solver6.Edit(symbol6a, casso.Strong)
	solver6.Suggest(symbol6a, 100)
	solver6.Edit(symbol6b, casso.Weak)
	solver6.Suggest(symbol6b, 200)
	fmt.Printf("  Edit.Strong + Suggest: 100\n")
	fmt.Printf("  Edit.Weak + Suggest: 200\n")
	fmt.Printf("  Strong result: %v\n", solver6.Val(symbol6a))
	fmt.Printf("  Weak result: %v\n\n", solver6.Val(symbol6b))

	// Test 7: Default constraint (no priority) vs Edit.Strong
	fmt.Println("Test 7: Default constraint (no priority) vs Edit.Strong")
	solver7 := casso.NewSolver()
	symbol7 := casso.New()
	solver7.AddConstraint(casso.NewConstraint(casso.GTE, -500, symbol7.T(1.0)))
	solver7.Edit(symbol7, casso.Strong)
	solver7.Suggest(symbol7, 200)
	fmt.Printf("  Constraint: width >= 500 (no priority specified)\n")
	fmt.Printf("  Edit.Strong + Suggest: 200\n")
	fmt.Printf("  Result: %v\n", solver7.Val(symbol7))
	fmt.Printf("  Winner: %v\n\n", map[bool]string{true: "Default constraint", false: "Edit.Strong"}[solver7.Val(symbol7) == 500])

	// Test 8: Default constraint vs Edit.Weak
	fmt.Println("Test 8: Default constraint vs Edit.Weak")
	solver8 := casso.NewSolver()
	symbol8 := casso.New()
	solver8.AddConstraint(casso.NewConstraint(casso.GTE, -500, symbol8.T(1.0)))
	solver8.Edit(symbol8, casso.Weak)
	solver8.Suggest(symbol8, 200)
	fmt.Printf("  Constraint: width >= 500 (no priority)\n")
	fmt.Printf("  Edit.Weak + Suggest: 200\n")
	fmt.Printf("  Result: %v\n", solver8.Val(symbol8))
	fmt.Printf("  Winner: %v\n\n", map[bool]string{true: "Default constraint", false: "Edit.Weak"}[solver8.Val(symbol8) == 500])
}
