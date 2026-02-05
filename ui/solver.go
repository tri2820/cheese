package ui

import "github.com/lithdew/casso"

// Priority represents constraint strength (alias for casso.Priority)
type Priority = casso.Priority

const (
	Required Priority = casso.Required
	Strong   Priority = casso.Strong
	Weak     Priority = casso.Weak
)

// Solver wraps a casso.Solver with convenient methods for our types
type Solver struct {
	inner *casso.Solver
}

// NewSolver creates a new constraint solver
func NewSolver() *Solver {
	return &Solver{inner: casso.NewSolver()}
}

// Add adds constraints to the solver
// Accepts both single constraints and slices of constraints
// Uses the constraint's stored priority (zero/Strong is default)
func (s *Solver) Add(constraints ...Constraints) {
	for _, group := range constraints {
		for _, c := range group {
			s.inner.AddConstraintWithPriority(c.priority, c.ToCasso())
		}
	}
}

// AddWithPriority adds constraints with a specific priority (overrides stored priority)
func (s *Solver) AddWithPriority(priority Priority, constraints ...Constraints) {
	for _, group := range constraints {
		for _, c := range group {
			s.inner.AddConstraintWithPriority(priority, c.ToCasso())
		}
	}
}

// Set sets a value for an expression's variable with a priority
// Combines Edit + Suggest: marks variable as editable and suggests a value
// Priority can be Strong or Weak. Required is treated as Strong (Edit doesn't support Required).
func (s *Solver) Set(expr Expr, value float64, priority Priority) error {
	// Edit doesn't support Required, treat it as Strong
	editPriority := priority
	if priority == Required {
		editPriority = Strong
	}
	if err := s.inner.Edit(expr.Symbol(), editPriority); err != nil {
		return err
	}
	return s.inner.Suggest(expr.Symbol(), value)
}

// Val returns the computed value for an expression's variable
func (s *Solver) Val(expr Expr) float64 {
	return s.inner.Val(expr.Symbol())
}

// Inner returns the underlying casso.Solver for advanced use
func (s *Solver) Inner() *casso.Solver {
	return s.inner
}
