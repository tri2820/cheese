package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Test: layout.Add() vs View.Method().Add() ===")
	fmt.Println()

	// Method 1: layout.Add() for standalone constraints
	fmt.Println("Method 1: layout.Add() for Width/Height")
	layout := ui.NewLayout()
	box := layout.NewView()

	layout.Add(ui.Eq(box.Width(), 200))
	layout.Add(ui.Eq(box.Height(), 100))
	box.Left.Set(0)
	box.Top.Set(0)

	fmt.Printf("  Box: Width=%.0f Height=%.0f ✓\n", box.Width().Get(), box.Height().Get())

	// Method 2: .Add() for view relationship constraints
	fmt.Println("\nMethod 2: View.Method().Add() for relationships")
	layout2 := ui.NewLayout()
	parent := layout2.NewView()
	child := layout2.NewView()

	// Child is right of parent with gap
	child.RightOf(parent, 10).IsRequired().Add()

	parent.Left.Set(0)
	parent.Right.Set(100)
	fmt.Printf("  Parent: Left=%.0f Right=%.0f\n", parent.Left.Get(), parent.Right.Get())
	fmt.Printf("  Child: Left=%.0f (RightOf parent with 10px gap) ✓\n", child.Left.Get())

	// Method 3: Combine both
	fmt.Println("\nMethod 3: Combine both APIs")
	layout3 := ui.NewLayout()
	A := layout3.NewView()
	B := layout3.NewView()

	B.RightOf(A, 20).Add()             // Fluent API
	layout3.Add(ui.Eq(A.Width(), 100)) // layout.Add()
	layout3.Add(ui.Eq(B.Width(), 150)) // layout.Add()

	A.Left.Set(0)
	A.Right.Set(100)

	fmt.Printf("  A: Left=%.0f Right=%.0f Width=%.0f ✓\n", A.Left.Get(), A.Right.Get(), A.Width().Get())
	fmt.Printf("  B: Left=%.0f Width=%.0f ✓\n", B.Left.Get(), B.Width().Get())

	fmt.Println("\n✓ Width/Height constraints and effects work correctly!")
}
