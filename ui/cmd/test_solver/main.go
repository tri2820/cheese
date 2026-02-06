package main

import (
	"fmt"

	"github.com/tri2820/cheese/signals"
	"github.com/tri2820/cheese/ui"
)

func main() {
	fmt.Println("=== Comprehensive Solver Correctness Test ===")
	fmt.Println()

	testBasicWidthHeight()
	fmt.Println()
	testDerivedSignalArea()
	fmt.Println()
	testMixSignalAndExpr()
	fmt.Println()
	testSolverCorrectnessWithConflicts()
	fmt.Println()
	testComputedExpressionReactivity()
	fmt.Println()
	testAspectRatio()
	fmt.Println()

	fmt.Println("=== All solver tests passed! ===")
}

func testBasicWidthHeight() {
	fmt.Println("Test 1: Basic Width/Height Correctness")
	layout := ui.NewLayout()
	box := layout.NewView()

	// Direct set of bounds
	box.Left.Set(0)
	box.Right.Set(200)
	box.Top.Set(0)
	box.Bottom.Set(100)

	width := box.Width().Get()
	height := box.Height().Get()

	fmt.Printf("  Box: Left=%.0f Right=%.0f Top=%.0f Bottom=%.0f\n",
		box.Left.Get(), box.Right.Get(), box.Top.Get(), box.Bottom.Get())
	fmt.Printf("  Width = Right - Left = %.0f - %.0f = %.0f\n",
		box.Right.Get(), box.Left.Get(), width)
	fmt.Printf("  Height = Bottom - Top = %.0f - %.0f = %.0f\n",
		box.Bottom.Get(), box.Top.Get(), height)

	// Verify
	assertEqual("Width", width, 200)
	assertEqual("Height", height, 100)
	fmt.Println("  ✓ Width/Height computed correctly")
}

func testDerivedSignalArea() {
	fmt.Println("Test 2: Derived Signal from Expr (Area = Width * Height)")
	layout := ui.NewLayout()
	box := layout.NewView()

	box.Left.Set(0)
	box.Right.Set(150)
	box.Top.Set(0)
	box.Bottom.Set(80)

	// Create a derived signal for area
	var area *signals.Signal[float64]
	effectRuns := 0

	// Effect that computes and prints area
	signals.Effect(func() {
		effectRuns++
		w := box.Width().Get()
		h := box.Height().Get()
		areaVal := w * h
		fmt.Printf("  [Effect #%d] Area = %.0f * %.0f = %.0f\n", effectRuns, w, h, areaVal)

		// Update area signal (using a signal to track value)
		if area == nil {
			sig := signals.New(areaVal)
			area = &sig
		} else {
			(*area).Set(areaVal)
		}
	}, box.Width(), box.Height())

	fmt.Printf("  Initial area: %.0f\n", (*area).Get())
	assertEqual("Area", (*area).Get(), 150*80)

	// Change bounds - effect should re-run twice (once for Right, once for Bottom)
	fmt.Println("  Changing box bounds to 300x200...")
	box.Right.Set(300)
	box.Bottom.Set(200)

	fmt.Printf("  New area: %.0f\n", (*area).Get())
	assertEqual("Area after resize", (*area).Get(), 300*200)
	assertEqual("Effect runs", effectRuns, 3) // Initial + Right change + Bottom change
	fmt.Println("  ✓ Derived signal updates correctly")
}

