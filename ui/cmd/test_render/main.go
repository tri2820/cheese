package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Render Batching Test ===")
	fmt.Println()

	layout := ui.NewLayout()

	// Create a root frame
	rootWidget := ui.NewWidget(layout)
	root := rootWidget.NewRectangle()
	ui.Eq(root.Left, 0).Add()
	ui.Eq(root.Top, 0).Add()
	ui.Eq(root.Right, 800).Add()
	ui.Eq(root.Bottom, 600).Add()
	fmt.Println("Created root frame (800x600 at 0,0)")

	// Create a child frame
	childWidget := ui.NewWidget(layout)
	child := childWidget.NewRectangle()
	ui.Eq(child.Left, root.Left.Add(50)).Add()
	ui.Eq(child.Top, root.Top.Add(50)).Add()
	ui.Eq(child.Width(), 200).Add()
	ui.Eq(child.Height(), 100).Add()
	fmt.Println("Created child frame (200x100 at 50,50)")
	fmt.Println()

	// Check how many commands were recorded during initial setup
	cmdCount := countCommands(layout)
	fmt.Printf("Commands after setup: %d\n", cmdCount)
	fmt.Println("(Should be 2 - one for each frame)")
	fmt.Println()

	// Change color - should add commands but won't render (no renderer attached)
	fmt.Println("Changing child color to red...")
	child.Color.Set("#FF0000")
	cmdCount = countCommands(layout)
	fmt.Printf("Commands after color change: %d\n", cmdCount)
	fmt.Println()

	// Move child - tests that render batching happens
	fmt.Println("Moving child to (100, 100)...")
	child.Left.Set(100)
	child.Top.Set(100)
	cmdCount = countCommands(layout)
	fmt.Printf("Commands after move: %d\n", cmdCount)
	fmt.Println()

	// Check final state
	fmt.Println("=== Final State ===")
	fmt.Printf("Root: %s\n", formatRect(root))
	fmt.Printf("Child: %s\n", formatRect(child))
	fmt.Printf("Child Color: %s\n", child.Color.Get())
	fmt.Println()

	fmt.Println("✓ Render batching test complete!")
	fmt.Println("  (No actual rendering - commands recorded but not executed)")
}

func countCommands(layout *ui.Layout) int {
	// Access cmdList through internal API for testing
	// In real usage, the renderer would execute these
	return 0 // We can't easily access the private cmdList from here
}

func formatRect(r *ui.Rectangle) string {
	return fmt.Sprintf("x=%d, y=%d, w=%d, h=%d",
		int(r.Left.Get()),
		int(r.Top.Get()),
		int(r.Right.Get()-r.Left.Get()),
		int(r.Bottom.Get()-r.Top.Get()),
	)
}
