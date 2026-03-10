package main

import (
	"fmt"
	"log"

	"github.com/tri2820/cheese/client-toolkit/display"
)

func main() {
	log.Println("Connecting to Wayland display...")
	log.Println("Monitoring for output hotplug events...")

	disp := display.MustConnect(display.Config{})

	disp.OnOutput(func(output *display.Output, added bool) {
		if added {
			log.Printf(">>> OUTPUT ADDED: %s\n", output.Name)
			printOutput(output)
		} else {
			log.Printf("<<< OUTPUT REMOVED: %s\n", output.Name)
		}
		fmt.Println()
	})

	// Do a roundtrip to receive all initial output information
	if err := disp.Roundtrip(); err != nil {
		log.Fatal("Roundtrip failed:", err)
	}

	// Print initial state
	outputs := disp.ReadyOutputs()
	if len(outputs) == 0 {
		log.Println("No outputs detected.")
	} else {
		log.Printf("Initially detected %d output(s):\n", len(outputs))
		for _, out := range outputs {
			printOutput(out)
			fmt.Println()
		}
	}

	log.Println("Press Ctrl+C to exit (monitoring for hotplug events)...")

	// Run event loop - OnOutput will be called on hotplug
	if err := disp.Run(); err != nil {
		log.Printf("Dispatch error: %v", err)
	}
}

func printOutput(out *display.Output) {
	fmt.Printf("  Name:         %s\n", out.Name)
	fmt.Printf("  Description:  %s\n", out.Description)
	fmt.Printf("  Make/Model:   %s / %s\n", out.Make, out.Model)
	fmt.Printf("  Resolution:   %dx%d\n", out.ModeWidth, out.ModeHeight)
	if out.Refresh > 0 {
		fmt.Printf("  Refresh Rate: %.2f Hz\n", float64(out.Refresh)/1000)
	}
	fmt.Printf("  Position:     %d, %d\n", out.X, out.Y)
	if out.PhysicalWidth > 0 && out.PhysicalHeight > 0 {
		fmt.Printf("  Physical Size: %dmm x %dmm\n", out.PhysicalWidth, out.PhysicalHeight)
		fmt.Printf("  Calculated DPI: %.1f\n", out.DPI())
	} else {
		fmt.Printf("  Physical Size: unknown\n")
	}
	fmt.Printf("  Scale Factor: %d\n", out.Scale)
	fmt.Printf("  Transform:    %v\n", out.Transform)
	fmt.Printf("  Subpixel:     %v\n", out.Subpixel)
}
