package main

import (
	"fmt"
	"log"
	"os"

	"github.com/tri2820/cheese/protocols/client"
)

func main() {
	// Connect to Wayland compositor
	display, err := client.Connect("")
	if err != nil {
		log.Fatalf("Failed to connect to Wayland: %v", err)
	}
	defer display.Context().Close()

	fmt.Println("Connected to Wayland compositor!")

	// Set up error handler
	display.SetErrorHandler(func(ev client.WlDisplayErrorEvent) {
		fmt.Printf("Display error: object_id=%v code=%d message=%s\n",
			ev.ObjectId, ev.Code, ev.Message)
		os.Exit(1)
	})

	// Get the registry to discover global interfaces
	registry, err := display.GetRegistry()
	if err != nil {
		log.Fatalf("Failed to get registry: %v", err)
	}

	// Set up global handler to list available interfaces
	registry.SetGlobalHandler(func(ev client.WlRegistryGlobalEvent) {
		fmt.Printf("Global interface: %s (name=%d, version=%d)\n",
			ev.Interface, ev.Name, ev.Version)
	})

	// Set up global_remove handler
	registry.SetGlobalRemoveHandler(func(ev client.WlRegistryGlobalRemoveEvent) {
		fmt.Printf("Global removed: name=%d\n", ev.Name)
	})

	// Dispatch events to receive the global list
	fmt.Println("\nAvailable Wayland interfaces:")
	for i := 0; i < 10; i++ {
		if err := display.Context().Dispatch(); err != nil {
			log.Fatalf("Dispatch error: %v", err)
		}
	}

	fmt.Println("\nSuccessfully listed Wayland interfaces!")
}
