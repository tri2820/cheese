package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Frame LayoutItem Example ===")
	fmt.Println()

	layout := ui.NewLayout()

	// Create a root frame
	rootWidget := ui.NewWidget(layout)
	root := rootWidget.NewRectangle()
	layout.Add(
		ui.Eq(root.Left, 0),
		ui.Eq(root.Top, 0),
		ui.Eq(root.Right, 800),
		ui.Eq(root.Bottom, 600),
	)
	fmt.Println("Created root frame (800x600 at 0,0)")

	// Create a child frame
	childWidget := ui.NewWidget(layout)
	child := childWidget.NewRectangle()
	layout.Add(
		ui.Eq(child.Left, root.Left.Add(50)),
		ui.Eq(child.Top, root.Top.Add(50)),
		ui.Eq(child.Width(), 200),
		ui.Eq(child.Height(), 100),
	)
	fmt.Println("Created child frame (200x100 at 50,50)")
	fmt.Println()

	// Create a label
	labelWidget := ui.NewWidget(layout)
	label := labelWidget.NewLabel("Hello World!")
	layout.Add(
		ui.Eq(label.Left, root.Left.Add(300)),
		ui.Eq(label.Top, root.Top.Add(100)),
		ui.Eq(label.Width(), 300),
		ui.Eq(label.Height(), 50),
	)
	fmt.Println("Created label (300x50 at 300,100)")
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

	// Change label properties
	fmt.Println("Changing label text and color...")
	label.Text.Set("New Text!")
	label.Color.Set("#00FF00") // Green
	label.FontSize.Set(16.0)
	fmt.Println()

	// Check final state
	fmt.Println("=== Final State ===")
	fmt.Printf("Root: %s\n", formatRect(root))
	fmt.Printf("Child: %s\n", formatRect(child))
	fmt.Printf("Child Color: %s\n", child.Color.Get())
	fmt.Printf("Label Text: %s\n", label.Text.Get())
	fmt.Printf("Label Color: %s\n", label.Color.Get())
	fmt.Printf("Label Font Size: %.1fpt\n", label.FontSize.Get())
}

func formatRect(r *ui.Rectangle) string {
	return fmt.Sprintf("x=%d, y=%d, w=%d, h=%d",
		int(r.Left.Get()),
		int(r.Top.Get()),
		int(r.Right.Get()-r.Left.Get()),
		int(r.Bottom.Get()-r.Top.Get()),
	)
}
