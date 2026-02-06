package casso_test

import (
	"github.com/lithdew/casso"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestConstraint(t *testing.T) {
	l := casso.New()
	m := casso.New()
	r := casso.New()

	a := casso.NewConstraint(casso.EQ, 0, r.T(1), l.T(1), m.T(-2))
	b := casso.NewConstraint(casso.GTE, -100, r.T(1), l.T(-1))
	c := casso.NewConstraint(casso.GTE, 0, l.T(1))

	s := casso.NewSolver()

	_, err := s.AddConstraint(a)
	require.NoError(t, err)

	_, err = s.AddConstraint(b)
	require.NoError(t, err)

	_, err = s.AddConstraint(c)
	require.NoError(t, err)

	require.EqualValues(t, 0, s.Val(l))
	require.EqualValues(t, 50, s.Val(m))
	require.EqualValues(t, 100, s.Val(r))
}

func TestRemoveConstraint(t *testing.T) {
	l := casso.New()
	r := casso.New()

	c1 := casso.NewConstraint(casso.GTE, -200, r.T(1), l.T(-1))
	c2 := casso.NewConstraint(casso.GTE, 0, r.T(1), l.T(-1))

	s := casso.NewSolver()

	c1t, err := s.AddConstraint(c1)
	require.NoError(t, err)

	c2t, err := s.AddConstraint(c2)
	require.NoError(t, err)

	require.NoError(t, s.RemoveConstraint(c1t))
	require.NoError(t, s.RemoveConstraint(c2t))
}

func TestEditableConstraint(t *testing.T) {
	s := casso.NewSolver()
	l := casso.New()
	m := casso.New()
	r := casso.New()

	a := casso.NewConstraint(casso.EQ, 0, r.T(1), l.T(1), m.T(-2))
	b := casso.NewConstraint(casso.GTE, -100, r.T(1), l.T(-1))
	c := casso.NewConstraint(casso.GTE, 0, l.T(1))

	_, err := s.AddConstraint(a)
	require.NoError(t, err)

	_, err = s.AddConstraint(b)
	require.NoError(t, err)

	_, err = s.AddConstraint(c)
	require.NoError(t, err)

	// Suggest that 'l' should have a value of 100.

	require.NoError(t, s.Edit(l, casso.Strong))
	require.NoError(t, s.Suggest(l, 100))

	require.EqualValues(t, 100, s.Val(l))
	require.EqualValues(t, 150, s.Val(m))
	require.EqualValues(t, 200, s.Val(r))
}

func TestConstraintRequiringArtificialVariable(t *testing.T) {
	s := casso.NewSolver()

	p1 := casso.New()
	p2 := casso.New()
	p3 := casso.New()

	container := casso.New()

	require.NoError(t, s.Edit(container, casso.Strong))
	require.NoError(t, s.Suggest(container, 100.0))

	c1 := casso.NewConstraint(casso.GTE, -30.0, p1.T(1.0))
	c2 := casso.NewConstraint(casso.EQ, 0, p1.T(1), p3.T(-1.0))
	c3 := casso.NewConstraint(casso.EQ, 0, p2.T(1.0), p1.T(-2.0))
	c4 := casso.NewConstraint(casso.EQ, 0.0, container.T(1.0), p1.T(-1.0), p2.T(-1.0), p3.T(-1.0))

	_, err := s.AddConstraintWithPriority(casso.Strong, c1)
	require.NoError(t, err)

	_, err = s.AddConstraintWithPriority(casso.Medium, c2)
	require.NoError(t, err)

	_, err = s.AddConstraint(c3)
	require.NoError(t, err)

	_, err = s.AddConstraint(c4)
	require.NoError(t, err)

	require.EqualValues(t, 30, s.Val(p1))
	require.EqualValues(t, 60, s.Val(p2))
	require.EqualValues(t, 10, s.Val(p3))
	require.EqualValues(t, 100, s.Val(container))
}

func TestPaddingUI(t *testing.T) {
	s := casso.NewSolver()

	sw := casso.New() // screen width
	sh := casso.New() // screen height

	padding := casso.New() // padding

	require.NoError(t, s.Edit(sw, casso.Strong))
	require.NoError(t, s.Edit(sh, casso.Strong))
	require.NoError(t, s.Edit(padding, casso.Strong))

	require.NoError(t, s.Suggest(sw, 800))
	require.NoError(t, s.Suggest(sh, 600))
	require.NoError(t, s.Suggest(padding, 30))

	r := func(c casso.Constraint) {
		_, err := s.AddConstraint(c)
		require.NoError(t, err)
	}

	x := casso.New()
	y := casso.New()
	w := casso.New()
	h := casso.New()

	// x >= padding
	// x + width + padding <= screen_width - 1
	// y >= padding
	// y + height + padding <= screen_height - 1

	c1 := casso.NewConstraint(casso.GTE, 0, x.T(1), padding.T(-1))
	c2 := casso.NewConstraint(casso.LTE, 1, x.T(1), w.T(1), padding.T(1), sw.T(-1))
	c3 := casso.NewConstraint(casso.GTE, 0, y.T(1), padding.T(-1))
	c4 := casso.NewConstraint(casso.LTE, 1, y.T(1), h.T(1), padding.T(1), sh.T(-1))

	r(c1)
	r(c2)
	r(c3)
	r(c4)

	require.EqualValues(t, 30, s.Val(x))
	require.EqualValues(t, 30, s.Val(y))
	require.EqualValues(t, 739, s.Val(w))
	require.EqualValues(t, 539, s.Val(h))

	require.NoError(t, s.Suggest(padding, 50))

	require.EqualValues(t, 50, s.Val(x))
	require.EqualValues(t, 50, s.Val(y))
	require.EqualValues(t, 699, s.Val(w))
	require.EqualValues(t, 499, s.Val(h))
}

func TestComplexConstraints(t *testing.T) {
	s := casso.NewSolver()

	containerWidth := casso.New()

	childX := casso.New()
	childCompWidth := casso.New()

	child2X := casso.New()
	child2CompWidth := casso.New()

	c1 := casso.NewConstraint(casso.EQ, 0, childX.T(1.0), containerWidth.T(-50.0/1024))
	c2 := casso.NewConstraint(casso.EQ, 0, childCompWidth.T(1.0), containerWidth.T(-200.0/1024))
	c3 := casso.NewConstraint(casso.GTE, -200, childCompWidth.T(1.0))
	c4 := casso.NewConstraint(casso.EQ, -50, child2X.T(1.0), childX.T(-1.0), childCompWidth.T(-1.0))
	c5 := casso.NewConstraint(casso.EQ, 50, child2CompWidth.T(1.0), containerWidth.T(-1.0), child2X.T(1.0))

	require.NoError(t, s.Edit(containerWidth, casso.Strong))
	require.NoError(t, s.Suggest(containerWidth, 2048))

	_, err := s.AddConstraint(c1)
	require.NoError(t, err)

	_, err = s.AddConstraintWithPriority(casso.Weak, c2)
	require.NoError(t, err)

	_, err = s.AddConstraintWithPriority(casso.Strong, c3)
	require.NoError(t, err)

	_, err = s.AddConstraint(c4)
	require.NoError(t, err)

	_, err = s.AddConstraint(c5)
	require.NoError(t, err)

	require.EqualValues(t, 2048, s.Val(containerWidth))
	require.EqualValues(t, 400, s.Val(childCompWidth))
	require.EqualValues(t, 1448, s.Val(child2CompWidth))

	require.NoError(t, s.Suggest(containerWidth, 500))

	require.EqualValues(t, 500, s.Val(containerWidth))
	require.EqualValues(t, 200, s.Val(childCompWidth))
	require.EqualValues(t, 175.5859375, s.Val(child2CompWidth))
}

func BenchmarkAddConstraint(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		s := casso.NewSolver()
		l := casso.New()
		m := casso.New()
		r := casso.New()
		a := casso.NewConstraint(casso.EQ, 0, l.T(1), r.T(1), m.T(-2))
		b := casso.NewConstraint(casso.GTE, -10, r.T(1), l.T(-1))
		_, _ = s.AddConstraint(a)
		_, _ = s.AddConstraint(b)
	}
}

