package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// ScrollViewport manages an offscreen scrollable viewport with an interactive,
// draggable scrollbar that widens and highlights on hover/drag.
type ScrollViewport struct {
	ScrollOffset      float64
	MaxScroll         float64
	ContentHeight     int
	ViewportTop       int
	ViewportBottom    int
	ViewportHeight    int
	DraggingScrollbar bool
	DragStartMouseY   int
	DragStartScroll   float64
	ScrollHitRect     image.Rectangle
	ScrollThumbRect   image.Rectangle
	ScrollTrackAvail  int
}

// Setup initializes the viewport boundaries and computes scrollbar geometry.
func (v *ScrollViewport) Setup(top, bottom, contentH, width int) {
	v.ViewportTop = top
	v.ViewportBottom = bottom
	if v.ViewportBottom <= v.ViewportTop {
		v.ViewportBottom = v.ViewportTop + 1
	}
	v.ViewportHeight = v.ViewportBottom - v.ViewportTop
	v.ContentHeight = contentH

	maxScroll := float64(contentH - v.ViewportHeight)
	if maxScroll < 0 {
		maxScroll = 0
	}
	v.MaxScroll = maxScroll

	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	if v.ScrollOffset > v.MaxScroll {
		v.ScrollOffset = v.MaxScroll
	}

	if v.MaxScroll > 0 && v.ContentHeight > 0 {
		thumbH := int(math.Round(float64(v.ViewportHeight) * float64(v.ViewportHeight) / float64(v.ContentHeight)))
		if thumbH < 24 {
			thumbH = 24
		}
		if thumbH > v.ViewportHeight {
			thumbH = v.ViewportHeight
		}
		trackAvail := v.ViewportHeight - thumbH
		thumbOffset := 0
		if trackAvail > 0 && v.MaxScroll > 0 {
			thumbOffset = int(math.Round((v.ScrollOffset / v.MaxScroll) * float64(trackAvail)))
		}
		v.ScrollTrackAvail = trackAvail
		v.ScrollHitRect = image.Rect(width-12, v.ViewportTop, width, v.ViewportBottom)
		v.ScrollThumbRect = image.Rect(width-12, v.ViewportTop+thumbOffset, width, v.ViewportTop+thumbOffset+thumbH)
	} else {
		v.ScrollTrackAvail = 0
		v.ScrollHitRect = image.Rectangle{}
		v.ScrollThumbRect = image.Rectangle{}
	}
}

// ScrollBy scrolls by delta pixels and returns true if the offset changed.
func (v *ScrollViewport) ScrollBy(delta float64) bool {
	if v.MaxScroll <= 0 {
		return false
	}
	next := v.ScrollOffset + delta
	if next < 0 {
		next = 0
	}
	if next > v.MaxScroll {
		next = v.MaxScroll
	}
	if math.Abs(next-v.ScrollOffset) < 0.1 {
		return false
	}
	v.ScrollOffset = next
	return true
}

// HandleMotion updates dragging if active and returns true if scroll offset changed.
func (v *ScrollViewport) HandleMotion(mouseY int) bool {
	if !v.DraggingScrollbar || v.ScrollTrackAvail <= 0 || v.MaxScroll <= 0 {
		return false
	}
	dy := mouseY - v.DragStartMouseY
	next := v.DragStartScroll + (float64(dy)/float64(v.ScrollTrackAvail))*v.MaxScroll
	if next < 0 {
		next = 0
	}
	if next > v.MaxScroll {
		next = v.MaxScroll
	}
	if math.Abs(next-v.ScrollOffset) < 0.1 {
		return false
	}
	v.ScrollOffset = next
	return true
}

