package signals

import (
	"sync"
)

// Dep is the dependency interface that all signals implement.
type Dep interface {
	OnChange(fn func())
}

// QuietDep is an optional interface for quiet notifications (during batch/constraint updates).
type QuietDep interface {
	Dep
	OnChangeQuiet(fn func())
}

// Signal is a reactive value that can be get, set, and watched for changes.
type Signal[T any] interface {
	Get() T
	Set(v T)
	Dep
	QuietDep
}

// signal is the concrete implementation of Signal[T].
type signal[T any] struct {
	value          T
	subscribers    []func(T)
	quietObservers []func()
	mu             sync.RWMutex
}

// Get returns the current value.
func (s *signal[T]) Get() T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

// Set updates the value and notifies all subscribers.
func (s *signal[T]) Set(v T) {
	s.mu.Lock()
	s.value = v
	subscribers := s.subscribers
	quietObservers := s.quietObservers
	s.mu.Unlock()

	for _, sub := range subscribers {
		sub(v)
	}
	for _, obs := range quietObservers {
		obs()
	}
}

// OnChange registers a callback that runs when the signal changes.
func (s *signal[T]) OnChange(fn func()) {
	s.mu.Lock()
	s.subscribers = append(s.subscribers, func(_ T) {
		fn()
	})
	s.mu.Unlock()
}

// OnChangeQuiet registers a callback that runs even during SetQuiet.
func (s *signal[T]) OnChangeQuiet(fn func()) {
	s.mu.Lock()
	s.quietObservers = append(s.quietObservers, fn)
	s.mu.Unlock()
}

// SetQuiet updates the value without triggering regular OnChange callbacks.
// Still notifies quiet observers.
func (s *signal[T]) SetQuiet(v T) {
	s.mu.Lock()
	s.value = v
	observers := s.quietObservers
	s.mu.Unlock()

	for _, obs := range observers {
		obs()
	}
}

// Subscribe registers a callback that receives the current value immediately
// and future values on every change.
func (s *signal[T]) Subscribe(fn func(T)) {
	s.mu.Lock()
	s.subscribers = append(s.subscribers, fn)
	currentValue := s.value
	s.mu.Unlock()

	fn(currentValue)
}

// New creates a new Signal with the given initial value.
func New[T any](value T) Signal[T] {
	return &signal[T]{value: value}
}

// Derive creates a computed Signal from dependencies.
func Derive[T any](fn func() T, deps ...Dep) Signal[T] {
	sig := &signal[T]{value: fn()}

	for _, dep := range deps {
		dep.OnChange(func() {
			sig.Set(fn())
		})
	}

	return sig
}

// Deps groups multiple dependencies into a single signal.
func Deps(deps ...Dep) Signal[bool] {
	return Derive(func() bool { return false }, deps...)
}

// Effect runs a side effect function when dependencies change.
// Works with Signal[T] and Expr (which implements Signal[float64]).
// Uses OnChangeQuiet if available (for constraint resolution support).
func Effect(fn func(), deps ...Dep) {
	// Run immediately
	fn()

	// Register to run on changes (including quiet changes if supported)
	for _, dep := range deps {
		if qd, ok := dep.(QuietDep); ok {
			qd.OnChangeQuiet(fn)
		} else {
			dep.OnChange(fn)
		}
	}
}