// TestRemoveVariable_Basic tests removing a variable with a single constraint.
func TestRemoveVariable_Basic(t *testing.T) {
	s := casso.NewSolver()
	x := casso.New()

	// Add a simple constraint: x = 10
	c1 := x.EQ(10)
	_, err := s.AddConstraint(c1)
	require.NoError(t, err)

	require.EqualValues(t, 10, s.Val(x))

	// Remove x - should remove the constraint
	err = s.RemoveVariable(x)
	require.NoError(t, err)

	// x should now be 0 (no constraint)
	require.EqualValues(t, 0, s.Val(x))
}

// TestRemoveVariable_MultipleConstraints tests removing a variable involved in multiple constraints.
func TestRemoveVariable_MultipleConstraints(t *testing.T) {
	s := casso.NewSolver()
	x := casso.New()
	y := casso.New()

	// Add multiple constraints involving x
	_, err := s.AddConstraint(x.EQ(10))
	require.NoError(t, err)

	_, err = s.AddConstraint(casso.NewConstraint(casso.GTE, 0, y.T(1), x.T(-1)))
	require.NoError(t, err)

	_, err = s.AddConstraint(x.LTE(20))
	require.NoError(t, err)

	require.EqualValues(t, 10, s.Val(x))
	require.EqualValues(t, 10, s.Val(y))

	// Remove x - should remove all three constraints
	err = s.RemoveVariable(x)
	require.NoError(t, err)

	// Both variables should now be 0 (no constraints)
	require.EqualValues(t, 0, s.Val(x))
	require.EqualValues(t, 0, s.Val(y))
}

