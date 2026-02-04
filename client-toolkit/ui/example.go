package main

import (
	"fmt"

	"github.com/tri2820/cheese/signals"
)

func main() {
	// Source signals - type inferred!
	count := signals.New(42)
	text := signals.New("Count: ")

	// Computed signal - type inferred from return type!
	computed := signals.Compute(func() string {
		return text.Get() + fmt.Sprint(count.Get())
	}, count, text)

	signals.Effect(func() {
		fmt.Println("Derived:", computed.Get())
	}, computed)

	signals.Effect(func() {
		fmt.Printf("Count: %d\n", count.Get())
	}, count)

	count.Set(100)
	text.Set("New count: ")

	computed.Subscribe(func(v string) {
		fmt.Println("Subscriber got:", v)
	})

	count.Set(999)

	fmt.Println("\n--- Using Deps ---")

	username := signals.New("Alice")
	email := signals.New("alice@example.com")

	userDeps := signals.Deps(username, email)

	signals.Effect(func() {
		fmt.Println("User changed")
	}, userDeps)

	username.Set("Bob")

	fmt.Println("\n--- Nested Deps ---")

	settingsDeps := signals.Deps(count, text)
	allDeps := signals.Deps(userDeps, settingsDeps)

	signals.Effect(func() {
		fmt.Println("Any input changed")
	}, allDeps)

	count.Set(1000)
	email.Set("bob@example.com")
}
