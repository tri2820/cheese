package main

import (
	"fmt"

	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Test: IsRequired() and IsWeak() API ===")
	fmt.Println()

	layout := ui.NewLayout()
	parent := layout.NewElement()
	child := layout.NewElement()

	// Test Effect with computed expressions
	effectRuns := 0
	signals.Effect(func() {
		effectRuns++
		left := child.Left.Get()
		top := child.Top.Get()
		fmt.Printf("  [Effect #%d] Child position: (%.0f, %.0f)\n",
			effectRuns, left, top)
	}, child.Left, child.Top)

	// Test 1: Default (Strong)
	fmt.Println("Test 1: Default priority (Strong)")
	layout.Add(ui.Eq(child.Left, parent.Left.Add(10)))
	fmt.Println("  layout.Add(Eq(child.Left, parent.Left.Add(10)))")
	fmt.Println("  → Uses Strong (default)")
	fmt.Println()

	// Test 2: IsRequired()
	fmt.Println("Test 2: IsRequired()")
	layout.Add(child.Inside(parent).IsRequired())
	fmt.Println("  layout.Add(child.Inside(parent).IsRequired())")
	fmt.Println("  → Uses Required priority")
	fmt.Println()

	// Test 3: IsWeak()
	fmt.Println("Test 3: IsWeak()")
	layout.Add(ui.Between(child.Width(), 50, 200).IsWeak())
	fmt.Println("  layout.Add(Between(child.Width(), 50, 200).IsWeak())")
	fmt.Println("  → Uses Weak priority")
	fmt.Println()

	// Test 4: Chaining - multiple constraints with same priority
	fmt.Println("Test 4: Multiple constraints with .IsRequired()")
	another := layout.NewElement()
	layout.Add(
		ui.Eq(child.Width(), another.Width()),
		ui.Eq(child.Height(), another.Height()),
	)
	fmt.Println("  layout.Add(Eq(child.Width(), another.Width()), Eq(child.Height(), another.Height()))")
	fmt.Println("  → Both use Strong (default)")
	fmt.Println()

	// Test 5: Verify it compiles and runs
	fmt.Println("Test 5: Setting values (triggers Effect)")
	parent.Left.Set(0)
	parent.Right.Set(1000)
	parent.Top.Set(0)
	parent.Bottom.Set(500)

	fmt.Printf("  Parent bounds: (0, 0) to (1000, 500)\n")
	fmt.Printf("  Child at: (%v, %v)\n", child.Left.Get(), child.Top.Get())
	fmt.Printf("  Effect ran %d time(s)\n", effectRuns)
	fmt.Println()

	// Test 6: Update values, Effect should run again
	fmt.Println("Test 6: Moving parent (Effect should trigger again)")
	parent.Left.Set(100)
	fmt.Printf("  Parent bounds: (100, 0) to (1000, 500)\n")
	fmt.Printf("  Child at: (%v, %v)\n", child.Left.Get(), child.Top.Get())
	fmt.Printf("  Effect ran %d time(s) total\n", effectRuns)
	fmt.Println()

	fmt.Println("=== API Summary ===")
	fmt.Println("  constraint                   → Strong (default)")
	fmt.Println("  constraint.IsRequired()       → Required")
	fmt.Println("  constraint.IsWeak()           → Weak")
	fmt.Println("  Effect(fn, deps...)           → Runs on dep change")

	fmt.Println("=== Test 7: Area Effect ===")
	// Area as (Right - Left) * (Bottom - Top)
	area := signals.Derive(func() float64 {
		width := child.Right.Get() - child.Left.Get()
		height := child.Bottom.Get() - child.Top.Get()
		return width * height
	}, child.Left, child.Right, child.Top, child.Bottom)
	areaEffectRuns := 0
	signals.Effect(func() {
		areaEffectRuns++
		fmt.Printf("  [Area Effect #%d] Area (Width*Height): %.0f\n", areaEffectRuns, area.Get())
	}, area)
	fmt.Printf("  Initial area: %.0f\n", area.Get())
	fmt.Println()

	// Test 8: Change parent bounds to trigger area effect
	fmt.Println("Test 8: Changing parent bounds (triggers Area Effect)")
	parent.Left.Set(50)
	parent.Bottom.Set(600)
	fmt.Printf("  Parent bounds: (50, 0) to (1000, 600)\n")
	width := child.Right.Get() - child.Left.Get()
	height := child.Bottom.Get() - child.Top.Get()
	fmt.Printf("  Child: Left=%v, Top=%v, Width=%v, Height=%v\n",
		child.Left.Get(), child.Top.Get(), width, height)
	fmt.Printf("  Area: %.0f\n", area.Get())
	fmt.Printf("  Area Effect ran %d time(s)\n", areaEffectRuns)
	fmt.Println()
}
