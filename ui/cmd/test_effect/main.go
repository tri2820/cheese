package main

import (
	"fmt"

	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Test: Effect and Reactivity ===")
	fmt.Println()

	layout := ui.NewLayout()
	parent := layout.NewView()
	child := layout.NewView()

	// Test 1: Effect triggers on dependency change
	fmt.Println("Test 1: Effect with Expr dependencies")
	effectCount := 0
	signals.Effect(func() {
		effectCount++
		left := child.Left.Get()
		top := child.Top.Get()
		fmt.Printf("  [Effect #%d] Child at: (%.0f, %.0f)\n", effectCount, left, top)
	}, child.Left, child.Top)

	// Test 2: Effect runs immediately on creation
	fmt.Println("  → Effect runs immediately on creation")
	fmt.Println()

	// Add constraints and set values
	child.Inside(parent).Add()
	ui.Eq(child.Left, parent.Left.Add(10)).Add()

	parent.Left.Set(0)
	parent.Right.Set(1000)
	parent.Top.Set(0)
	parent.Bottom.Set(500)

	fmt.Println("Test 2: Effect runs on constraint resolution")
	fmt.Printf("  Parent bounds: (0, 0) to (1000, 500)\n")
	fmt.Printf("  Child at: (%.0f, %.0f)\n", child.Left.Get(), child.Top.Get())
	fmt.Printf("  Effect ran %d time(s)\n", effectCount)
	fmt.Println()

	// Test 3: Effect runs on manual value change
	fmt.Println("Test 3: Effect runs on Set()")
	parent.Left.Set(100)
	fmt.Printf("  After parent.Left.Set(100):\n")
	fmt.Printf("  Child at: (%.0f, %.0f)\n", child.Left.Get(), child.Top.Get())
	fmt.Printf("  Effect ran %d time(s) total\n", effectCount)
	fmt.Println()

	// Test 4: Effect with computed expressions
	fmt.Println("Test 4: Effect with Width/Height (computed expressions)")
	widthCount := 0
	signals.Effect(func() {
		widthCount++
		w := child.Width().Get()
		h := child.Height().Get()
		fmt.Printf("  [WidthEffect #%d] Size: %.0f x %.0f\n", widthCount, w, h)
	}, child.Width(), child.Height())

	child.Right.Set(600)
	child.Bottom.Set(400)
	fmt.Printf("  After child.Right.Set(600), child.Bottom.Set(400):\n")
	fmt.Printf("  Size: %.0f x %.0f\n", child.Width().Get(), child.Height().Get())

	fmt.Println()
	fmt.Println("=== Effect Summary ===")
	fmt.Println("  Effect(fn, deps...) → Runs immediately, then on dep change")
	fmt.Println("  Supports both Signal[T] and Expr as dependencies")
	fmt.Println("  Runs during constraint resolution (via OnChangeQuiet)")
}