// HandleButtonPress handles clicking on the scrollbar track or thumb.
func (v *ScrollViewport) HandleButtonPress(pt image.Point) bool {
	if v.MaxScroll <= 0 || v.ScrollHitRect.Empty() || !pt.In(v.ScrollHitRect) {
		return false
	}
	if !pt.In(v.ScrollThumbRect) && v.ScrollTrackAvail > 0 {
		thumbH := v.ScrollThumbRect.Dy()
		relY := float64(pt.Y - v.ScrollHitRect.Min.Y - thumbH/2)
		next := (relY / float64(v.ScrollTrackAvail)) * v.MaxScroll
		if next < 0 {
			next = 0
		}
		if next > v.MaxScroll {
			next = v.MaxScroll
		}
		v.ScrollOffset = next
	}
	v.DraggingScrollbar = true
	v.DragStartMouseY = pt.Y
	v.DragStartScroll = v.ScrollOffset
	return true
}

// HandleButtonRelease ends dragging.
func (v *ScrollViewport) HandleButtonRelease() bool {
	if v.DraggingScrollbar {
		v.DraggingScrollbar = false
		return true
	}
	return false
}

// IsScrollbarHovered checks if the mouse pointer is over the scrollbar hit area.
func (v *ScrollViewport) IsScrollbarHovered(hoverInside bool, hoverPt image.Point) bool {
	return v.DraggingScrollbar || (hoverInside && !v.ScrollHitRect.Empty() && hoverPt.In(v.ScrollHitRect))
}

// ClipHit translates a viewport-local hit rectangle to surface coordinates
// and intersects it with the visible viewport bounds.
func (v *ScrollViewport) ClipHit(rectInViewport image.Rectangle) (image.Rectangle, bool) {
	vpRect := image.Rect(0, v.ViewportTop, rectInViewport.Max.X, v.ViewportBottom)
	surfRect := rectInViewport.Add(image.Pt(0, v.ViewportTop)).Intersect(vpRect)
	return surfRect, !surfRect.Empty()
}

// Render executes drawContent on an offscreen viewport canvas, blits it to canvas,
// and draws the interactive scrollbar on the right.
func (v *ScrollViewport) Render(canvas *image.RGBA, width int, hoverInside bool, hoverPt image.Point, drawContent func(vpCanvas *image.RGBA, scrollY int)) {
	viewportRect := image.Rect(0, v.ViewportTop, width, v.ViewportBottom)
	vpCanvas := image.NewRGBA(image.Rect(0, 0, width, v.ViewportHeight))

	scrollY := int(math.Round(v.ScrollOffset))
	drawContent(vpCanvas, scrollY)

	draw.Draw(canvas, viewportRect, vpCanvas, image.Point{}, draw.Over)

	if v.MaxScroll > 0 && v.ContentHeight > 0 {
		thumbH := v.ScrollThumbRect.Dy()
		thumbOffset := v.ScrollThumbRect.Min.Y - v.ViewportTop
		hovered := v.IsScrollbarHovered(hoverInside, hoverPt)

		if hovered {
			trackDrawRect := image.Rect(width-8, v.ViewportTop, width-2, v.ViewportBottom)
			drawSmoothRoundedRect(canvas, trackDrawRect, 3.0, color.RGBA{R: 0x26, G: 0x26, B: 0x26, A: 0xff}, draw.Over)
			thumbCol := color.RGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff}
			if v.DraggingScrollbar {
				thumbCol = color.RGBA{R: 0xd0, G: 0xd0, B: 0xd0, A: 0xff}
			}
			barRect := image.Rect(width-8, v.ViewportTop+thumbOffset, width-2, v.ViewportTop+thumbOffset+thumbH)
			drawSmoothRoundedRect(canvas, barRect, 3.0, thumbCol, draw.Over)
		} else {
			barRect := image.Rect(width-5, v.ViewportTop+thumbOffset, width-2, v.ViewportTop+thumbOffset+thumbH)
			drawSmoothRoundedRect(canvas, barRect, 1.5, color.RGBA{R: 0x66, G: 0x66, B: 0x66, A: 0xff}, draw.Over)
		}
	}
}
