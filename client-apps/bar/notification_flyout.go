package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"sync"
	"time"

	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/shm"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	flyoutWidth        = 340
	flyoutMaxHeight    = 420
	flyoutHeaderHeight = 36
	flyoutCardPad      = 10
	flyoutCardGap      = 8
	flyoutCornerRadius = 8
)

type flyoutPointerState struct {
	inside bool
	x      int
	y      int
}

type NotificationFlyout struct {
	disp        *display.Display
	output      *display.Output
	service     *NotificationService
	icons       *IconResolver
	face        font.Face
	mu          sync.Mutex
	isOpen      bool
	surface     *surface.Surface
	layer       *shell.LayerSurface
	frame       *shm.Frame
	closeSub    func()
	hits        []flyoutHit
	hover       flyoutPointerState
	hoverIndex  int
	startTime   int64
}

type flyoutHit struct {
	rect    image.Rectangle
	action  string
	notifID uint32
}

func NewNotificationFlyout(disp *display.Display, output *display.Output, service *NotificationService, icons *IconResolver) *NotificationFlyout {
	f := &NotificationFlyout{
		disp:       disp,
		output:     output,
		service:    service,
		icons:      icons,
		face:       basicfont.Face7x13,
		hoverIndex: -1,
		startTime:  time.Now().UnixMilli(),
	}

	return f
}

func (f *NotificationFlyout) IsOpen() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.isOpen
}

func (f *NotificationFlyout) Toggle(anchorX int32) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.isOpen {
		f.closeLocked()
	} else {
		f.openLocked(anchorX)
	}
}

func (f *NotificationFlyout) openLocked(anchorX int32) {
	if f.isOpen {
		return
	}

	surf, err := surface.New(f.disp.Compositor())
	if err != nil {
		log.Printf("NotificationFlyout: Failed to create surface: %v", err)
		return
	}

	// Position right below the bar, aligned near the module
	leftMargin := anchorX - 20
	if leftMargin < barSidePad {
		leftMargin = barSidePad
	}

	layer, err := shell.NewLayer(surf, f.disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          "cheese-notification-flyout",
		Anchor:        shell.AnchorTop | shell.AnchorLeft,
		Width:         flyoutWidth,
		Height:        flyoutMaxHeight,
		ExclusiveZone: 0, // Float over windows
		Output:        f.output.WlOutput(),
	})
	if err != nil {
		_ = surf.Close()
		log.Printf("NotificationFlyout: Failed to create layer surface: %v", err)
		return
	}

	topMargin := int32(barHeight + barTopMargin + 6)
	if err := layer.SetMargin(topMargin, 0, 0, leftMargin); err != nil {
		_ = layer.Close()
		_ = surf.Close()
		return
	}

	frame, err := shm.NewFrame(f.disp.Shm(), layer, f.disp, shm.FrameConfig{
		Format:  client.WlShmFormatArgb8888,
		Buffers: 2,
	})
	if err != nil {
		_ = layer.Close()
		_ = surf.Close()
		return
	}

	f.surface = surf
	f.layer = layer
	f.frame = frame
	f.isOpen = true

	frame.SetRender(func(width, height int, frameTime uint32, pixels []byte) {
		f.draw(width, height, pixels)
	})
	frame.SetManualMode(true)

	layer.OnClose(func() {
		f.mu.Lock()
		f.closeLocked()
		f.mu.Unlock()
	})

	f.closeSub = f.service.Subscribe(func() {
		f.render()
	})

	f.service.MarkAllRead()
	f.render()
}

func (f *NotificationFlyout) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeLocked()
}

func (f *NotificationFlyout) closeLocked() {
	if !f.isOpen {
		return
	}
	f.isOpen = false
	if f.closeSub != nil {
		f.closeSub()
		f.closeSub = nil
	}
	if f.frame != nil {
		_ = f.frame.Close()
		f.frame = nil
	}
	if f.layer != nil {
		_ = f.layer.Close()
		f.layer = nil
	}
	if f.surface != nil {
		_ = f.surface.Close()
		f.surface = nil
	}
}

func (f *NotificationFlyout) render() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.frame != nil && f.frame.Ready() {
		f.frame.ManualRender(uint32(time.Now().UnixMilli() - f.startTime))
	}
}

