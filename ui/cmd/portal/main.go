package main

import (
	"log"
	"sync"

	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui"
)

func main() {
	// Connect to Wayland display
	disp, err := ui.Connect(display.Config{
		Required: display.RequiredGlobals{
			Compositor: true,
			Shm:        true,
			LayerShell: true,
		},
	})
	if err != nil {
		log.Fatalf("Failed to connect to display: %v", err)
	}
	defer disp.Close()

	// Create layout
	layout := ui.NewLayout()
	go layout.RenderLoop()

	// Track masks by output for dynamic hotplug handling
	type maskState struct {
		mask *ui.Mask
		left bool // true = left output (shows right 70%), false = right output (shows left 30%)
	}
	masks := make(map[*client.WlOutput]*maskState)
	var masksMu sync.Mutex

	// Note: We now handle dynamic output changes, so we don't fail fast
	// The portal will adapt as outputs are added/removed

	// Create ONE widget with contents (coordinate-free, shared across outputs)
	widget := ui.NewWidget(layout)

	// Set content size at 96 DPI baseline
	widget.Width = 600
	widget.Height = 600

	// Create 9 colorful rectangles arranged in 3x3 grid
	colors := []string{
		"#FF0000", "#FF7F00", "#FFFF00", // Red, Orange, Yellow
		"#00FF00", "#0000FF", "#4B0082", // Green, Blue, Indigo
		"#9400D3", "#FF1493", "#00CED1", // Violet, Pink, Cyan
	}

	// Each cell is 1/3 of content size in pixels
	cellPixelW := widget.Width / 3.0
	cellPixelH := widget.Height / 3.0

	for i, color := range colors {
		row := i / 3
		col := i % 3

		// Create rectangle with constraint-based positioning
		rect := widget.NewRectangle()
		rect.Color.Set(color)

		// Position using constraints (in content coordinate space 0-600 x 0-200)
		ui.Eq(rect.Left, float64(col)*cellPixelW).Add()
		ui.Eq(rect.Top, float64(row)*cellPixelH).Add()
		ui.Eq(rect.Right, float64(col+1)*cellPixelW).Add()
		ui.Eq(rect.Bottom, float64(row+1)*cellPixelH).Add()
	}

	// Add a label for testing text rendering
	label := widget.NewLabel("Portal Demo Testing Clipping Masks")
	label.Color.Set("#FFFFFF")
	label.FontSize.Set(24.0)
	label.Justify.Set(ui.JustifyCenter)

	// Center label vertically with equal spacing
	ui.Eq(label.Left, 0).Add()
	ui.Eq(label.Right, widget.Width).Add()
	ui.Eq(label.Top, 0).Add()
	ui.Eq(label.Bottom, widget.Height).Add()

	log.Printf("Created widget with 9 colorful rectangles in 3x3 grid and label")

	// Helper function to create a mask for an output
	// Anchor is always AnchorTop|AnchorLeft - position controlled by margin via reactive Effect
	setupMask := func(output *display.Output, isLeft bool) *ui.Mask {
		if isLeft {
			// Left side: shows right 70% of content (180 to 600)
			clipLeft := 0.3 * widget.Width
			clipTop := 0.0
			surfaceW := int(0.7 * float64(output.ScaleFrom96DPI(widget.Width)))
			surfaceH := int(1.0 * float64(output.ScaleFrom96DPI(widget.Height)))

			log.Printf("Creating LEFT mask for %s: clip=(%.1f, %.1f) size=%dx%d",
				output.Name, clipLeft, clipTop, surfaceW, surfaceH)

			mask := widget.NewMask(disp.Display(), output.WlOutput(), ui.LayerConfig{
				Layer: shell.LayerPositionTop,
				Name:  "portal-left-" + output.Name,
			})
			mask.ClipX = clipLeft
			mask.ClipY = clipTop

			// Position at left edge, vertically centered
			ui.Eq(mask.Left, 0).Add()
			ui.Eq(mask.Width(), float64(surfaceW)).Add()
			ui.Eq(mask.CenterY(), float64(output.ModeHeight)/2).Add()
			ui.Eq(mask.Height(), float64(surfaceH)).Add()

			return mask
		} else {
			// Right side: shows left 30% of content (0 to 180)
			clipLeft := 0.031
			clipTop := 0.0
			surfaceW := int(0.3 * float64(output.ScaleFrom96DPI(widget.Width)))
			surfaceH := int(1.0 * float64(output.ScaleFrom96DPI(widget.Height)))

			log.Printf("Creating RIGHT mask for %s: clip=(%.1f, %.1f) size=%dx%d",
				output.Name, clipLeft, clipTop, surfaceW, surfaceH)

			mask := widget.NewMask(disp.Display(), output.WlOutput(), ui.LayerConfig{
				Layer: shell.LayerPositionTop,
				Name:  "portal-right-" + output.Name,
			})
			mask.ClipX = clipLeft
			mask.ClipY = clipTop

			// Position at right edge, vertically centered
			ui.Eq(mask.Right, float64(output.ModeWidth)).Add()
			ui.Eq(mask.Width(), float64(surfaceW)).Add()
			ui.Eq(mask.CenterY(), float64(output.ModeHeight)/2).Add()
			ui.Eq(mask.Height(), float64(surfaceH)).Add()

			return mask
		}
	}

	// Track previous outputs for diffing
	var prevOutputs []*display.Output

	// Set up reactive output handling using Effect
	signals.Effect(func() {
		current := disp.Outputs().Get()

		// Build sets for O(1) lookup
		prevSet := make(map[*display.Output]bool)
		for _, p := range prevOutputs {
			prevSet[p] = true
		}
		currentSet := make(map[*display.Output]bool)
		for _, c := range current {
			currentSet[c] = true
		}

		// Find added outputs
		for _, output := range current {
			if !prevSet[output] {
				// New output added
				masksMu.Lock()
				isLeft := len(masks) == 1
				mask := setupMask(output, isLeft)
				masks[output.WlOutput()] = &maskState{mask: mask, left: isLeft}
				masksMu.Unlock()
				log.Printf("Output added: %s (assigned to %s side)", output.Name, map[bool]string{true: "LEFT", false: "RIGHT"}[isLeft])
			}
		}

		// Find removed outputs
		for _, p := range prevOutputs {
			if !currentSet[p] {
				// Output removed
				masksMu.Lock()
				if state, ok := masks[p.WlOutput()]; ok {
					// Close the frame to release resources
					if frame := state.mask.Frame(); frame != nil {
						frame.Close()
						// Remove from layout tracking to prevent rendering to closed frame
						layout.RemoveFrame(frame)
					}
					delete(masks, p.WlOutput())
					log.Printf("Output removed: %s", p.Name)
				}
				masksMu.Unlock()
			}
		}

		prevOutputs = current
	}, disp.Outputs())

	log.Println()
	log.Println("Portal running... Press Ctrl+C to exit")

	// Run event loopz
	if err := disp.Run(); err != nil {
		log.Fatalf("Dispatch error: %v", err)
	}
}
