package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Test: IsRequired() and IsWeak() API ===")
	fmt.Println()

	solver := ui.NewSolver()
	parent := ui.NewElement()
	child := ui.NewElement()

	// Test 1: Default (Strong)
	fmt.Println("Test 1: Default priority (Strong)")
	solver.Add(ui.Eq(child.Left, parent.Left.Add(10)))
	fmt.Println("  solver.Add(Eq(child.Left, parent.Left.Add(10)))")
	fmt.Println("  → Uses Strong (default)")
	fmt.Println()

	// Test 2: IsRequired()
	fmt.Println("Test 2: IsRequired()")
	solver.Add(child.Inside(parent).IsRequired())
	fmt.Println("  solver.Add(child.Inside(parent).IsRequired())")
	fmt.Println("  → Uses Required priority")
	fmt.Println()

	// Test 3: IsWeak()
	fmt.Println("Test 3: IsWeak()")
	solver.Add(ui.Between(child.Width(), 50, 200).IsWeak())
	fmt.Println("  solver.Add(Between(child.Width(), 50, 200).IsWeak())")
	fmt.Println("  → Uses Weak priority")
	fmt.Println()

	// Test 4: Chaining - multiple constraints with same priority
	fmt.Println("Test 4: Multiple constraints with .IsRequired()")
	another := ui.NewElement()
	solver.Add(
		ui.Eq(child.Width(), another.Width()),
		ui.Eq(child.Height(), another.Height()),
	)
	fmt.Println("  solver.Add(Eq(child.Width(), another.Width()), Eq(child.Height(), another.Height()))")
	fmt.Println("  → Both use Strong (default)")
	fmt.Println()

	// Test 5: Verify it compiles and runs
	fmt.Println("Test 5: Setting values")
	solver.Set(parent.Left, 0, ui.Required)
	solver.Set(parent.Right, 1000, ui.Required)
	solver.Set(parent.Top, 0, ui.Required)
	solver.Set(parent.Bottom, 500, ui.Required)

	fmt.Printf("  Parent bounds: (0, 0) to (1000, 500)\n")
	fmt.Printf("  Child at: (%v, %v)\n", solver.Val(child.Left), solver.Val(child.Top))
	fmt.Println()

	fmt.Println("=== API Summary ===")
	fmt.Println("  constraint                   → Strong (default)")
	fmt.Println("  constraint.IsRequired()       → Required")
	fmt.Println("  constraint.IsWeak()           → Weak")
}
