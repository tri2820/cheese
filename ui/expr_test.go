package ui

import (
	"testing"

	"github.com/tri2820/cheese/signals"
)

func TestExprEffectCanBeCanceled(t *testing.T) {
	layout := NewLayout()
	value := layout.NewVar()

	calls := 0
	cancel := signals.Effect(func() {
		calls++
	}, value)

	value.Set(10)
	cancel()
	value.Set(20)

	if calls != 2 {
		t.Fatalf("Effect() calls for Expr = %d, want 2", calls)
	}
}

func TestExprEffectDeduplicatesSharedDependencies(t *testing.T) {
	layout := NewLayout()
	value := layout.NewVar()

	calls := 0
	cancel := signals.Effect(func() {
		calls++
	}, value.Add(value))
	defer cancel()

	value.Set(10)

	if calls != 2 {
		t.Fatalf("Effect() calls for repeated Expr dependency = %d, want 2", calls)
	}
}
