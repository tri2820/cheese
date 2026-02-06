package ui

import (
	"fmt"

	"github.com/lithdew/casso"
)

// exprState holds the mutable state for a variable expression
// This is shared among all Expr references to the same variable.
type exprState struct {
	value         float64
	onChange      []func()
	onChangeQuiet []func()
	symbol        casso.Symbol
}

// Expr represents a constraint expression
type Expr struct {
	kind   exprKind
	state  *exprState // Shared state for exprVar
	layout *Layout    // Layout for exprVar (where it was created)
	// For constant: the float64 value
	constant float64
	// For operations: the operator and operands
	op    operator
	left  *Expr
	right *Expr
}

type exprKind int

const (
	exprConst exprKind = iota
	exprVar
	exprOp
)

type operator int

const (
	opAdd operator = iota
	opSub
	opMul
	opDiv
)

// Get reads the current value from the expression
func (e Expr) Get() float64 {
	switch e.kind {
	case exprVar:
		return e.state.value
	case exprConst:
		return e.constant
	case exprOp:
		return e.eval()
	}
	return 0
}

// Set updates the value and triggers solver resolve
// Priority must be Strong or Weak (defaults to Strong)
func (e *Expr) Set(value float64, priority ...Priority) {
	p := Strong
	if len(priority) > 0 {
		p = priority[0]
		if p != Strong && p != Weak {
			panic("Set(): priority must be Strong or Weak")
		}
	}
	if e.kind != exprVar || e.state == nil {
		panic("Set() only valid for variable expressions")
	}
	e.state.value = value
	// Trigger all observers
	for _, fn := range e.state.onChange {
		fn()
	}
	for _, fn := range e.state.onChangeQuiet {
		fn()
	}
}

// OnChange implements Dep interface for Effect()
// For exprVar: registers callback
// For exprOp: recursively watches variable dependencies
func (e Expr) OnChange(fn func()) {
	e.traverseVars(func(v *Expr) {
		if v.kind == exprVar && v.state != nil {
			v.state.onChange = append(v.state.onChange, fn)
		}
	})
}

// OnChangeQuiet registers a callback that runs even when SetQuiet is used
// For effects that should run during constraint resolution
func (e Expr) OnChangeQuiet(fn func()) {
	e.traverseVars(func(v *Expr) {
		if v.kind == exprVar && v.state != nil {
			v.state.onChangeQuiet = append(v.state.onChangeQuiet, fn)
		}
	})
}

// SetQuiet updates the value without triggering OnChange callbacks.
// Only notifies OnChangeQuiet observers.
// Used internally by the solver to avoid infinite loops.
func (e *Expr) SetQuiet(value float64) {
	if e.kind != exprVar || e.state == nil {
		return
	}
	e.state.value = value
	// Only trigger quiet observers
	for _, fn := range e.state.onChangeQuiet {
		fn()
	}
}

// traverseVars visits all variable expressions that this expr depends on
func (e Expr) traverseVars(fn func(*Expr)) {
	switch e.kind {
	case exprVar:
		fn(&e)
	case exprConst:
		// no dependencies
	case exprOp:
		e.left.traverseVars(fn)
		e.right.traverseVars(fn)
	}
}

// getLayout traverses the expression tree to find the layout from any variable expression
func (e Expr) getLayout() *Layout {
	var layout *Layout
	e.traverseVars(func(v *Expr) {
		if v.kind == exprVar && v.layout != nil && layout == nil {
			layout = v.layout
		}
	})
	return layout
}

// eval computes value for operation expressions
func (e Expr) eval() float64 {
	switch e.op {
	case opAdd:
		return e.left.Get() + e.right.Get()
	case opSub:
		return e.left.Get() - e.right.Get()
	case opMul:
		return e.left.Get() * e.right.Get()
	case opDiv:
		return e.left.Get() / e.right.Get()
	}
	return 0
}

