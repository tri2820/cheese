package ui

import (
	"github.com/lithdew/casso"

	"github.com/tri2820/cheese/signals"
)

// Priority represents constraint strength (alias for casso.Priority)
type Priority = casso.Priority

const (
	Required Priority = casso.Required
	Strong   Priority = casso.Strong
	Weak     Priority = casso.Weak
)

// Solver wraps a casso.Solver with convenient methods for our types
type Solver struct {
	inner   *casso.Solver
	vars    map[casso.Symbol]*signals.Signal[float64] // symbol → signal
	symbols map[*signals.Signal[float64]]casso.Symbol // signal → symbol
}

// NewSolver creates a new constraint solver
func NewSolver() *Solver {
	return &Solver{
		inner:   casso.NewSolver(),
		vars:    make(map[casso.Symbol]*signals.Signal[float64]),
		symbols: make(map[*signals.Signal[float64]]casso.Symbol),
	}
}

// ConstraintHandle represents a handle to a constraint that can be removed
type ConstraintHandle struct {
	solver  *Solver
	markers []casso.Symbol
}

// Remove removes these constraints from the solver
func (h ConstraintHandle) Remove() {
	for _, marker := range h.markers {
		h.solver.inner.RemoveConstraint(marker)
	}
}

// Add adds constraints to the solver
// Accepts both single constraints and slices of constraints
// Uses the constraint's stored priority (defaults to Strong if not set)
// Returns a handle that can be used to remove the constraints
func (s *Solver) Add(constraints ...Constraints) ConstraintHandle {
	var markers []casso.Symbol
	for _, group := range constraints {
		for _, c := range group {
			priority := c.priority
			if priority == 0 {
				priority = Strong
			}
			marker, _ := s.inner.AddConstraintWithPriority(priority, c.ToCasso())
			markers = append(markers, marker)
		}
	}
	return ConstraintHandle{solver: s, markers: markers}
}

// AddWithPriority adds constraints with a specific priority (overrides stored priority)
// Returns a handle that can be used to remove the constraints
func (s *Solver) AddWithPriority(priority Priority, constraints ...Constraints) ConstraintHandle {
	var markers []casso.Symbol
	for _, group := range constraints {
		for _, c := range group {
			marker, _ := s.inner.AddConstraintWithPriority(priority, c.ToCasso())
			markers = append(markers, marker)
		}
	}
	return ConstraintHandle{solver: s, markers: markers}
}

// NewVar creates a new variable expression (signal + symbol pair)
func (s *Solver) NewVar() Expr {
	sym := casso.New()
	sig := signals.New(0.0)

	s.vars[sym] = sig
	s.symbols[sig] = sym

	// Capture symbol for closure
	symCopy := sym
	// Watch signal for changes - immediate resolve on every Set()
	sig.OnChange(func() {
		s.resolve(symCopy)
	})

	return Expr{kind: exprVar, symbol: sym, signal: sig}
}

// resolve syncs signals → casso → signals
// Runs immediately on every signal.Set()
func (s *Solver) resolve(changedSymbol casso.Symbol) {
	sig := s.vars[changedSymbol]
	// Edit with Weak priority so Strong constraints can override
	s.inner.Edit(changedSymbol, Weak)
	s.inner.Suggest(changedSymbol, sig.Get())

	// Get resolved values from casso and update signals
	// Use SetQuiet to avoid triggering onChange (prevents infinite loop)
	for sym, sig := range s.vars {
		newVal := s.inner.Val(sym)
		if newVal != sig.Get() {
			sig.SetQuiet(newVal)
		}
	}
}

// NewElement creates element via solver
func (s *Solver) NewElement() *Element {
	return &Element{
		solver: s,
		Left:   s.NewVar(),
		Right:  s.NewVar(),
		Top:    s.NewVar(),
		Bottom: s.NewVar(),
	}
}

// Inner returns the underlying casso.Solver for advanced use
func (s *Solver) Inner() *casso.Solver {
	return s.inner
}
