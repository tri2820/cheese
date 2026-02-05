package main

import (
	"fmt"

	"github.com/tri2820/cheese/ui"
)

func main() {
	// Create elements
	parent := ui.NewElement()
	child := ui.NewElement()

	fmt.Println("=== UI Kit Expression & Constraint Demo ===")
	fmt.Println()

	// Example 1: Simple positioning
	fmt.Println("Example 1: Position child 10px from parent's left edge")
	c1 := ui.Eq(child.Left, parent.Left.Add(10))
	fmt.Printf("  ui.Eq(child.Left, parent.Left.Add(10))\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c1))
	fmt.Printf("  → %s\n", c1[0])
	fmt.Printf("  → Casso: %v\n\n", c1[0].ToCasso())

	// Example 2: Fixed width
	fmt.Println("Example 2: Fixed child width of 100px")
	c2 := ui.Eq(child.Width(), 100)
	fmt.Printf("  ui.Eq(child.Width(), 100)\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c2))
	fmt.Printf("  → %s\n", c2[0])
	fmt.Printf("  → (child.Right - child.Left) == 100\n\n")

	// Example 3: Center alignment with Point
	fmt.Println("Example 3: Center child in parent using Point")
	c3 := ui.Eq(child.Center(), parent.Center())
	fmt.Printf("  ui.Eq(child.Center(), parent.Center())\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c3))
	fmt.Printf("  → %s\n", c3[0])
	fmt.Printf("  → %s\n", c3[1])
	fmt.Printf("  (constraints for X and Y)\n\n")

	// Example 4: Center alignment - individual axes
	fmt.Println("Example 4: Center child horizontally only")
	c4 := ui.Eq(child.CenterX(), parent.CenterX())
	fmt.Printf("  ui.Eq(child.CenterX(), parent.CenterX())\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c4))
	fmt.Printf("  → %s\n\n", c4[0])

	// Example 5: Minimum width constraint
	fmt.Println("Example 5: Minimum width of 50px")
	c5 := ui.Gte(child.Width(), 50)
	fmt.Printf("  ui.Gte(child.Width(), 50)\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n\n", len(c5))

	// Example 6: Chain multiple operations
	fmt.Println("Example 6: Complex chained expression")
	c6 := ui.Eq(child.Left, parent.Left.Add(parent.Width().Div(4)).Add(10))
	fmt.Printf("  ui.Eq(child.Left, parent.Left.Add(parent.Width().Div(4)).Add(10))\n")
	fmt.Printf("  → %s\n\n", c6[0])

	// Example 6b: Multiple equal values
	fmt.Println("Example 6b: Multiple elements with equal width")
	another := ui.NewElement()
	c6b := ui.Eq(child.Width(), another.Width(), parent.Width().Mul(0.5))
	fmt.Printf("  ui.Eq(child.Width(), another.Width(), parent.Width().Mul(0.5))\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c6b))
	fmt.Printf("  All equal: child.Width == another.Width == parent.Width * 0.5\n\n")

	// Example 6c: Inside helper
	fmt.Println("Example 6c: Child inside parent")
	c6c := child.Inside(parent)
	fmt.Printf("  child.Inside(parent)\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c6c))
	fmt.Printf("  Ensures: child.Left >= parent.Left\n")
	fmt.Printf("           child.Right <= parent.Right\n")
	fmt.Printf("           child.Top >= parent.Top\n")
	fmt.Printf("           child.Bottom <= parent.Bottom\n\n")

	// Example 6d: LeftOf helper
	fmt.Println("Example 6d: Child positioned to the left of parent with gap")
	c6d := child.LeftOf(parent, 10)
	fmt.Printf("  child.LeftOf(parent, 10)\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c6d))
	fmt.Printf("  Ensures: child.Right - 10 <= parent.Left\n\n")

	// Example 6e: Below helper
	fmt.Println("Example 6e: Child positioned below parent with gap")
	c6e := child.Below(parent, 20)
	fmt.Printf("  child.Below(parent, 20)\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c6e))
	fmt.Printf("  Ensures: child.Top >= parent.Bottom + 20\n\n")

	// Example 6f: Percentage helper
	fmt.Println("Example 6f: Percentage helper")
	c6f := ui.Eq(child.Width(), ui.Percent(parent.Width(), 50))
	fmt.Printf("  ui.Eq(child.Width(), ui.Percent(parent.Width(), 50))\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c6f))
	fmt.Printf("  child.Width() == parent.Width() * 0.5\n\n")

	// Example 6g: Between helper
	fmt.Println("Example 6g: Between (range) helper")
	c6g := ui.Between(child.Width(), 50, 200)
	fmt.Printf("  ui.Between(child.Width(), 50, 200)\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c6g))
	fmt.Printf("  Ensures: child.Width() >= 50 AND child.Width() <= 200\n\n")

	// Example 6h: AspectRatio helper
	fmt.Println("Example 6h: Aspect ratio helper")
	c6h := ui.AspectRatio(child, 16, 9)
	fmt.Printf("  ui.AspectRatio(child, 16, 9)\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c6h))
	fmt.Printf("  Ensures: child.Width() * 9 == child.Height() * 16\n\n")

	// Example 6i: Near helper
	fmt.Println("Example 6i: Near - keep element within range of another")
	c6i := ui.Near(child.Left, parent.Left, 100)
	fmt.Printf("  ui.Near(child.Left, parent.Left, 100)\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c6i))
	fmt.Printf("  Ensures: |child.Left - parent.Left| <= 100\n")
	fmt.Printf("  (child.Left is within 100px of parent.Left)\n\n")

	// Example 6j: Element Near helper
	fmt.Println("Example 6j: Element Near helper")
	c6j := child.Near(parent, 50)
	fmt.Printf("  child.Near(parent, 50)\n")
	fmt.Printf("  → returns []Constraint (len=%d)\n", len(c6j))
	fmt.Printf("  Ensures: child is within 50px of parent (on both axes)\n\n")

	// Example 7: Using with a real solver
	fmt.Println("Example 7: Solving constraints with wrapped solver")
	solver := ui.NewSolver()

	// Add constraints to solver (handles []Constraint automatically)
	solver.Add(c1, c2, c3, c5)

	// Set parent dimensions
	solver.Set(parent.Left, 0, ui.Required)
	solver.Set(parent.Right, 1000, ui.Required)
	solver.Set(parent.Top, 0, ui.Required)
	solver.Set(parent.Bottom, 500, ui.Required)

	// Get computed values
	fmt.Printf("  Parent: Left=%v, Top=%v, Right=%v, Bottom=%v\n",
		solver.Val(parent.Left),
		solver.Val(parent.Top),
		solver.Val(parent.Right),
		solver.Val(parent.Bottom))
	fmt.Printf("  Child:  Left=%v, Top=%v, Right=%v, Bottom=%v\n",
		solver.Val(child.Left),
		solver.Val(child.Top),
		solver.Val(child.Right),
		solver.Val(child.Bottom))

	fmt.Println("\n=== API Summary ===")
	fmt.Println("Fields (stored):  element.Left, element.Right, element.Top, element.Bottom")
	fmt.Println("Methods (computed): element.Width(), element.Height(), element.CenterX(), element.CenterY()")
	fmt.Println("                   element.Center(), element.Position(), element.Size()")
	fmt.Println("                   element.Inside(other), element.Outside(other)")
	fmt.Println("                   element.LeftOf(other, gap), element.RightOf(other, gap)")
	fmt.Println("                   element.Above(other, gap), element.Below(other, gap)")
	fmt.Println("                   element.Near(other, distance), element.NearX(other, d), element.NearY(other, d)")
	fmt.Println("Helpers:           ui.Percent(expr, pct)")
	fmt.Println("                   ui.Between(expr, min, max), ui.Near(expr, expr, max)")
	fmt.Println("                   ui.AspectRatio(element, wRatio, hRatio)")
	fmt.Println("Constraints:       ui.Eq(a, b, ...) returns []Constraint")
	fmt.Println("                   Supports Point: Eq(child.Center(), parent.Center()) → 2 constraints")
	fmt.Println("Operations:        expr.Add(v), expr.Sub(v), expr.Mul(v), expr.Div(v)")
	fmt.Println("                   expr.Add(other), expr.Sub(other)")
	fmt.Println("Types accepted:    Expr, float64, int, Point")
}