// Const creates a constant expression
func Const(v float64) Expr {
	return Expr{kind: exprConst, constant: v}
}

// Add returns a + b (accepts float64, int, or Expr)
func (e Expr) Add(v any) Expr {
	switch val := v.(type) {
	case float64:
		return Expr{kind: exprOp, op: opAdd, left: &e, right: &Expr{kind: exprConst, constant: val}}
	case int:
		return Expr{kind: exprOp, op: opAdd, left: &e, right: &Expr{kind: exprConst, constant: float64(val)}}
	case Expr:
		return Expr{kind: exprOp, op: opAdd, left: &e, right: &val}
	default:
		panic(fmt.Sprintf("Add: unsupported type %T", v))
	}
}

// Sub returns a - b (accepts float64, int, or Expr)
func (e Expr) Sub(v any) Expr {
	switch val := v.(type) {
	case float64:
		return Expr{kind: exprOp, op: opSub, left: &e, right: &Expr{kind: exprConst, constant: val}}
	case int:
		return Expr{kind: exprOp, op: opSub, left: &e, right: &Expr{kind: exprConst, constant: float64(val)}}
	case Expr:
		return Expr{kind: exprOp, op: opSub, left: &e, right: &val}
	default:
		panic(fmt.Sprintf("Sub: unsupported type %T", v))
	}
}

// Mul returns a * b (b must be constant)
func (e Expr) Mul(v float64) Expr {
	return Expr{kind: exprOp, op: opMul, left: &e, right: &Expr{kind: exprConst, constant: v}}
}

// Div returns a / b (b must be constant)
func (e Expr) Div(v float64) Expr {
	return Expr{kind: exprOp, op: opDiv, left: &e, right: &Expr{kind: exprConst, constant: v}}
}

// String returns a string representation of the expression
func (e Expr) String() string {
	switch e.kind {
	case exprConst:
		return fmt.Sprintf("%v", e.constant)
	case exprVar:
		return "var"
	case exprOp:
		opStr := ""
		switch e.op {
		case opAdd:
			opStr = "+"
		case opSub:
			opStr = "-"
		case opMul:
			opStr = "*"
		case opDiv:
			opStr = "/"
		}
		return fmt.Sprintf("(%s %s %s)", e.left.String(), opStr, e.right.String())
	}
	return "?"
}

// flatten walks the expression tree and returns (constant, []terms)
func (e Expr) flatten() (float64, []term) {
	constant, terms := e.flattenRec()

	// Combine like terms (same symbol)
	combined := make(map[casso.Symbol]float64)
	for _, t := range terms {
		combined[t.sym] += t.coeff
	}

	result := make([]term, 0, len(combined))
	for sym, coeff := range combined {
		if coeff != 0 {
			result = append(result, term{sym: sym, coeff: coeff})
		}
	}
	return constant, result
}

type term struct {
	sym   casso.Symbol
	coeff float64
}

func (e Expr) flattenRec() (float64, []term) {
	switch e.kind {
	case exprConst:
		return e.constant, nil
	case exprVar:
		return 0, []term{{sym: e.state.symbol, coeff: 1.0}}
	case exprOp:
		leftConst, leftTerms := e.left.flattenRec()
		rightConst, rightTerms := e.right.flattenRec()

		switch e.op {
		case opAdd:
			return leftConst + rightConst, append(leftTerms, rightTerms...)
		case opSub:
			for i := range rightTerms {
				rightTerms[i].coeff *= -1
			}
			return leftConst - rightConst, append(leftTerms, rightTerms...)
		case opMul:
			// One side must be constant
			if e.right.kind == exprConst {
				mult := e.right.constant
				constant, terms := leftConst, leftTerms
				constant *= mult
				for i := range terms {
					terms[i].coeff *= mult
				}
				return constant, terms
			}
			if e.left.kind == exprConst {
				mult := e.left.constant
				constant, terms := rightConst, rightTerms
				constant *= mult
				for i := range terms {
					terms[i].coeff *= mult
				}
				return constant, terms
			}
			panic("mul: can only multiply by constant")
		case opDiv:
			if e.right.kind != exprConst {
				panic("div: can only divide by constant")
			}
			div := e.right.constant
			constant, terms := leftConst, leftTerms
			constant /= div
			for i := range terms {
				terms[i].coeff /= div
			}
			return constant, terms
		}
	}
	return 0, nil
}

