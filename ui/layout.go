package ui

import (
	"github.com/lithdew/casso"
)

// Priority represents constraint strength (alias for casso.Priority)
type Priority = casso.Priority

const (
	Required Priority = casso.Required
	Strong   Priority = casso.Strong
	Weak     Priority = casso.Weak
)

// Layout wraps a casso.Solver with convenient methods for our types
type Layout struct {
	inner *casso.Solver
	vars  map[casso.Symbol]*exprState // symbol → shared state
}

// NewLayout creates a new constraint solver
func NewLayout() *Layout {
	return &Layout{
		inner: casso.NewSolver(),
		vars:  make(map[casso.Symbol]*exprState),
	}
}

// ConstraintHandle represents a handle to a constraint that can be removed
type ConstraintHandle struct {
	layout  *Layout
	markers []casso.Symbol
}

// Remove removes these constraints from the solver
func (h ConstraintHandle) Remove() {
	for _, marker := range h.markers {
		h.layout.inner.RemoveConstraint(marker)
	}
}

// Add adds constraints to the solver
// Accepts both single constraints and slices of constraints
// Uses the constraint's stored priority (defaults to Strong if not set)
// Returns a handle that can be used to remove the constraints
func (l *Layout) Add(constraints ...Constraints) ConstraintHandle {
	var markers []casso.Symbol
	for _, group := range constraints {
		for _, c := range group {
			priority := c.priority
			if priority == 0 {
				priority = Strong
			}
			marker, _ := l.inner.AddConstraintWithPriority(priority, c.ToCasso())
			markers = append(markers, marker)
		}
	}
	return ConstraintHandle{layout: l, markers: markers}
}

// AddWithPriority adds constraints with a specific priority (overrides stored priority)
// Returns a handle that can be used to remove the constraints
func (l *Layout) AddWithPriority(priority Priority, constraints ...Constraints) ConstraintHandle {
	var markers []casso.Symbol
	for _, group := range constraints {
		for _, c := range group {
			marker, _ := l.inner.AddConstraintWithPriority(priority, c.ToCasso())
			markers = append(markers, marker)
		}
	}
	return ConstraintHandle{layout: l, markers: markers}
}

// NewVar creates a new variable expression
func (l *Layout) NewVar() Expr {
	state := &exprState{
		symbol: casso.New(),
		value:  0,
	}
	l.vars[state.symbol] = state

	// Capture symbol for closure
	symCopy := state.symbol
	// Watch for changes - immediate resolve on every Set()
	state.onChange = append(state.onChange, func() {
		l.resolve(symCopy)
	})

	return Expr{kind: exprVar, state: state, layout: l}
}

// resolve syncs expr values → casso → expr values
// Runs immediately on every Expr.Set()
func (l *Layout) resolve(changedSymbol casso.Symbol) {
	state := l.vars[changedSymbol]
	// Edit with Weak priority so Strong constraints can override
	l.inner.Edit(changedSymbol, Weak)
	l.inner.Suggest(changedSymbol, state.value)

	// Get resolved values from casso and update expressions
	// Use SetQuiet to avoid triggering OnChange (prevents infinite loop)
	for sym, state := range l.vars {
		newVal := l.inner.Val(sym)
		if newVal != state.value {
			state.value = newVal
			// Trigger quiet observers only
			for _, fn := range state.onChangeQuiet {
				fn()
			}
		}
	}
}

// NewElement creates element via layout
func (l *Layout) NewElement() *Element {
	return &Element{
		layout: l,
		Left:   l.NewVar(),
		Right:  l.NewVar(),
		Top:    l.NewVar(),
		Bottom: l.NewVar(),
	}
}

// Inner returns the underlying casso.Solver for advanced use
func (l *Layout) Inner() *casso.Solver {
	return l.inner
}
