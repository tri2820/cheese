package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Test: New Fluent API ===")
	solver := ui.NewSolver()
	A := solver.NewElement()
	B := solver.NewElement()

	// New API: B.RightOf(A, 10).Add()
	B.RightOf(A, 10).Add()

	A.Left.Set(0)
	A.Right.Set(100)
	fmt.Printf("A: left=%v right=%v\n", A.Left.Get(), A.Right.Get())
	fmt.Printf("B: left=%v (should be >= 110)\n", B.Left.Get())

	fmt.Println("\n=== Test: With priority ===")
	solver2 := ui.NewSolver()
	C := solver2.NewElement()
	D := solver2.NewElement()

	// With priority: D.RightOf(C, 20).IsRequired().Add()
	D.RightOf(C, 20).IsRequired().Add()

	C.Left.Set(0)
	C.Right.Set(100)
	fmt.Printf("C: left=%v right=%v\n", C.Left.Get(), C.Right.Get())
	fmt.Printf("D: left=%v (should be >= 120)\n", D.Left.Get())

	fmt.Println("\n=== All Tests Passed! ===")
}