// TestRemoveVariable_DependentVariables tests removal affecting dependent variables.
func TestRemoveVariable_DependentVariables(t *testing.T) {
	s := casso.NewSolver()
	x := casso.New()
	y := casso.New()
	z := casso.New()

	// y = x * 2
	// z = y + 5
	// x = 10
	_, err := s.AddConstraint(casso.NewConstraint(casso.EQ, 0, y.T(1), x.T(-2)))
	require.NoError(t, err)

	_, err = s.AddConstraint(casso.NewConstraint(casso.EQ, -5, z.T(1), y.T(-1)))
	require.NoError(t, err)

	_, err = s.AddConstraint(x.EQ(10))
	require.NoError(t, err)

	require.EqualValues(t, 10, s.Val(x))
	require.EqualValues(t, 20, s.Val(y))
	require.EqualValues(t, 25, s.Val(z))

	// Remove x - removes constraints directly mentioning x (y=x*2 and x=10)
	err = s.RemoveVariable(x)
	require.NoError(t, err)

	// x is unconstrained (no constraints left)
	require.EqualValues(t, 0, s.Val(x))
	// y still has constraint z-y=-5 (which doesn't mention x), so y = -5
	require.EqualValues(t, -5, s.Val(y))
	// z is unconstrained (not in tableau)
	require.EqualValues(t, 0, s.Val(z))
}

