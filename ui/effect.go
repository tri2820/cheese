package ui

import (
	"github.com/tri2820/cheese/signals"
)

// Effect watches Exprs and runs fn when they change
// Works with both variable and computed expressions
// The effect runs immediately and then on every dependency change
func Effect(fn func(), deps ...Expr) {
	signalDeps := make([]signals.Dep, len(deps))
	for i, dep := range deps {
		signalDeps[i] = dep // Expr implements Dep via OnChange()
	}
	signals.Effect(fn, signalDeps...)
}
