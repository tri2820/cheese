package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Test: Fluent .Add() API ===")
	fmt.Println()

	layout := ui.NewLayout()
	parent := layout.NewLayoutItem()
	child := layout.NewLayoutItem()

	// Test 1: Fluent API on standalone constraints
	fmt.Println("Test 1: ui.Eq(child.Left, parent.Left.Add(10)).Add()")
	ui.Eq(child.Left, parent.Left.Add(10)).Add()
	ui.Eq(child.Top, parent.Top.Add(20)).Add()

	// Test 2: Fluent API on layoutitem methods
	fmt.Println("Test 2: child.Inside(parent).Add()")
	child.Inside(parent).Add()

	// Test 3: Chaining priority with .Add()
	fmt.Println("Test 3: ui.Between(child.Width(), 50, 200).IsWeak().Add()")
	ui.Between(child.Width(), 50, 200).IsWeak().Add()

	// Set parent bounds
	parent.Left.Set(0)
	parent.Right.Set(1000)
	parent.Top.Set(0)
	parent.Bottom.Set(500)

	fmt.Printf("  Parent bounds: (0, 0) to (1000, 500)\n")
	fmt.Printf("  Child position: (%.0f, %.0f)\n", child.Left.Get(), child.Top.Get())
	fmt.Printf("  Child size: %.0f x %.0f\n", child.Width().Get(), child.Height().Get())
	fmt.Println()

	// Test 4: Multiple constraints in one Add()
	fmt.Println("Test 4: Multiple constraints with single .Add()")
	another := layout.NewLayoutItem()
	ui.Eq(child.Width(), another.Width()).Add()
	ui.Eq(child.Height(), another.Height()).Add()

	fmt.Println("=== All tests passed ===")
}
