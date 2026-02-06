package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Test: Computed Expressions ===")
	fmt.Println()

	layout := ui.NewLayout()
	box := layout.NewView()

	// Set bounds
	box.Left.Set(100)
	box.Right.Set(500)
	box.Top.Set(200)
	box.Bottom.Set(400)

	fmt.Println("Box bounds:")
	fmt.Printf("  Left: %.0f, Right: %.0f\n", box.Left.Get(), box.Right.Get())
	fmt.Printf("  Top: %.0f, Bottom: %.0f\n", box.Top.Get(), box.Bottom.Get())
	fmt.Println()

	// Test 1: Width and Height
	fmt.Println("Test 1: Width() and Height()")
	fmt.Printf("  Width() = Right - Left = %.0f\n", box.Width().Get())
	fmt.Printf("  Height() = Bottom - Top = %.0f\n", box.Height().Get())
	fmt.Println()

	// Test 2: Center X and Y
	fmt.Println("Test 2: CenterX() and CenterY()")
	fmt.Printf("  CenterX() = Left + Width/2 = %.0f\n", box.CenterX().Get())
	fmt.Printf("  CenterY() = Top + Height/2 = %.0f\n", box.CenterY().Get())
	fmt.Println()

	// Test 3: Position() and Size()
	fmt.Println("Test 3: Position() and Size()")
	pos := box.Position()
	size := box.Size()
	fmt.Printf("  Position() → X: %.0f, Y: %.0f\n", pos.X.Get(), pos.Y.Get())
	fmt.Printf("  Size() → X: %.0f, Y: %.0f\n", size.X.Get(), size.Y.Get())
	fmt.Println()

	// Test 4: Center() returns Point
	fmt.Println("Test 4: Center() returns Point")
	center := box.Center()
	fmt.Printf("  Center() → X: %.0f, Y: %.0f\n", center.X.Get(), center.Y.Get())
	fmt.Println()

	// Test 5: Constrain using computed expressions
	fmt.Println("Test 5: Constraints with computed expressions")
	other := layout.NewView()
	ui.Eq(other.CenterX(), box.CenterX()).Add()
	ui.Eq(other.CenterY(), box.CenterY()).Add()
	ui.Eq(other.Width(), 100).Add()
	ui.Eq(other.Height(), 80).Add()

	other.Left.Set(0)
	other.Top.Set(0)

	fmt.Printf("  Box center: (%.0f, %.0f)\n", box.CenterX().Get(), box.CenterY().Get())
	fmt.Printf("  Other center: (%.0f, %.0f)\n", other.CenterX().Get(), other.CenterY().Get())
	fmt.Printf("  Other bounds: (%.0f, %.0f) to (%.0f, %.0f)\n",
		other.Left.Get(), other.Top.Get(), other.Right.Get(), other.Bottom.Get())
	fmt.Println()

	fmt.Println("=== Computed Expression Summary ===")
	fmt.Println("  Width()  → Right - Left")
	fmt.Println("  Height() → Bottom - Top")
	fmt.Println("  CenterX() → Left + Width/2")
	fmt.Println("  CenterY() → Top + Height/2")
	fmt.Println("  Position() → Point{X, Y}")
	fmt.Println("  Size() → Point{Width, Height}")
	fmt.Println("  Center() → Point{CenterX, CenterY}")
}