// TestRemoveVariable_EditVariable tests removing a variable that's registered as editable.
func TestRemoveVariable_EditVariable(t *testing.T) {
	s := casso.NewSolver()
	x := casso.New()
	y := casso.New()

	// Make x editable
	err := s.Edit(x, casso.Strong)
	require.NoError(t, err)

	err = s.Suggest(x, 100)
	require.NoError(t, err)

	// Add a constraint involving x
	_, err = s.AddConstraint(casso.NewConstraint(casso.GTE, 0, y.T(1), x.T(-1)))
	require.NoError(t, err)

	require.EqualValues(t, 100, s.Val(x))
	require.EqualValues(t, 100, s.Val(y))

	// Remove x - should remove both the constraint and the edit
	err = s.RemoveVariable(x)
	require.NoError(t, err)

	// x should be unconstrained
	require.EqualValues(t, 0, s.Val(x))
	require.EqualValues(t, 0, s.Val(y))

	// Suggesting x again should fail (edit was removed)
	err = s.Suggest(x, 50)
	require.Error(t, err)
}

// TestRemoveVariable_NonExistent tests removing a variable that doesn't exist.
func TestRemoveVariable_NonExistent(t *testing.T) {
	s := casso.NewSolver()
	x := casso.New()
	y := casso.New() // Never added to any constraint

	// Add constraint for x only
	_, err := s.AddConstraint(x.EQ(10))
	require.NoError(t, err)

	// Remove y (which has no constraints) - should succeed
	err = s.RemoveVariable(y)
	require.NoError(t, err)

	// x should still be constrained
	require.EqualValues(t, 10, s.Val(x))
}

// TestRemoveVariable_ComplexUI tests removing variables in a complex UI layout scenario.
func TestRemoveVariable_ComplexUI(t *testing.T) {
	s := casso.NewSolver()

	sw := casso.New() // screen width
	sh := casso.New() // screen height
	padding := casso.New() // padding

	require.NoError(t, s.Edit(sw, casso.Strong))
	require.NoError(t, s.Edit(sh, casso.Strong))
	require.NoError(t, s.Edit(padding, casso.Strong))

	require.NoError(t, s.Suggest(sw, 800))
	require.NoError(t, s.Suggest(sh, 600))
	require.NoError(t, s.Suggest(padding, 30))

	x := casso.New()
	y := casso.New()
	w := casso.New()
	h := casso.New()

	// Create UI constraints
	c1 := casso.NewConstraint(casso.GTE, 0, x.T(1), padding.T(-1))
	c2 := casso.NewConstraint(casso.LTE, 1, x.T(1), w.T(1), padding.T(1), sw.T(-1))
	c3 := casso.NewConstraint(casso.GTE, 0, y.T(1), padding.T(-1))
	c4 := casso.NewConstraint(casso.LTE, 1, y.T(1), h.T(1), padding.T(1), sh.T(-1))

	_, err := s.AddConstraint(c1)
	require.NoError(t, err)
	_, err = s.AddConstraint(c2)
	require.NoError(t, err)
	_, err = s.AddConstraint(c3)
	require.NoError(t, err)
	_, err = s.AddConstraint(c4)
	require.NoError(t, err)

	// Verify initial layout
	require.EqualValues(t, 30, s.Val(x))
	require.EqualValues(t, 30, s.Val(y))
	require.EqualValues(t, 739, s.Val(w))
	require.EqualValues(t, 539, s.Val(h))

	// Remove padding - should remove all constraints that reference it
	err = s.RemoveVariable(padding)
	require.NoError(t, err)

	// All UI elements should now be unconstrained
	require.EqualValues(t, 0, s.Val(x))
	require.EqualValues(t, 0, s.Val(y))
	require.EqualValues(t, 0, s.Val(w))
	require.EqualValues(t, 0, s.Val(h))

	// sw and sh should still be editable (they had Edit constraints, not regular ones)
	require.EqualValues(t, 800, s.Val(sw))
	require.EqualValues(t, 600, s.Val(sh))
}

