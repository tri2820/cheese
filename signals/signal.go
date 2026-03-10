package signals

import (
	"sync"
)

// CancelFunc unregisters a reactive subscription or effect.
type CancelFunc func()

// Join combines multiple cancel functions into one.
func Join(cancelFns ...CancelFunc) CancelFunc {
	return func() {
		for _, cancel := range cancelFns {
			if cancel != nil {
				cancel()
			}
		}
	}
}

// Dep is the dependency interface that all signals implement.
type Dep interface {
	OnChange(fn func()) CancelFunc
}

// QuietDep is an optional interface for quiet notifications (during batch/constraint updates).
type QuietDep interface {
	Dep
	OnChangeQuiet(fn func()) CancelFunc
}

// Signal is a reactive value that can be get, set, and watched for changes.
type Signal[T any] interface {
	Get() T
	Set(v T)
	SetQuiet(v T)
	Subscribe(fn func(T)) CancelFunc
	Dispose()
	Dep
	QuietDep
}

type subscriber[T any] struct {
	id int
	fn func(T)
}

type observer struct {
	id int
	fn func()
}

// signal is the concrete implementation of Signal[T].
type signal[T any] struct {
	value          T
	nextID         int
	disposed       bool
	subscribers    []subscriber[T]
	quietObservers []observer
	disposeFns     []CancelFunc
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
	subscribers := append([]subscriber[T](nil), s.subscribers...)
	quietObservers := append([]observer(nil), s.quietObservers...)
	s.mu.Unlock()

	for _, sub := range subscribers {
		if sub.fn != nil {
			sub.fn(v)
		}
	}
	for _, obs := range quietObservers {
		if obs.fn != nil {
			obs.fn()
		}
	}
}

// OnChange registers a callback that runs when the signal changes.
func (s *signal[T]) OnChange(fn func()) CancelFunc {
	if fn == nil {
		return nil
	}

	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return nil
	}
	s.nextID++
	id := s.nextID
	s.subscribers = append(s.subscribers, subscriber[T]{id: id, fn: func(_ T) {
		fn()
	}})
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, sub := range s.subscribers {
			if sub.id == id {
				s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
				return
			}
		}
	}
}

// OnChangeQuiet registers a callback that runs even during SetQuiet.
func (s *signal[T]) OnChangeQuiet(fn func()) CancelFunc {
	if fn == nil {
		return nil
	}

	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return nil
	}
	s.nextID++
	id := s.nextID
	s.quietObservers = append(s.quietObservers, observer{id: id, fn: fn})
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, obs := range s.quietObservers {
			if obs.id == id {
				s.quietObservers = append(s.quietObservers[:i], s.quietObservers[i+1:]...)
				return
			}
		}
	}
}

// SetQuiet updates the value without triggering regular OnChange callbacks.
// Still notifies quiet observers.
func (s *signal[T]) SetQuiet(v T) {
	s.mu.Lock()
	s.value = v
	observers := append([]observer(nil), s.quietObservers...)
	s.mu.Unlock()

	for _, obs := range observers {
		if obs.fn != nil {
			obs.fn()
		}
	}
}

// Subscribe registers a callback that receives the current value immediately
// and future values on every change.
func (s *signal[T]) Subscribe(fn func(T)) CancelFunc {
	if fn == nil {
		return nil
	}

	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return nil
	}
	s.nextID++
	id := s.nextID
	s.subscribers = append(s.subscribers, subscriber[T]{id: id, fn: fn})
	currentValue := s.value
	s.mu.Unlock()

	fn(currentValue)

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, sub := range s.subscribers {
			if sub.id == id {
				s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
				return
			}
		}
	}
}

// Dispose removes all subscribers and detaches any dependency subscriptions.
func (s *signal[T]) Dispose() {
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return
	}
	s.disposed = true
	disposeFns := append([]CancelFunc(nil), s.disposeFns...)
	s.disposeFns = nil
	s.subscribers = nil
	s.quietObservers = nil
	s.mu.Unlock()

	for _, dispose := range disposeFns {
		if dispose != nil {
			dispose()
		}
	}
}

// New creates a new Signal with the given initial value.
func New[T any](value T) Signal[T] {
	return &signal[T]{value: value}
}

// Derive creates a computed Signal from dependencies.
// Uses OnChangeQuiet if available (for constraint resolution support).
func Derive[T any](fn func() T, deps ...Dep) Signal[T] {
	sig := &signal[T]{value: fn()}
	cancelFns := make([]CancelFunc, 0, len(deps))

	for _, dep := range deps {
		if qd, ok := dep.(QuietDep); ok {
			cancelFns = append(cancelFns, qd.OnChangeQuiet(func() {
				sig.Set(fn())
			}))
		} else {
			cancelFns = append(cancelFns, dep.OnChange(func() {
				sig.Set(fn())
			}))
		}
	}
	sig.disposeFns = cancelFns

	return sig
}

// Deps groups multiple dependencies into a single signal.
func Deps(deps ...Dep) Signal[bool] {
	return Derive(func() bool { return false }, deps...)
}

// Effect runs a side effect function when dependencies change.
// Works with Signal[T] and Expr (which implements Signal[float64]).
// Uses OnChangeQuiet if available (for constraint resolution support).
func Effect(fn func(), deps ...Dep) CancelFunc {
	if fn == nil {
		return nil
	}

	// Run immediately
	fn()

	// Register to run on changes (including quiet changes if supported)
	cancelFns := make([]CancelFunc, 0, len(deps))
	for _, dep := range deps {
		if qd, ok := dep.(QuietDep); ok {
			cancelFns = append(cancelFns, qd.OnChangeQuiet(fn))
		} else {
			cancelFns = append(cancelFns, dep.OnChange(fn))
		}
	}

	return Join(cancelFns...)
}

// Scope runs a cleanup-aware reactive scope.
// It runs fn immediately, runs the previous cleanup before re-running on dependency changes,
// and runs the final cleanup when the scope is canceled.
func Scope(fn func() CancelFunc, deps ...Dep) CancelFunc {
	if fn == nil {
		return nil
	}

	var mu sync.Mutex
	cleanup := fn()

	rerun := func() {
		mu.Lock()
		prevCleanup := cleanup
		cleanup = nil
		mu.Unlock()

		if prevCleanup != nil {
			prevCleanup()
		}

		nextCleanup := fn()

		mu.Lock()
		cleanup = nextCleanup
		mu.Unlock()
	}

	cancelFns := make([]CancelFunc, 0, len(deps))
	for _, dep := range deps {
		if qd, ok := dep.(QuietDep); ok {
			cancelFns = append(cancelFns, qd.OnChangeQuiet(rerun))
		} else {
			cancelFns = append(cancelFns, dep.OnChange(rerun))
		}
	}

	return func() {
		Join(cancelFns...)()

		mu.Lock()
		prevCleanup := cleanup
		cleanup = nil
		mu.Unlock()

		if prevCleanup != nil {
			prevCleanup()
		}
	}
}
