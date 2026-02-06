package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Test: Constraint Priorities ===")
	fmt.Println()

	layout := ui.NewLayout()
	parent := layout.NewView()
	child := layout.NewView()

	// Test 1: Required (highest priority - must be satisfied)
	fmt.Println("Test 1: Required priority")
	child.Inside(parent).IsRequired().Add()
	fmt.Println("  child.Inside(parent).IsRequired().Add()")
	fmt.Println("  → Child MUST be inside parent (will never be violated)")
	fmt.Println()

	// Test 2: Strong (default priority)
	fmt.Println("Test 2: Strong priority (default)")
	ui.Eq(child.Left, parent.Left.Add(10)).Add()
	fmt.Println("  ui.Eq(child.Left, parent.Left.Add(10)).Add()")
	fmt.Println("  → Uses Strong by default")
	fmt.Println()

	// Test 3: Weak (lowest priority - can be overridden)
	fmt.Println("Test 3: Weak priority")
	ui.Between(child.Width(), 50, 200).IsWeak().Add()
	fmt.Println("  ui.Between(child.Width(), 50, 200).IsWeak().Add()")
	fmt.Println("  → Prefers width between 50-200, but can be overridden")
	fmt.Println()

	// Test 4: Conflict resolution
	fmt.Println("Test 4: Priority conflict resolution")
	smallChild := layout.NewView()
	smallChild.Inside(parent).IsRequired().Add()
	ui.Eq(smallChild.Width(), 500).Add()                   // Strong: wants width=500
	ui.Between(smallChild.Width(), 50, 100).IsWeak().Add() // Weak: wants 50-100

	parent.Left.Set(0)
	parent.Right.Set(1000)
	parent.Top.Set(0)
	parent.Bottom.Set(500)
	smallChild.Left.Set(0)
	smallChild.Top.Set(0)

	fmt.Printf("  Strong (width=500) vs Weak (50-100)\n")
	fmt.Printf("  Result: width = %.0f (Strong wins)\n", smallChild.Width().Get())
	fmt.Println()

	fmt.Println("=== Priority Summary ===")
	fmt.Println("  Required (1e9) → Must be satisfied")
	fmt.Println("  Strong   (1e6) → Default, can be overridden by Required")
	fmt.Println("  Weak     (1)   → Suggestion, can be overridden by Strong/Required")
}