func testMixSignalAndExpr() {
	fmt.Println("Test 3: Mix Signal[T] and Expr in Effect")
	layout := ui.NewLayout()
	box := layout.NewView()

	// Create a standalone signal
	scaleFactor := signals.New(2.0)

	// Effect that mixes signal and expr
	effectRuns := 0
	signals.Effect(func() {
		effectRuns++
		scale := scaleFactor.Get()
		width := box.Width().Get()
		height := box.Height().Get()
		fmt.Printf("  [Effect #%d] Scale=%.1f, Size=%.0fx%.0f, Scaled=%.0fx%.0f\n",
			effectRuns, scale, width, height, width*scale, height*scale)
	}, scaleFactor, box.Width(), box.Height())

	// Initial setup - each Set() triggers effect
	box.Left.Set(0)
	box.Right.Set(100)
	box.Top.Set(0)
	box.Bottom.Set(50)

	initialRuns := effectRuns
	fmt.Printf("  Initial setup: Scale=%.1f, Size=100x50, effect ran %d times (4 Set() calls)\n",
		scaleFactor.Get(), initialRuns)

	// Change scale - should trigger once
	fmt.Println("  Changing scale to 3.0...")
	scaleFactor.Set(3.0)
	scaleRuns := effectRuns - initialRuns
	fmt.Printf("  Scale change: effect ran %d time(s)\n", scaleRuns)

	// Resize - each Set triggers effect
	fmt.Println("  Resizing box to 200x100...")
	box.Right.Set(200)
	box.Bottom.Set(100)
	finalRuns := effectRuns
	fmt.Printf("  Final effect runs: %d\n", effectRuns)

	// Just verify the effect runs at all
	assertEqual("Effect ran during scale change", scaleRuns >= 1, true)
	assertEqual("Effect ran during resize", finalRuns > initialRuns, true)

	fmt.Println("  ✓ Mixed Signal and Expr dependencies work")
}

func testSolverCorrectnessWithConflicts() {
	fmt.Println("Test 4: Solver Correctness with Conflicting Constraints")
	layout := ui.NewLayout()
	box := layout.NewView()
	container := layout.NewView()

	// Set container bounds
	container.Left.Set(0)
	container.Right.Set(500)
	container.Top.Set(0)
	container.Bottom.Set(400)

	// Add conflicting constraints
	// Strong: box width should be 200
	ui.Eq(box.Width(), 200).Add()

	// Weak: box width should be between 50-100 (should lose to Strong)
	ui.Between(box.Width(), 50, 100).IsWeak().Add()

	// Required: box must be inside container
	box.Inside(container).IsRequired().Add()

	// Set box position
	box.Left.Set(50)
	box.Top.Set(50)

	// Check what width we got
	width := box.Width().Get()
	fmt.Printf("  Container: 500x400\n")
	fmt.Printf("  Box: width=%.0f (Strong=200, Weak=50-100)\n", width)

	// Strong should win
	assertEqual("Box width (Strong wins)", width, 200)

	// Verify box is inside container
	assertEqual("Box Left >= Container Left", box.Left.Get() >= container.Left.Get(), true)
	assertEqual("Box Right <= Container Right", box.Right.Get() <= container.Right.Get(), true)
	assertEqual("Box Top >= Container Top", box.Top.Get() >= container.Top.Get(), true)
	assertEqual("Box Bottom <= Container Bottom", box.Bottom.Get() <= container.Bottom.Get(), true)

	fmt.Println("  ✓ Solver correctly prioritizes Strong over Weak")
}

