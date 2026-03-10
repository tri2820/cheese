package ui

import "testing"

func TestGridPlace(t *testing.T) {
	layout := NewLayout()
	widget := NewWidget(layout)
	widget.SetSize(300, 300)

	rect := widget.NewRectangle()
	widget.Grid(3, 3).Place(rect.LayoutItem, 1, 2)

	if got := rect.Left.Get(); got != 100 {
		t.Fatalf("rect.Left = %v, want 100", got)
	}
	if got := rect.Top.Get(); got != 200 {
		t.Fatalf("rect.Top = %v, want 200", got)
	}
	if got := rect.Right.Get(); got != 200 {
		t.Fatalf("rect.Right = %v, want 200", got)
	}
	if got := rect.Bottom.Get(); got != 300 {
		t.Fatalf("rect.Bottom = %v, want 300", got)
	}
}

func TestWidgetFill(t *testing.T) {
	layout := NewLayout()
	widget := NewWidget(layout)
	widget.SetSize(640, 360)

	label := widget.NewLabel("fill")
	widget.Fill(label.LayoutItem)

	if got := label.Left.Get(); got != 0 {
		t.Fatalf("label.Left = %v, want 0", got)
	}
	if got := label.Top.Get(); got != 0 {
		t.Fatalf("label.Top = %v, want 0", got)
	}
	if got := label.Right.Get(); got != 640 {
		t.Fatalf("label.Right = %v, want 640", got)
	}
	if got := label.Bottom.Get(); got != 360 {
		t.Fatalf("label.Bottom = %v, want 360", got)
	}
}

func TestGridReactsToWidgetSizeChanges(t *testing.T) {
	layout := NewLayout()
	widget := NewWidget(layout)
	widget.SetSize(300, 300)

	rect := widget.NewRectangle()
	widget.Grid(3, 3).Place(rect.LayoutItem, 1, 0)

	if got := rect.Left.Get(); got != 100 {
		t.Fatalf("initial rect.Left = %v, want 100", got)
	}
	if got := rect.Right.Get(); got != 200 {
		t.Fatalf("initial rect.Right = %v, want 200", got)
	}

	widget.SetSize(600, 300)

	if got := rect.Left.Get(); got != 200 {
		t.Fatalf("updated rect.Left = %v, want 200", got)
	}
	if got := rect.Right.Get(); got != 400 {
		t.Fatalf("updated rect.Right = %v, want 400", got)
	}
}