// TestRemoveVariable_PartialRemoval tests removing one variable from a multi-variable system.
func TestRemoveVariable_PartialRemoval(t *testing.T) {
	s := casso.NewSolver()
	x := casso.New()
	y := casso.New()
	z := casso.New()

	// x = 10
	_, err := s.AddConstraint(x.EQ(10))
	require.NoError(t, err)

	// y = 20
	_, err = s.AddConstraint(y.EQ(20))
	require.NoError(t, err)

	// z >= x + y
	_, err = s.AddConstraint(casso.NewConstraint(casso.GTE, 0, z.T(1), x.T(-1), y.T(-1)))
	require.NoError(t, err)

	require.EqualValues(t, 10, s.Val(x))
	require.EqualValues(t, 20, s.Val(y))
	require.EqualValues(t, 30, s.Val(z))

	// Remove x - should remove two constraints (x.EQ and z >= x + y)
	err = s.RemoveVariable(x)
	require.NoError(t, err)

	// x should be unconstrained
	require.EqualValues(t, 0, s.Val(x))

	// y should still be constrained
	require.EqualValues(t, 20, s.Val(y))

	// z should now be unconstrained (the constraint involving x was removed)
	require.EqualValues(t, 0, s.Val(z))
}

// TestRemoveVariable_ChainedConstraints tests removing variables in a chain of dependencies.
func TestRemoveVariable_ChainedConstraints(t *testing.T) {
	s := casso.NewSolver()
	a := casso.New()
	b := casso.New()
	c := casso.New()
	d := casso.New()

	// a = 1
	_, err := s.AddConstraint(a.EQ(1))
	require.NoError(t, err)

	// b = a + 1
	_, err = s.AddConstraint(casso.NewConstraint(casso.EQ, -1, b.T(1), a.T(-1)))
	require.NoError(t, err)

	// c = b + 1
	_, err = s.AddConstraint(casso.NewConstraint(casso.EQ, -1, c.T(1), b.T(-1)))
	require.NoError(t, err)

	// d = c + 1
	_, err = s.AddConstraint(casso.NewConstraint(casso.EQ, -1, d.T(1), c.T(-1)))
	require.NoError(t, err)

	require.EqualValues(t, 1, s.Val(a))
	require.EqualValues(t, 2, s.Val(b))
	require.EqualValues(t, 3, s.Val(c))
	require.EqualValues(t, 4, s.Val(d))

	// Remove b - removes constraints directly mentioning b (b=a+1 and c=b+1)
	err = s.RemoveVariable(b)
	require.NoError(t, err)

	require.EqualValues(t, 1, s.Val(a)) // a still has its constraint
	require.EqualValues(t, 0, s.Val(b)) // b is unconstrained (constraints removed)
	// c still has constraint d-c=-1 (which doesn't mention b), so c = -1
	require.EqualValues(t, -1, s.Val(c))
	// d is unconstrained (not in tableau)
	require.EqualValues(t, 0, s.Val(d))
}

// TestRemoveVariable_OnlyRemovesReferencedConstraints tests that only constraints
// referencing the removed variable are deleted.
func TestRemoveVariable_OnlyRemovesReferencedConstraints(t *testing.T) {
	s := casso.NewSolver()
	x := casso.New()
	y := casso.New()
	z := casso.New()

	// x = 10
	_, _ = s.AddConstraint(x.EQ(10))

	// y = 20 (independent of x)
	_, _ = s.AddConstraint(y.EQ(20))

	// z = x + y (references both x and y)
	_, _ = s.AddConstraint(casso.NewConstraint(casso.EQ, 0, z.T(1), x.T(-1), y.T(-1)))

	require.EqualValues(t, 10, s.Val(x))
	require.EqualValues(t, 20, s.Val(y))
	require.EqualValues(t, 30, s.Val(z))

	// Remove x - should remove c1 and c3, but not c2
	err := s.RemoveVariable(x)
	require.NoError(t, err)

	require.EqualValues(t, 0, s.Val(x))
	require.EqualValues(t, 20, s.Val(y)) // y's constraint should remain
	require.EqualValues(t, 0, s.Val(z)) // z's constraint was removed (it referenced x)
}
