package main

import (
	"fmt"

	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui"
)

func main() {
	layout := ui.NewLayout(nil)

	// Create a simple rectangle with LayoutItem
	rect := layout.NewLayoutItem()

	// Effect runs immediately and when rect.Left or rect.Top change
	fmt.Println("=== Setting up Effect ===")
	signals.Effect(func() {
		fmt.Printf("Rect position changed: Left=%v, Top=%v\n",
			rect.Left.Get(), rect.Top.Get())
	}, rect.Left, rect.Top)

	// Add constraints - starting position
	fmt.Println("\n=== Adding initial constraints ===")
	leftHandle := ui.Eq(rect.Left, 10).Add()
	topHandle := ui.Eq(rect.Top, 20).Add()
	rightHandle := ui.Eq(rect.Right, 110).Add()
	bottomHandle := ui.Eq(rect.Bottom, 70).Add()

	fmt.Printf("Initial: Left=%v, Top=%v, Right=%v, Bottom=%v\n",
		rect.Left.Get(), rect.Top.Get(), rect.Right.Get(), rect.Bottom.Get())
	fmt.Printf("Initial size: Width=%v, Height=%v\n",
		rect.Width().Get(), rect.Height().Get())

	// Update Left position
	fmt.Println("\n=== Updating constraint: Left = 50 ===")
	leftHandle.Remove()
	ui.Eq(rect.Left, 50).Add()

	// Update Top position (and Bottom to maintain height)
	fmt.Println("\n=== Updating constraint: Top = 40, Bottom = 90 ===")
	topHandle.Remove()
	bottomHandle.Remove()
	ui.Eq(rect.Top, 40).Add()
	ui.Eq(rect.Bottom, 90).Add()

	// Effect with computed expressions (Width, Height)
	fmt.Println("\n=== Setting up Effect on Width and Height ===")
	signals.Effect(func() {
		fmt.Printf("Rect size changed: Width=%v, Height=%v\n",
			rect.Width().Get(), rect.Height().Get())
	}, rect.Width(), rect.Height())

	// Update Right constraint - should trigger width effect
	fmt.Println("\n=== Updating constraint: Right = 200 (affects Width) ===")
	rightHandle.Remove()
	ui.Eq(rect.Right, 200).Add()

	fmt.Println("\n=== Final state ===")
	fmt.Printf("Left=%v, Top=%v, Right=%v, Bottom=%v\n",
		rect.Left.Get(), rect.Top.Get(),
		rect.Right.Get(), rect.Bottom.Get())
	fmt.Printf("Width=%v, Height=%v\n",
		rect.Width().Get(), rect.Height().Get())

	// === TEST MIXED SIGNAL + EXPR ===
	fmt.Println("\n\n=== Testing mixed Signal + Expr dependencies ===")

	// Create a widget with a rectangle
	widget := ui.NewWidget(layout)
	rectangle := widget.NewRectangle()

	// Set initial color
	rectangle.Color.Set("#FF0000") // Red

	// Mixed effect: watches both Expr (position) and Signal (color)
	signals.Effect(func() {
		color := rectangle.Color.Get()
		left := rectangle.LayoutItem.Left.Get()
		top := rectangle.LayoutItem.Top.Get()
		fmt.Printf("Mixed effect: Color=%s, Position=(%v,%v)\n", color, left, top)
	}, rectangle.Color, rectangle.LayoutItem.Left, rectangle.LayoutItem.Top)

	fmt.Println("\n--- Add position constraints ---")
	leftHandle = ui.Eq(rectangle.LayoutItem.Left, 300).Add()
	topHandle = ui.Eq(rectangle.LayoutItem.Top, 150).Add()

	fmt.Println("\n--- Change color (pure Signal) ---")
	rectangle.Color.Set("#00FF00") // Green

	fmt.Println("\n--- Change position (pure Expr) ---")
	leftHandle.Remove()
	ui.Eq(rectangle.LayoutItem.Left, 400).Add()

	fmt.Println("\n--- Change both ---")
	rectangle.Color.Set("#0000FF") // Blue
	topHandle.Remove()
	ui.Eq(rectangle.LayoutItem.Top, 200).Add()
}
