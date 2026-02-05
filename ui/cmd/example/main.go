package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Frame Widget Example ===")
	fmt.Println()

	layout := ui.NewLayout()

	// Create a root frame
	root := layout.NewFrame()
	layout.Add(
		ui.Eq(root.Left, 0),
		ui.Eq(root.Top, 0),
		ui.Eq(root.Right, 800),
		ui.Eq(root.Bottom, 600),
	)
	fmt.Println("Created root frame (800x600 at 0,0)")

	// Create a child frame
	child := layout.NewFrame()
	layout.Add(
		ui.Eq(child.Left, root.Left.Add(50)),
		ui.Eq(child.Top, root.Top.Add(50)),
		ui.Eq(child.Width(), 200),
		ui.Eq(child.Height(), 100),
	)
	fmt.Println("Created child frame (200x100 at 50,50)")
	fmt.Println()

	// Change color to trigger effect
	fmt.Println("Changing child color...")
	child.Color.Set("#FF0000")
	fmt.Println()

	// Move child to trigger effect
	fmt.Println("Moving child...")
	child.Left.Set(100)
	child.Top.Set(100)
	fmt.Println()

	// Resize root to trigger cascading effects
	fmt.Println("Resizing root...")
	root.Right.Set(1000)
	root.Bottom.Set(800)
	fmt.Println()

	// Check final state
	fmt.Println("=== Final State ===")
	fmt.Printf("Root: %s\n", formatRect(root))
	fmt.Printf("Child: %s\n", formatRect(child))
	fmt.Printf("Child Color: %s\n", child.Color.Get())
}

func formatRect(f *ui.Frame) string {
	return fmt.Sprintf("x=%d, y=%d, w=%d, h=%d",
		int(f.Left.Get()),
		int(f.Top.Get()),
		int(f.Right.Get()-f.Left.Get()),
		int(f.Bottom.Get()-f.Top.Get()),
	)
}