type relation int

const (
	relEq relation = iota
	relGte
	relLte
	relGt
	relLt
)

func (r relation) String() string {
	switch r {
	case relEq:
		return "=="
	case relGte:
		return ">="
	case relLte:
		return "<="
	case relGt:
		return ">"
	case relLt:
		return "<"
	}
	return "?"
}

// Constraint represents a constraint between expressions
type Constraint struct {
	relation relation
	left     Expr
	right    Expr
	priority Priority // zero value = Strong (default)
	layout   *Layout  // layout to add this constraint to
}

// Constraints is a slice of Constraint with helper methods
type Constraints []Constraint

func (c Constraint) String() string {
	return fmt.Sprintf("%s %s %s", c.left.String(), c.relation.String(), c.right.String())
}

// toExpr converts float64, int, or Expr to Expr
func toExpr(v any) Expr {
	switch val := v.(type) {
	case Expr:
		return val
	case float64:
		return Const(val)
	case int:
		return Const(float64(val))
	default:
		panic(fmt.Sprintf("unsupported type: %T", v))
	}
}

// Eq creates equality constraints: all args are equal
// Returns Constraints (always a slice for consistency)
// Supports Expr, float64, int, or Point (generates 2 constraints for Point)
//
// Examples:
//
//	Eq(a, b)           → a == b
//	Eq(a, b, c)        → a == b, b == c, a == c
//	Eq(p1, p2)         → p1.X == p2.X, p1.Y == p2.Y (Point)
//	Eq(p1, p2, p3)     → all X equal, all Y equal
func Eq(args ...any) Constraints {
	if len(args) < 2 {
		panic("Eq: requires at least 2 arguments")
	}

	// Check if any arg is Point → all must be Point
	hasPoint := false
	for _, arg := range args {
		if _, ok := arg.(Point); ok {
			hasPoint = true
			break
		}
	}

	if hasPoint {
		// All args must be Point
		points := make([]Point, len(args))
		for i, arg := range args {
			p, ok := arg.(Point)
			if !ok {
				panic("Eq: all arguments must be Point when any is Point")
			}
			points[i] = p
		}

		// All X equal, all Y equal
		var result Constraints
		for i := 0; i < len(points)-1; i++ {
			result = append(result,
				Constraint{relation: relEq, left: points[i].X, right: points[i+1].X, layout: points[i].X.getLayout()},
				Constraint{relation: relEq, left: points[i].Y, right: points[i+1].Y, layout: points[i].Y.getLayout()},
			)
		}
		return result
	}

	exprs := make([]Expr, len(args))
	for i, arg := range args {
		exprs[i] = toExpr(arg)
	}

	// Chain equality
	var result Constraints
	layout := exprs[0].getLayout()
	for i := 0; i < len(exprs)-1; i++ {
		result = append(result,
			Constraint{relation: relEq, left: exprs[i], right: exprs[i+1], layout: layout},
		)
	}
	return result
}

// Gte creates: a >= b
func Gte(a, b any) Constraints {
	left := toExpr(a)
	right := toExpr(b)
	return Constraints{{relation: relGte, left: left, right: right, layout: left.getLayout()}}
}

// Lte creates: a <= b
func Lte(a, b any) Constraints {
	left := toExpr(a)
	right := toExpr(b)
	return Constraints{{relation: relLte, left: left, right: right, layout: left.getLayout()}}
}

