package signals

import (
	"sync"
)

// Dep is the dependency interface that all signals implement.
type Dep interface {
	OnChange(fn func())
}

// Signal is a thread-safe reactive signal holding a value of type T.
type Signal[T any] struct {
	value       T
	subscribers []func(T)
	mu          sync.RWMutex
}

// Get returns the current value of the signal.
func (s *Signal[T]) Get() T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

// Set updates the signal's value and notifies all subscribers.
func (s *Signal[T]) Set(v T) {
	s.mu.Lock()
	s.value = v
	subscribers := s.subscribers
	s.mu.Unlock()

	for _, sub := range subscribers {
		sub(v)
	}
}

// SetQuiet updates the signal's value without notifying subscribers.
// Used internally to avoid circular updates.
func (s *Signal[T]) SetQuiet(v T) {
	s.mu.Lock()
	s.value = v
	s.mu.Unlock()
}

// Subscribe registers a callback that receives the current value immediately
// and future values on every change.
func (s *Signal[T]) Subscribe(fn func(T)) {
	s.mu.Lock()
	s.subscribers = append(s.subscribers, fn)
	currentValue := s.value
	s.mu.Unlock()

	fn(currentValue)
}

// OnChange registers a callback that runs when the signal changes,
// but doesn't receive the value. Implements Dep.
func (s *Signal[T]) OnChange(fn func()) {
	s.mu.Lock()
	s.subscribers = append(s.subscribers, func(_ T) {
		fn()
	})
	s.mu.Unlock()
}

// New creates a source signal with an initial value.
func New[T any](value T) *Signal[T] {
	return &Signal[T]{value: value}
}

// Derive creates a computed signal that derives its value from dependencies.
// The computation function runs immediately and whenever any dependency changes.
func Derive[T any](fn func() T, deps ...Dep) *Signal[T] {
	sig := &Signal[T]{value: fn()}

	for _, dep := range deps {
		dep.OnChange(func() {
			sig.Set(fn())
		})
	}

	return sig
}

// Deps groups multiple dependencies into a single signal.
// Useful for batching or tracking multiple dependencies together.
func Deps(deps ...Dep) *Signal[bool] {
	return Derive(func() bool { return false }, deps...)
}

// Effect runs a side effect function when dependencies change.
// The effect runs immediately and then on every dependency change.
func Effect(fn func(), deps ...Dep) {
	Derive(func() bool {
		fn()
		return false // Dummy return value
	}, deps...)
}
