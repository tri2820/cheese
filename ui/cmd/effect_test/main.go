package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	layout := ui.NewLayout(nil)

	// Create a simple rectangle with LayoutItem
	rect := layout.NewLayoutItem()

	// Effect runs immediately and when rect.Left or rect.Top change
	fmt.Println("=== Setting up Effect ===")
	ui.Effect(func() {
		fmt.Printf("Rect position changed: Left=%v, Top=%v\n",
			rect.Left.Get(), rect.Top.Get())
	}, rect.Left, rect.Top)

	// Add constraints
	fmt.Println("\n=== Adding constraints ===")
	ui.Eq(rect.Left, 10).Add()
	ui.Eq(rect.Top, 20).Add()
	ui.Eq(rect.Right, 110).Add()
	ui.Eq(rect.Bottom, 70).Add()

	// Update constraints - should trigger effect
	fmt.Println("\n=== Updating constraint: Left = 50 ===")
	ui.Eq(rect.Left, 50).Add()

	fmt.Println("\n=== Updating constraint: Top = 100 ===")
	ui.Eq(rect.Top, 100).Add()

	// Effect with computed expressions (Width, Height)
	fmt.Println("\n=== Setting up Effect on Width and Height ===")
	ui.Effect(func() {
		fmt.Printf("Rect size changed: Width=%v, Height=%v\n",
			rect.Width().Get(), rect.Height().Get())
	}, rect.Width(), rect.Height())

	// Update Right constraint - should trigger width effect
	fmt.Println("\n=== Updating constraint: Right = 200 (affects Width) ===")
	ui.Eq(rect.Right, 200).Add()

	fmt.Println("\n=== Final state ===")
	fmt.Printf("Left=%v, Top=%v, Right=%v, Bottom=%v\n",
		rect.Left.Get(), rect.Top.Get(),
		rect.Right.Get(), rect.Bottom.Get())
	fmt.Printf("Width=%v, Height=%v\n",
		rect.Width().Get(), rect.Height().Get())
}