// Gt creates: a > b
func Gt(a, b any) Constraints {
	left := toExpr(a)
	right := toExpr(b)
	return Constraints{{relation: relGt, left: left, right: right, layout: left.getLayout()}}
}

// Lt creates: a < b
func Lt(a, b any) Constraints {
	left := toExpr(a)
	right := toExpr(b)
	return Constraints{{relation: relLt, left: left, right: right, layout: left.getLayout()}}
}

// Percent returns a percentage of an expression
// Percent(width, 50) == width.Mul(0.5)
func Percent(e Expr, percentage float64) Expr {
	return e.Mul(percentage / 100.0)
}

// Between constrains an expression to be within [min, max]
// Returns: expr >= min, expr <= max
func Between(expr any, min, max float64) Constraints {
	e := toExpr(expr)
	layout := e.getLayout()
	result := Gte(e, min)
	for i := range result {
		result[i].layout = layout
	}
	result = append(result, Lte(e, max)...)
	for i := range result {
		result[i].layout = layout
	}
	return result
}

// AspectRatio constrains an layoutitem to have the given aspect ratio
// AspectRatio(child, 16, 9) means width:height = 16:9
// Returns: child.Width() * 9 == child.Height() * 16
// Formula: width * heightRatio == height * widthRatio
// So: width/height = widthRatio/heightRatio
func AspectRatio(layoutitem *LayoutItem, widthRatio, heightRatio float64) Constraints {
	width := layoutitem.Width()
	height := layoutitem.Height()
	return Constraints{{
		relation: relEq,
		left:     width.Mul(heightRatio),
		right:    height.Mul(widthRatio),
		layout:   width.getLayout(),
	}}
}

// Near constrains a to be within maxDistance of b (in either direction)
// Near(a, b, 100) means |a - b| <= 100, which is: a <= b + 100 AND a >= b - 100
func Near(a, b Expr, maxDistance float64) Constraints {
	layout := a.getLayout()
	result := Lte(a, b.Add(maxDistance))
	for i := range result {
		result[i].layout = layout
	}
	result = append(result, Gte(a, b.Sub(maxDistance))...)
	for i := range result {
		result[i].layout = layout
	}
	return result
}

// IsRequired marks constraints as Required priority
func (c Constraints) IsRequired() Constraints {
	for i := range c {
		c[i].priority = Required
	}
	return c
}

// IsWeak marks constraints as Weak priority
func (c Constraints) IsWeak() Constraints {
	for i := range c {
		c[i].priority = Weak
	}
	return c
}

// Add adds these constraints to the solver
// Returns a handle that can be used to remove the constraints
func (c Constraints) Add() ConstraintHandle {
	if len(c) == 0 || c[0].layout == nil {
		panic("Add(): constraint has no layout - use LayoutItem methods like layoutitem.RightOf(other)")
	}
	return c[0].layout.Add(c)
}

// ToCasso converts the constraint to a casso constraint
func (c Constraint) ToCasso() casso.Constraint {
	leftConst, leftTerms := c.left.flatten()
	rightConst, rightTerms := c.right.flatten()

	constant := leftConst - rightConst
	terms := append(leftTerms, rightTerms...)
	for i := len(leftTerms); i < len(terms); i++ {
		terms[i].coeff *= -1
	}

	var op casso.Op
	switch c.relation {
	case relEq:
		op = casso.EQ
	case relGte:
		op = casso.GTE
	case relLte:
		op = casso.GTE
		for i := range terms {
			terms[i].coeff *= -1
		}
		constant *= -1
	case relGt:
		op = casso.GTE
		constant += 1e-6
	case relLt:
		op = casso.GTE
		for i := range terms {
			terms[i].coeff *= -1
		}
		constant *= -1
		constant += 1e-6
	}

	cassoTerms := make([]casso.Term, len(terms))
	for i, t := range terms {
		cassoTerms[i] = t.sym.T(t.coeff)
	}

	return casso.NewConstraint(op, constant, cassoTerms...)
}
