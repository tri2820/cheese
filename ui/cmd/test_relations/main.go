package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Test: Relational Constraints ===")
	fmt.Println()

	layout := ui.NewLayout()
	parent := layout.NewView()
	child1 := layout.NewView()
	child2 := layout.NewView()
	child3 := layout.NewView()

	// Set parent bounds
	parent.Left.Set(0)
	parent.Right.Set(1000)
	parent.Top.Set(0)
	parent.Bottom.Set(500)

	// Test 1: Inside
	fmt.Println("Test 1: Inside(parent)")
	child1.Inside(parent).Add()
	child1.Left.Set(50)
	child1.Top.Set(50)
	child1.Right.Set(200)
	child1.Bottom.Set(200)
	fmt.Printf("  Child1 inside parent: (%.0f, %.0f) to (%.0f, %.0f)\n",
		child1.Left.Get(), child1.Top.Get(), child1.Right.Get(), child1.Bottom.Get())
	fmt.Println()

	// Test 2: RightOf and LeftOf
	fmt.Println("Test 2: RightOf(parent, gap)")
	child2.RightOf(parent, 20).Add()
	ui.Eq(child2.Top, parent.Top.Add(50)).Add()
	child2.Left.Set(0)
	child2.Top.Set(0)
	child2.Right.Set(150)
	child2.Bottom.Set(150)
	fmt.Printf("  Child2 right of parent (gap=20): Left=%.0f (should be %.0f)\n",
		child2.Left.Get(), parent.Right.Get()+20)
	fmt.Println()

	// Test 3: Below and Above
	fmt.Println("Test 3: Below(parent, gap)")
	child3.Below(parent, 30).Add()
	ui.Eq(child3.Left, parent.Left.Add(50)).Add()
	child3.Left.Set(0)
	child3.Top.Set(0)
	child3.Right.Set(100)
	child3.Bottom.Set(100)
	fmt.Printf("  Child3 below parent (gap=30): Top=%.0f (should be %.0f)\n",
		child3.Top.Get(), parent.Bottom.Get()+30)
	fmt.Println()

	// Test 4: Near (centers close together)
	fmt.Println("Test 4: NearX/NearY (centers within distance)")
	near1 := layout.NewView()
	near2 := layout.NewView()

	ui.Eq(near1.Left, 100).Add()
	ui.Eq(near1.Top, 100).Add()
	ui.Eq(near1.Width(), 80).Add()
	ui.Eq(near1.Height(), 80).Add()

	near1.NearX(near2, 10).Add()                           // Centers within 10px
	ui.Eq(near2.CenterX(), near1.CenterX().Add(200)).Add() // Far apart
	ui.Eq(near2.Width(), 60).Add()
	ui.Eq(near2.Height(), 60).Add()

	fmt.Printf("  Near1 center: (%.0f, %.0f)\n", near1.CenterX().Get(), near1.CenterY().Get())
	fmt.Printf("  Near2 center: (%.0f, %.0f)\n", near2.CenterX().Get(), near2.CenterY().Get())
	fmt.Printf("  X distance: %.0f (constraint: ≤10)\n", near2.CenterX().Get()-near1.CenterX().Get())
	fmt.Println()

	// Test 5: Point equality with Eq()
	fmt.Println("Test 5: Eq(Point, Point) → equal X and Y")
	p1 := ui.Point{X: child1.Center().X, Y: child1.Center().Y}
	p2 := ui.Point{X: child2.Center().X, Y: child2.Center().Y}
	ui.Eq(p1, p2).Add()

	fmt.Printf("  Child1 center: (%.0f, %.0f)\n", child1.CenterX().Get(), child1.CenterY().Get())
	fmt.Printf("  Child2 center: (%.0f, %.0f)\n", child2.CenterX().Get(), child2.CenterY().Get())
	fmt.Println()

	fmt.Println("=== Relational Constraint Summary ===")
	fmt.Println("  a.Inside(b)           → a fully inside b")
	fmt.Println("  a.RightOf(b, gap)     → a's left ≥ b's right + gap")
	fmt.Println("  a.LeftOf(b, gap)      → a's right ≤ b's left - gap")
	fmt.Println("  a.Below(b, gap)       → a's top ≥ b's bottom + gap")
	fmt.Println("  a.Above(b, gap)       → a's bottom ≤ b's top - gap")
	fmt.Println("  a.NearX(b, dist)      → |a.CenterX - b.CenterX| ≤ dist")
	fmt.Println("  a.NearY(b, dist)      → |a.CenterY - b.CenterY| ≤ dist")
	fmt.Println("  a.Near(b, dist)       → NearX AND NearY")
	fmt.Println("  Eq(Point, Point)      → Equal X AND equal Y")
}