func (f *NotificationFlyout) draw(width, height int, pixels []byte) {
	img := &image.RGBA{
		Pix:    pixels,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}

	// Transparent base
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)

	// Container background
	containerRect := image.Rect(0, 0, width, height)
	drawRoundedRect(img, containerRect, flyoutCornerRadius, color.RGBA{R: 0x16, G: 0x16, B: 0x16, A: 0xf6}, draw.Over)
	// Outer border
	drawOutline(img, containerRect, flyoutCornerRadius, color.RGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xff})

	f.hits = f.hits[:0]

	// Header
	headerRect := image.Rect(0, 0, width, flyoutHeaderHeight)
	drawRoundedRect(img, headerRect, flyoutCornerRadius, color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff}, draw.Over)

	// Header Title
	items := f.service.List()
	title := fmt.Sprintf("Notifications (%d)", len(items))
	f.drawText(img, 12, 22, title, color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff})

	// Clear All Button (Header Right)
	if len(items) > 0 {
		clearText := "Clear All"
		clearX := width - 85
		clearBtnRect := image.Rect(clearX, 6, width-10, 30)
		drawRoundedRect(img, clearBtnRect, 4, color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff}, draw.Over)
		f.drawText(img, clearX+10, 22, clearText, color.RGBA{R: 0xcc, G: 0xcc, B: 0xcc, A: 0xff})
		f.hits = append(f.hits, flyoutHit{rect: clearBtnRect, action: "clear_all"})
	}

	// Content
	y := flyoutHeaderHeight + 10
	if len(items) == 0 {
		f.drawText(img, width/2-60, height/2, "No notifications", color.RGBA{R: 0x77, G: 0x77, B: 0x77, A: 0xff})
		return
	}

	for _, n := range items {
		cardHeight := 58
		if y+cardHeight > height-10 {
			break
		}

		cardRect := image.Rect(10, y, width-10, y+cardHeight)
		drawRoundedRect(img, cardRect, 6, color.RGBA{R: 0x24, G: 0x24, B: 0x24, A: 0xff}, draw.Over)

		// Header row: AppName + time
		appName := n.AppName
		if appName == "" {
			appName = "System"
		}
		timeStr := n.CreatedAt.Format("15:04")
		f.drawText(img, 18, y+16, fmt.Sprintf("%s • %s", appName, timeStr), color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff})

		// Dismiss [x] button on card
		dismissRect := image.Rect(width-32, y+4, width-12, y+24)
		f.drawText(img, width-26, y+18, "x", color.RGBA{R: 0x99, G: 0x99, B: 0x99, A: 0xff})
		f.hits = append(f.hits, flyoutHit{rect: dismissRect, action: "dismiss", notifID: n.ID})

		// Summary / Title
		summary := truncateString(n.Summary, 34)
		f.drawText(img, 18, y+32, summary, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})

		// Body
		body := truncateString(n.Body, 42)
		if body != "" {
			f.drawText(img, 18, y+46, body, color.RGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff})
		}

		// Clicking the card itself also dismisses
		f.hits = append(f.hits, flyoutHit{rect: cardRect, action: "dismiss", notifID: n.ID})

		y += cardHeight + flyoutCardGap
	}
}

func (f *NotificationFlyout) drawText(dst draw.Image, x, y int, text string, col color.Color) {
	drawer := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(col),
		Face: f.face,
		Dot:  fixed.P(x, y),
	}
	drawer.DrawString(text)
}

func (f *NotificationFlyout) HandlePointerEnter(ev client.WlPointerEnterEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.isOpen || f.surface == nil || ev.Surface != f.surface.WlSurface() {
		return
	}
	f.hover.inside = true
	f.hover.x = int(ev.SurfaceX)
	f.hover.y = int(ev.SurfaceY)
}

func (f *NotificationFlyout) HandlePointerLeave(ev client.WlPointerLeaveEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.isOpen || f.surface == nil || ev.Surface != f.surface.WlSurface() {
		return
	}
	f.hover.inside = false
}

func (f *NotificationFlyout) HandlePointerMotion(ev client.WlPointerMotionEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.isOpen || !f.hover.inside {
		return
	}
	f.hover.x = int(ev.SurfaceX)
	f.hover.y = int(ev.SurfaceY)
}

func (f *NotificationFlyout) HandlePointerButton(ev client.WlPointerButtonEvent) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.isOpen || !f.hover.inside {
		return false
	}

	if ev.Button != 0x110 || ev.State != client.WlPointerButtonStatePressed {
		return true
	}

	pt := image.Pt(f.hover.x, f.hover.y)
	for _, hit := range f.hits {
		if pt.In(hit.rect) {
			if hit.action == "clear_all" {
				f.service.ClearAll()
				return true
			}
			if hit.action == "dismiss" {
				f.service.Dismiss(hit.notifID)
				return true
			}
		}
	}

	return true
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func drawOutline(dst draw.Image, rect image.Rectangle, radius int, col color.Color) {
	// Simple border drawing on inner edges
	top := image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+1)
	bottom := image.Rect(rect.Min.X, rect.Max.Y-1, rect.Max.X, rect.Max.Y)
	left := image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+1, rect.Max.Y)
	right := image.Rect(rect.Max.X-1, rect.Min.Y, rect.Max.X, rect.Max.Y)

	u := image.NewUniform(col)
	draw.Draw(dst, top, u, image.Point{}, draw.Over)
	draw.Draw(dst, bottom, u, image.Point{}, draw.Over)
	draw.Draw(dst, left, u, image.Point{}, draw.Over)
	draw.Draw(dst, right, u, image.Point{}, draw.Over)
}
