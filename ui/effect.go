package ui

// Effect runs a side effect function when dependencies change.
// Runs immediately and then on every dependency change (both regular and quiet).
func Effect(fn func(), deps ...Expr) {
	// Run immediately
	fn()

	// Register to run on all changes (regular and quiet)
	for _, dep := range deps {
		dep.traverseVars(func(v *Expr) {
			if v.kind == exprVar && v.state != nil {
				v.state.onChange = append(v.state.onChange, fn)
				v.state.onChangeQuiet = append(v.state.onChangeQuiet, fn)
			}
		})
	}
}
