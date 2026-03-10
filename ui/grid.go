package ui

// Grid is a simple widget-local grid helper built on top of the constraint solver.
// It is intentionally small: it computes cell bounds from the widget's logical size
// and installs owned constraints on the placed layout items.
type Grid struct {
	widget  *Widget
	columns int
	rows    int
	gapX    DesignUnit
	gapY    DesignUnit
}

// Grid creates a widget-local grid helper.
func (w *Widget) Grid(columns, rows int) *Grid {
	if columns <= 0 {
		columns = 1
	}
	if rows <= 0 {
		rows = 1
	}

	return &Grid{
		widget:  w,
		columns: columns,
		rows:    rows,
	}
}

// SetGap configures the horizontal and vertical spacing between cells.
func (g *Grid) SetGap(x, y DesignUnit) *Grid {
	g.gapX = x
	g.gapY = y
	return g
}

// Place places an item into a single cell.
func (g *Grid) Place(item *LayoutItem, column, row int) ConstraintHandle {
	return g.PlaceSpan(item, column, row, 1, 1)
}

// PlaceSpan places an item into a cell range.
func (g *Grid) PlaceSpan(item *LayoutItem, column, row, columnSpan, rowSpan int) ConstraintHandle {
	if item == nil {
		return ConstraintHandle{}
	}
	if column < 0 {
		column = 0
	}
	if row < 0 {
		row = 0
	}
	if columnSpan <= 0 {
		columnSpan = 1
	}
	if rowSpan <= 0 {
		rowSpan = 1
	}
	if column >= g.columns {
		column = g.columns - 1
	}
	if row >= g.rows {
		row = g.rows - 1
	}
	if column+columnSpan > g.columns {
		columnSpan = g.columns - column
	}
	if row+rowSpan > g.rows {
		rowSpan = g.rows - row
	}

	totalGapX := g.gapX * DesignUnit(g.columns-1)
	totalGapY := g.gapY * DesignUnit(g.rows-1)
	cellWidth := g.widget.Width().Sub(totalGapX).Div(float64(g.columns))
	cellHeight := g.widget.Height().Sub(totalGapY).Div(float64(g.rows))

	left := cellWidth.Add(g.gapX).Mul(float64(column))
	top := cellHeight.Add(g.gapY).Mul(float64(row))
	width := cellWidth.Mul(float64(columnSpan)).Add(g.gapX * DesignUnit(columnSpan-1))
	height := cellHeight.Mul(float64(rowSpan)).Add(g.gapY * DesignUnit(rowSpan-1))

	return item.Own(
		Eq(item.Left, left),
		Eq(item.Top, top),
		Eq(item.Width(), width),
		Eq(item.Height(), height),
	)
}

// Fill constrains an item to occupy the widget's full logical bounds.
func (w *Widget) Fill(item *LayoutItem) ConstraintHandle {
	if item == nil {
		return ConstraintHandle{}
	}
	return item.Own(
		Eq(item.Left, 0),
		Eq(item.Top, 0),
		Eq(item.Right, w.Width()),
		Eq(item.Bottom, w.Height()),
	)
}
