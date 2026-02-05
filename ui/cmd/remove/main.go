package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Test: Add and Remove Constraints ===")
	solver := ui.NewSolver()
	parent := solver.NewElement()
	child := solver.NewElement()

	// Add constraint: child is 10px from parent's left
	handle := solver.Add(ui.Eq(child.Left, parent.Left.Add(10)))

	parent.Left.Set(0)
	fmt.Printf("parent.Left=%v, child.Left=%v (constraint active)\n", parent.Left.Get(), child.Left.Get())

	// Remove the constraint
	handle.Remove()

	parent.Left.Set(100)
	fmt.Printf("parent.Left=%v, child.Left=%v (constraint removed, child stays)\n", parent.Left.Get(), child.Left.Get())

	fmt.Println("\n=== Test: Dynamic Constraints ===")
	solver2 := ui.NewSolver()
	A := solver2.NewElement()
	B := solver2.NewElement()

	// Initially, B is right of A
	h1 := solver2.Add(B.RightOf(A, 10))
	A.Left.Set(0)
	A.Right.Set(100)
	fmt.Printf("A: left=%v right=%v\n", A.Left.Get(), A.Right.Get())
	fmt.Printf("B: left=%v (B.RightOf A with 10px gap)\n", B.Left.Get())

	// Remove the constraint
	h1.Remove()

	// Move A - B should not follow
	A.Left.Set(50)
	A.Right.Set(150)
	fmt.Printf("\nAfter removing constraint and moving A:\n")
	fmt.Printf("A: left=%v right=%v\n", A.Left.Get(), A.Right.Get())
	fmt.Printf("B: left=%v (B did not move)\n", B.Left.Get())

	// Add a new constraint: B is inside A
	_ = solver2.Add(B.Inside(A).IsRequired())

	// Set B's position explicitly (constraint will enforce it)
	B.Left.Set(60)
	B.Right.Set(140)
	fmt.Printf("\nAfter adding B.Inside(A) constraint:\n")
	fmt.Printf("A: left=%v right=%v\n", A.Left.Get(), A.Right.Get())
	fmt.Printf("B: left=%v right=%v (inside A)\n", B.Left.Get(), B.Right.Get())

	// Move A - B should move with it (Inside constraint affects all sides)
	A.Left.Set(100)
	A.Right.Set(200)
	fmt.Printf("\nMoving A:\n")
	fmt.Printf("A: left=%v right=%v\n", A.Left.Get(), A.Right.Get())
	fmt.Printf("B: left=%v right=%v (B moved with A)\n", B.Left.Get(), B.Right.Get())
}
