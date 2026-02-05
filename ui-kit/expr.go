package uikit

import (
	"fmt"

	"github.com/lithdew/casso"
)

// Expr represents a constraint expression
type Expr struct {
	kind exprKind
	// For symbol: the casso Symbol
	symbol casso.Symbol
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
	exprSymbol
	exprOp
)

type operator int

const (
	opAdd operator = iota
	opSub
	opMul
	opDiv
)

// Symbol creates a new expression from a casso Symbol
func Symbol(sym casso.Symbol) Expr {
	return Expr{kind: exprSymbol, symbol: sym}
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
	case exprSymbol:
		return "sym"
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

// Symbol returns the underlying casso.Symbol (only valid for symbol expressions)
func (e Expr) Symbol() casso.Symbol {
	if e.kind != exprSymbol {
		panic("Symbol() called on non-symbol expression")
	}
	return e.symbol
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
	case exprSymbol:
		return 0, []term{{sym: e.symbol, coeff: 1.0}}
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
//   Eq(a, b)           → a == b
//   Eq(a, b, c)        → a == b, b == c, a == c
//   Eq(p1, p2)         → p1.X == p2.X, p1.Y == p2.Y (Point)
//   Eq(p1, p2, p3)     → all X equal, all Y equal
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
				Constraint{relation: relEq, left: points[i].X, right: points[i+1].X},
				Constraint{relation: relEq, left: points[i].Y, right: points[i+1].Y},
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
	for i := 0; i < len(exprs)-1; i++ {
		result = append(result,
			Constraint{relation: relEq, left: exprs[i], right: exprs[i+1]},
		)
	}
	return result
}

// Gte creates: a >= b
func Gte(a, b any) Constraints {
	left := toExpr(a)
	right := toExpr(b)
	return Constraints{{relation: relGte, left: left, right: right}}
}

// Lte creates: a <= b
func Lte(a, b any) Constraints {
	left := toExpr(a)
	right := toExpr(b)
	return Constraints{{relation: relLte, left: left, right: right}}
}

// Gt creates: a > b
func Gt(a, b any) Constraints {
	left := toExpr(a)
	right := toExpr(b)
	return Constraints{{relation: relGt, left: left, right: right}}
}

// Lt creates: a < b
func Lt(a, b any) Constraints {
	left := toExpr(a)
	right := toExpr(b)
	return Constraints{{relation: relLt, left: left, right: right}}
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
	return append(Gte(e, min), Lte(e, max)...)
}

// AspectRatio constrains an element to have the given aspect ratio
// AspectRatio(child, 16, 9) means width:height = 16:9
// Returns: child.Width() * 9 == child.Height() * 16
func AspectRatio(element *Element, widthRatio, heightRatio float64) Constraints {
	return Eq(element.Width().Mul(widthRatio), element.Height().Mul(heightRatio))
}

// Near constrains a to be within maxDistance of b (in either direction)
// Near(a, b, 100) means |a - b| <= 100, which is: a <= b + 100 AND a >= b - 100
func Near(a, b Expr, maxDistance float64) Constraints {
	result := Lte(a, b.Add(maxDistance))
	result = append(result, Gte(a, b.Sub(maxDistance))...)
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

// ToCasso converts the constraint to a casso constraint
func (c Constraint) ToCasso() casso.Constraint {
	leftConst, leftTerms := c.left.flatten()
	rightConst, rightTerms := c.right.flatten()

	constant := leftConst - rightConst
	terms := append(leftTerms, rightTerms...)
	for i := range rightTerms {
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
