package signals

import (
	"reflect"
	"testing"
)

func TestEffectRunsImmediatelyAndCanBeCanceled(t *testing.T) {
	sig := New(1)

	var got []int
	cancel := Effect(func() {
		got = append(got, sig.Get())
	}, sig)

	sig.Set(2)
	cancel()
	sig.Set(3)

	want := []int{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Effect() calls = %v, want %v", got, want)
	}
}

func TestSubscribeReplaysCurrentValueAndCanBeCanceled(t *testing.T) {
	sig := New("a")

	var got []string
	cancel := sig.Subscribe(func(v string) {
		got = append(got, v)
	})

	sig.Set("b")
	cancel()
	sig.Set("c")

	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Subscribe() calls = %v, want %v", got, want)
	}
}

func TestSetQuietOnlyTriggersQuietObservers(t *testing.T) {
	sig := New(1)

	changeCalls := 0
	quietCalls := 0

	cancelChange := sig.OnChange(func() {
		changeCalls++
	})
	cancelQuiet := sig.OnChangeQuiet(func() {
		quietCalls++
	})
	defer cancelChange()
	defer cancelQuiet()

	sig.SetQuiet(2)
	if changeCalls != 0 {
		t.Fatalf("OnChange fired during SetQuiet: got %d, want 0", changeCalls)
	}
	if quietCalls != 1 {
		t.Fatalf("OnChangeQuiet calls after SetQuiet = %d, want 1", quietCalls)
	}

	sig.Set(3)
	if changeCalls != 1 {
		t.Fatalf("OnChange calls after Set = %d, want 1", changeCalls)
	}
	if quietCalls != 2 {
		t.Fatalf("OnChangeQuiet calls after Set = %d, want 2", quietCalls)
	}
}

func TestDeriveDisposeDetachesDependencies(t *testing.T) {
	src := New(2)
	derived := Derive(func() int {
		return src.Get() * 2
	}, src)

	var got []int
	cancel := derived.Subscribe(func(v int) {
		got = append(got, v)
	})

	src.Set(3)
	cancel()
	derived.Dispose()
	src.Set(4)

	want := []int{4, 6}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("derived values = %v, want %v", got, want)
	}
	if derived.Get() != 6 {
		t.Fatalf("derived value after Dispose() = %d, want 6", derived.Get())
	}
	if cancel := derived.Subscribe(func(int) {}); cancel != nil {
		t.Fatalf("Subscribe() on disposed signal returned non-nil cancel func")
	}
}

func TestSetSnapshotsObservers(t *testing.T) {
	sig := New(1)

	var got []string
	var lateCancel CancelFunc

	cancelPrimary := sig.OnChange(func() {
		got = append(got, "primary")
		if lateCancel == nil {
			lateCancel = sig.OnChange(func() {
				got = append(got, "late")
			})
		}
	})
	defer cancelPrimary()

	sig.Set(2)
	sig.Set(3)

	if lateCancel != nil {
		defer lateCancel()
	}

	want := []string{"primary", "primary", "late"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OnChange snapshot behavior = %v, want %v", got, want)
	}
}
