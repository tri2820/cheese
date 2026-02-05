package signals

import (
	"sync"
)

// Dep is the dependency interface that all signals implement.
type Dep interface {
	OnChange(fn func())
}

// Signal is a reactive value that can be get, set, and watched for changes.
type Signal[T any] interface {
	Get() T
	Set(v T)
	Dep
}

// signal is the concrete implementation of Signal[T].
type signal[T any] struct {
	value       T
	subscribers []func(T)
	mu          sync.RWMutex
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
	s.mu.Unlock()

	for _, sub := range subscribers {
		sub(v)
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
func Effect(fn func(), deps ...Dep) {
	fn()
	for _, dep := range deps {
		dep.OnChange(fn)
	}
}
