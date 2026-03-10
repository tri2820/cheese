package main

import (
	"log"
	"sort"

	"github.com/tri2820/cheese/ui"
)

type portalSide int

const (
	portalSideLeft portalSide = iota
	portalSideRight
)

func main() {
	// Connect to Wayland display
	disp, err := ui.Connect(ui.DisplayConfig{
		Required: ui.RequiredGlobals{
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
	layout.Start()
	defer layout.Close()
	layout.OnError(func(err error) {
		log.Printf("ui error: %v", err)
	})

	// Note: We now handle dynamic output changes, so we don't fail fast
	// The portal will adapt as outputs are added/removed

	// Create ONE widget with contents (coordinate-free, shared across outputs)
	widget := ui.NewWidget(layout)

	// Set content size at 96 DPI baseline
	widget.SetSize(600, 600)

	// Create 9 colorful rectangles arranged in 3x3 grid
	colors := []string{
		"#FF0000", "#FF7F00", "#FFFF00", // Red, Orange, Yellow
		"#00FF00", "#0000FF", "#4B0082", // Green, Blue, Indigo
		"#9400D3", "#FF1493", "#00CED1", // Violet, Pink, Cyan
	}

	// Each cell is 1/3 of content size in pixels
	grid := widget.Grid(3, 3)

	for i, color := range colors {
		row := i / 3
		col := i % 3

		// Create rectangle with constraint-based positioning
		rect := widget.NewRectangle()
		rect.Color.Set(color)
		grid.Place(rect.LayoutItem, col, row)
	}

	// Add a label for testing text rendering
	label := widget.NewLabel("Portal Demo Testing Clipping Masks")
	label.Color.Set("#FFFFFF")
	label.FontSize.Set(24.0)
	label.Justify.Set(ui.JustifyCenter)

	// Center label vertically with equal spacing
	widget.Fill(label.LayoutItem)

	log.Printf("Created widget with 9 colorful rectangles in 3x3 grid and label")

	// Helper function to create a mask for an output
	// Anchor is always AnchorTop|AnchorLeft - position controlled by margin via reactive Effect
	setupMask := func(output *ui.Output, side portalSide) (*ui.Mask, error) {
		if side == portalSideLeft {
			// Left side: shows right 70% of content (180 to 600)
			clipLeft := 0.3 * widget.ContentWidth()
			clipTop := ui.DesignUnit(0)
			surfaceW := output.ToPixels(0.7 * widget.ContentWidth())
			surfaceH := output.ToPixels(widget.ContentHeight())

			log.Printf("Creating LEFT mask for %s: clip=(%.1f, %.1f) size=%dx%d",
				output.Name(), clipLeft, clipTop, surfaceW.Int(), surfaceH.Int())

			mask, err := widget.NewMask(output, ui.LayerConfig{
				Layer: ui.LayerTop,
				Name:  "portal-left-" + output.Name(),
			})
			if err != nil {
				return nil, err
			}
			mask.SetClip(clipLeft, clipTop)

			// Position at left edge, vertically centered
			mask.Own(
				ui.Eq(mask.Left, 0),
				ui.Eq(mask.Width(), surfaceW),
				ui.Eq(mask.CenterY(), float64(output.Height())/2),
				ui.Eq(mask.Height(), surfaceH),
			)

			return mask, nil
		} else {
			// Right side: shows left 30% of content (0 to 180)
			clipLeft := ui.DesignUnit(0)
			clipTop := ui.DesignUnit(0)
			surfaceW := output.ToPixels(0.3 * widget.ContentWidth())
			surfaceH := output.ToPixels(widget.ContentHeight())

			log.Printf("Creating RIGHT mask for %s: clip=(%.1f, %.1f) size=%dx%d",
				output.Name(), clipLeft, clipTop, surfaceW.Int(), surfaceH.Int())

			mask, err := widget.NewMask(output, ui.LayerConfig{
				Layer: ui.LayerTop,
				Name:  "portal-right-" + output.Name(),
			})
			if err != nil {
				return nil, err
			}
			mask.SetClip(clipLeft, clipTop)

			// Position at right edge, vertically centered
			mask.Own(
				ui.Eq(mask.Right, float64(output.Width())),
				ui.Eq(mask.Width(), surfaceW),
				ui.Eq(mask.CenterY(), float64(output.Height())/2),
				ui.Eq(mask.Height(), surfaceH),
			)

			return mask, nil
		}
	}

	cancelOutputs := disp.WatchOutputs(func(outputs []*ui.Output) func() {
		if len(outputs) == 0 {
			log.Printf("No ready outputs; portal masks are not active")
			return nil
		}

		sorted := append([]*ui.Output(nil), outputs...)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].X() != sorted[j].X() {
				return sorted[i].X() < sorted[j].X()
			}
			if sorted[i].Y() != sorted[j].Y() {
				return sorted[i].Y() < sorted[j].Y()
			}
			return sorted[i].Name() < sorted[j].Name()
		})

		if len(sorted) > 2 {
			log.Printf("Portal uses the first 2 outputs by position; ignoring %d extra output(s)", len(sorted)-2)
			sorted = sorted[:2]
		}

		log.Printf("Rebuilding portal masks for %d output(s)", len(sorted))

		cleanups := make([]func(), 0, len(sorted))
		for i, output := range sorted {
			side := portalSideLeft
			sideName := "LEFT"
			if i == 1 {
				side = portalSideRight
				sideName = "RIGHT"
			}

			mask, err := setupMask(output, side)
			if err != nil {
				log.Printf("Failed to create %s portal mask for output %s: %v", sideName, output.Name(), err)
				continue
			}

			log.Printf("Activated %s portal mask on %s", sideName, output.Name())

			maskToClose := mask
			outputName := output.Name()
			cleanups = append(cleanups, func() {
				if err := maskToClose.Remove(); err != nil {
					log.Printf("Failed to close portal mask for output %s: %v", outputName, err)
				}
			})
		}

		return func() {
			for _, cleanup := range cleanups {
				if cleanup != nil {
					cleanup()
				}
			}
		}
	})
	defer cancelOutputs()

	log.Println()
	log.Println("Portal running... Press Ctrl+C to exit")

	// Run event loopz
	if err := disp.Run(); err != nil {
		log.Fatalf("Dispatch error: %v", err)
	}
}
