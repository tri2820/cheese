package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	solver := ui.NewSolver()
	parent := solver.NewElement()
	child := solver.NewElement()

	// Effect runs immediately and when child dimensions change
	ui.Effect(func() {
		fmt.Printf("Child: left=%v top=%v width=%v height=%v\n",
			child.Left.Get(), child.Top.Get(),
			child.Width().Get(), child.Height().Get())
	}, child.Left, child.Top, child.Width(), child.Height())

	// Add constraints
	solver.Add(ui.Eq(child.Left, parent.Left.Add(10)))
	solver.Add(ui.Eq(child.Top, parent.Top))
	solver.Add(ui.Eq(child.Width(), 100))
	solver.Add(ui.Eq(child.Height(), 50))

	// Set parent values - triggers resolve, updates child, runs effect
	fmt.Println("\n--- Setting parent.Left = 0, parent.Right = 1000 ---")
	parent.Left.Set(0, ui.Strong)
	parent.Right.Set(1000, ui.Strong)
	parent.Top.Set(0, ui.Strong)
	parent.Bottom.Set(500, ui.Strong)

	fmt.Println("\n--- Setting parent.Left = 100 (should trigger effect with child.Left = 110) ---")
	parent.Left.Set(100, ui.Strong)

	fmt.Println("\n--- Demonstrating computed expressions work with Effect ---")
	ui.Effect(func() {
		fmt.Printf("Child center: (%v, %v)\n",
			child.CenterX().Get(), child.CenterY().Get())
	}, child.CenterX(), child.CenterY())

	fmt.Println("\n--- Final state ---")
	fmt.Printf("Parent: left=%v top=%v width=%v height=%v\n",
		parent.Left.Get(), parent.Top.Get(),
		parent.Width().Get(), parent.Height().Get())
	fmt.Printf("Child: left=%v top=%v width=%v height=%v\n",
		child.Left.Get(), child.Top.Get(),
		child.Width().Get(), child.Height().Get())
}
