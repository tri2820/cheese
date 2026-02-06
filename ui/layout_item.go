package ui

// withSolver attaches the layout to all constraints
func withSolver(layout *Layout, c Constraints) Constraints {
	for i := range c {
		c[i].layout = layout
	}
	return c
}

// Point represents a 2D point with X and Y expressions
type Point struct {
	X, Y Expr
}

// LayoutItem represents a UI layoutitem with layout properties
type LayoutItem struct {
	// Reference to layout that created this layoutitem
	layout *Layout
	// Stored as Expr fields
	Left   Expr
	Right  Expr
	Top    Expr
	Bottom Expr
}

// Width returns the computed width: Right - Left
func (e *LayoutItem) Width() Expr {
	return e.Right.Sub(e.Left)
}

// Height returns the computed height: Bottom - Top
func (e *LayoutItem) Height() Expr {
	return e.Bottom.Sub(e.Top)
}

// CenterX returns the horizontal center: Left + Width/2
func (e *LayoutItem) CenterX() Expr {
	return e.Left.Add(e.Width().Div(2))
}

// CenterY returns the vertical center: Top + Height/2
func (e *LayoutItem) CenterY() Expr {
	return e.Top.Add(e.Height().Div(2))
}

// Center returns both center coordinates
func (e *LayoutItem) Center() Point {
	return Point{X: e.CenterX(), Y: e.CenterY()}
}

// Position returns both position coordinates (Left, Top)
func (e *LayoutItem) Position() Point {
	return Point{X: e.Left, Y: e.Top}
}

// Size returns both size dimensions (Width, Height)
func (e *LayoutItem) Size() Point {
	return Point{X: e.Width(), Y: e.Height()}
}

// Inside returns constraints ensuring this layoutitem is fully inside another
// Returns Constraints for: Left>=other.Left, Right<=other.Right, Top>=other.Top, Bottom<=other.Bottom
func (e *LayoutItem) Inside(other *LayoutItem) Constraints {
	return withSolver(e.layout, append(
		append(Gte(e.Left, other.Left), Lte(e.Right, other.Right)...),
		append(Gte(e.Top, other.Top), Lte(e.Bottom, other.Bottom)...)...,
	))
}

// Outside returns constraints ensuring this layoutitem is completely to the left of another
// (most common interpretation of "outside")
// For other directions, use LeftOf, RightOf, Above, Below
func (e *LayoutItem) Outside(other *LayoutItem) Constraints {
	return withSolver(e.layout, Lte(e.Right, other.Left))
}

// LeftOf returns constraint: this layoutitem's right edge is at or before other's left edge
func (e *LayoutItem) LeftOf(other *LayoutItem, gap float64) Constraints {
	if gap == 0 {
		return withSolver(e.layout, Lte(e.Right, other.Left))
	}
	return withSolver(e.layout, Lte(e.Right.Sub(gap), other.Left))
}

// RightOf returns constraint: this layoutitem's left edge is at or after other's right edge
func (e *LayoutItem) RightOf(other *LayoutItem, gap float64) Constraints {
	if gap == 0 {
		return withSolver(e.layout, Gte(e.Left, other.Right))
	}
	return withSolver(e.layout, Gte(e.Left, other.Right.Add(gap)))
}

// Above returns constraint: this layoutitem's bottom edge is at or before other's top edge
func (e *LayoutItem) Above(other *LayoutItem, gap float64) Constraints {
	if gap == 0 {
		return withSolver(e.layout, Lte(e.Bottom, other.Top))
	}
	return withSolver(e.layout, Lte(e.Bottom.Sub(gap), other.Top))
}

// Below returns constraint: this layoutitem's top edge is at or after other's bottom edge
func (e *LayoutItem) Below(other *LayoutItem, gap float64) Constraints {
	if gap == 0 {
		return withSolver(e.layout, Gte(e.Top, other.Bottom))
	}
	return withSolver(e.layout, Gte(e.Top, other.Bottom.Add(gap)))
}

// NearX constrains this layoutitem's horizontal position to be within maxDistance of another
// Bounds the X axis distance between centers
func (e *LayoutItem) NearX(other *LayoutItem, maxDistance float64) Constraints {
	return withSolver(e.layout, Near(e.CenterX(), other.CenterX(), maxDistance))
}

// NearY constrains this layoutitem's vertical position to be within maxDistance of another
// Bounds the Y axis distance between centers
func (e *LayoutItem) NearY(other *LayoutItem, maxDistance float64) Constraints {
	return withSolver(e.layout, Near(e.CenterY(), other.CenterY(), maxDistance))
}

// Near constrains this layoutitem to be within maxDistance of another (both axes)
func (e *LayoutItem) Near(other *LayoutItem, maxDistance float64) Constraints {
	return append(e.NearX(other, maxDistance), e.NearY(other, maxDistance)...)
}

// Draw renders the layoutitem to the framebuffer.
// Default implementation does nothing; embedded types can override this.
func (e *LayoutItem) Draw(fb Framebuffer, dpi float64) {
	// Default: draw nothing
}

// GetLayoutItem returns the layoutitem itself (for Drawable interface).
func (e *LayoutItem) GetLayoutItem() *LayoutItem {
	return e
}