func testComputedExpressionReactivity() {
	fmt.Println("Test 5: Computed Expression Correctness")
	layout := ui.NewLayout()
	box := layout.NewView()

	centerUpdates := 0
	sizeUpdates := 0

	// Effect tracking center
	signals.Effect(func() {
		centerUpdates++
	}, box.CenterX(), box.CenterY())

	// Effect tracking size
	signals.Effect(func() {
		sizeUpdates++
	}, box.Width(), box.Height())

	// Initial position
	box.Left.Set(100)
	box.Right.Set(300)
	box.Top.Set(100)
	box.Bottom.Set(250)

	// Check computed values
	width := box.Width().Get()
	height := box.Height().Get()
	centerX := box.CenterX().Get()
	centerY := box.CenterY().Get()

	fmt.Printf("  Box: (%.0f,%.0f) to (%.0f,%.0f)\n",
		box.Left.Get(), box.Top.Get(), box.Right.Get(), box.Bottom.Get())
	fmt.Printf("  Size: %.0f x %.0f\n", width, height)
	fmt.Printf("  Center: (%.0f, %.0f)\n", centerX, centerY)

	// Verify correctness
	assertEqual("Width", width, 200.0)
	assertEqual("Height", height, 150.0)
	assertEqual("CenterX", centerX, 200.0)
	assertEqual("CenterY", centerY, 175.0)

	// Move box - changing Left/Top changes both position AND size
	// because Right/Bottom stay constant (no implicit size constraint)
	fmt.Println("  Moving box by +50,+50 (changes position, shrinks size)...")
	box.Left.Set(150)
	box.Top.Set(150)

	newCenterX := box.CenterX().Get()
	newCenterY := box.CenterY().Get()
	movedWidth := box.Width().Get()
	movedHeight := box.Height().Get()

	fmt.Printf("  After move: Size %.0f x %.0f, Center (%.0f, %.0f)\n",
		movedWidth, movedHeight, newCenterX, newCenterY)

	// Right=300, Left=150 → Width=150
	// Bottom=250, Top=150 → Height=100
	assertEqual("Width after move (300-150)", movedWidth, 150.0)
	assertEqual("Height after move (250-150)", movedHeight, 100.0)
	assertEqual("CenterX after move (150+150/2)", newCenterX, 225.0)
	assertEqual("CenterY after move (150+100/2)", newCenterY, 200.0)

	// Resize box - changing Right/Bottom
	fmt.Println("  Resizing box (Right=400, Bottom=350)...")
	box.Right.Set(400)
	box.Bottom.Set(350)

	resizedWidth := box.Width().Get()
	resizedHeight := box.Height().Get()
	resizedCenterX := box.CenterX().Get()
	resizedCenterY := box.CenterY().Get()

	fmt.Printf("  After resize: Size %.0f x %.0f, Center (%.0f, %.0f)\n",
		resizedWidth, resizedHeight, resizedCenterX, resizedCenterY)

	// Left=150, Right=400 → Width=250
	// Top=150, Bottom=350 → Height=200
	assertEqual("Width after resize (400-150)", resizedWidth, 250.0)
	assertEqual("Height after resize (350-150)", resizedHeight, 200.0)
	assertEqual("CenterX after resize (150+250/2)", resizedCenterX, 275.0)
	assertEqual("CenterY after resize (150+200/2)", resizedCenterY, 250.0)

	// Verify effects ran
	assertEqual("Center effects ran", centerUpdates > 0, true)
	assertEqual("Size effects ran", sizeUpdates > 0, true)

	fmt.Println("  ✓ Computed expressions are correct")
}

func testAspectRatio() {
	fmt.Println("Test 6: Aspect Ratio Constraint")
	layout := ui.NewLayout()
	box := layout.NewView()

	// Note: Edits with Weak priority can be overridden by Strong constraints
	// When setting bounds individually, the solver may adjust some values to satisfy the aspect ratio

	// Set aspect ratio 16:9 (Strong constraint)
	ui.AspectRatio(box, 16, 9).Add()

	// Set bounds - solver will adjust to satisfy aspect ratio
	// We set width=160, and the solver will find the corresponding height
	box.Left.Set(0)
	box.Top.Set(0)
	box.Right.Set(160)
	box.Bottom.Set(100) // Start with a value, solver will adjust

	width := box.Width().Get()
	height := box.Height().Get()

	fmt.Printf("  Box: %.0f x %.0f\n", width, height)
	fmt.Printf("  Aspect ratio: %.2f (expected 16/9 ≈ 1.78)\n", width/height)

	// Verify the constraint is satisfied (width * 9 should equal height * 16)
	lhs := width * 9
	rhs := height * 16
	assertApprox("16:9 aspect ratio (width*9 == height*16)", lhs, rhs, 1.0)

	fmt.Println("  ✓ Aspect ratio constraint is satisfied")
	fmt.Println("  Note: Individual bounds may be adjusted to satisfy the Strong aspect ratio constraint")
}

func assertEqual(name string, got, want any) {
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		panic(fmt.Sprintf("%s failed: got %v, want %v", name, got, want))
	}
}

func assertApprox(name string, got, want, tolerance float64) {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		panic(fmt.Sprintf("%s failed: got %v, want %v (diff %.2f > %.2f)", name, got, want, diff, tolerance))
	}
}
