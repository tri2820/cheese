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
	"golang.org/x/image/math/fixed"
)

const (
	flyoutWidth        = 320
	flyoutHeaderHeight = 36
	flyoutCardHeight   = 62
	flyoutCardGap      = 6
	flyoutCornerRadius = 8
)

type flyoutPointerState struct {
	inside bool
	x      int
	y      int
}

type flyoutHit struct {
	rect    image.Rectangle
	action  string
	notifID uint32
}

type NotificationFlyout struct {
	disp      *display.Display
	output    *display.Output
	service   *NotificationService
	icons     *IconResolver
	face      font.Face
	mu        sync.Mutex
	isOpen    bool
	surface   *surface.Surface
	layer     *shell.LayerSurface
	frame     *shm.Frame
	closeSub  func()
	hits      []flyoutHit
	hover     flyoutPointerState
	startTime int64
	dirtyCh   chan struct{}
	stop      chan struct{}
}

func NewNotificationFlyout(disp *display.Display, output *display.Output, service *NotificationService, icons *IconResolver) *NotificationFlyout {
	return &NotificationFlyout{
		disp:      disp,
		output:    output,
		service:   service,
		icons:     icons,
		face:      loadBarFont(output),
		startTime: time.Now().UnixMilli(),
	}
}

func (f *NotificationFlyout) IsOpen() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.isOpen
}

func (f *NotificationFlyout) Toggle(anchorX int32) {
	f.mu.Lock()
	open := f.isOpen
	f.mu.Unlock()

	if open {
		f.Close()
	} else {
		f.Open(anchorX)
	}
}

func (f *NotificationFlyout) Open(anchorX int32) {
	f.mu.Lock()
	if f.isOpen {
		f.mu.Unlock()
		return
	}

	surf, err := surface.New(f.disp.Compositor())
	if err != nil {
		f.mu.Unlock()
		log.Printf("NotificationFlyout: Failed to create surface: %v", err)
		return
	}

	leftMargin := anchorX
	if leftMargin < barSidePad {
		leftMargin = barSidePad
	}

	items := f.service.List()
	calcHeight := f.computeHeight(len(items))

	layer, err := shell.NewLayer(surf, f.disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          "cheese-notification-flyout",
		Anchor:        shell.AnchorTop | shell.AnchorLeft,
		Width:         flyoutWidth,
		Height:        uint32(calcHeight),
		ExclusiveZone: 0,
		Output:        f.output.WlOutput(),
	})
	if err != nil {
		f.mu.Unlock()
		_ = surf.Close()
		log.Printf("NotificationFlyout: Failed to create layer surface: %v", err)
		return
	}

	topMargin := int32(barHeight + barTopMargin + 2)
	if err := layer.SetMargin(topMargin, 0, 0, leftMargin); err != nil {
		f.mu.Unlock()
		_ = layer.Close()
		_ = surf.Close()
		return
	}

	frame, err := shm.NewFrame(f.disp.Shm(), layer, f.disp, shm.FrameConfig{
		Format:  client.WlShmFormatArgb8888,
		Buffers: 2,
	})
	if err != nil {
		f.mu.Unlock()
		_ = layer.Close()
		_ = surf.Close()
		return
	}

	f.surface = surf
	f.layer = layer
	f.frame = frame
	f.isOpen = true
	f.dirtyCh = make(chan struct{}, 1)
	f.stop = make(chan struct{})

	frame.SetRender(func(width, height int, frameTime uint32, pixels []byte) {
		f.draw(width, height, pixels)
	})
	frame.SetManualMode(true)

	layer.OnClose(func() {
		f.Close()
	})

	stopCh := f.stop
	dirtyCh := f.dirtyCh
	go func() {
		ticker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		pending := false
		for {
			select {
			case <-dirtyCh:
				pending = true
			case <-ticker.C:
			case <-stopCh:
				return
			}
			if !pending {
				continue
			}
			f.mu.Lock()
			fr := f.frame
			f.mu.Unlock()
			if fr != nil && fr.Ready() {
				fr.ManualRender(uint32(time.Now().UnixMilli() - f.startTime))
				pending = false
			}
		}
	}()

	f.closeSub = f.service.Subscribe(func() {
		f.markDirty()
	})

	f.mu.Unlock()

	f.markDirty()
	f.service.MarkAllRead()
}

func (f *NotificationFlyout) computeHeight(count int) int {
	if count == 0 {
		return flyoutHeaderHeight + 48
	}
	h := flyoutHeaderHeight + 12 + count*(flyoutCardHeight+flyoutCardGap) + 8
	if h > 380 {
		return 380
	}
	return h
}

func (f *NotificationFlyout) markDirty() {
	f.mu.Lock()
	ch := f.dirtyCh
	f.mu.Unlock()
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (f *NotificationFlyout) Close() {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return
	}
	f.isOpen = false
	closeSub := f.closeSub
	f.closeSub = nil
	stopCh := f.stop
	f.stop = nil
	f.dirtyCh = nil
	frame := f.frame
	f.frame = nil
	layer := f.layer
	f.layer = nil
	surf := f.surface
	f.surface = nil
	f.hover = flyoutPointerState{}
	f.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}
	if closeSub != nil {
		closeSub()
	}
	if frame != nil {
		_ = frame.Close()
	}
	if layer != nil {
		_ = layer.Close()
	}
	if surf != nil {
		_ = surf.Close()
	}
}

func (f *NotificationFlyout) draw(width, height int, pixels []byte) {
	img := &image.RGBA{
		Pix:    pixels,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}

	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)

	containerRect := image.Rect(0, 0, width, height)
	drawRoundedRect(img, containerRect, flyoutCornerRadius, color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xfa}, draw.Over)

	hits := make([]flyoutHit, 0, 10)

	// Header
	headerRect := image.Rect(0, 0, width, flyoutHeaderHeight)
	drawRoundedRect(img, headerRect, flyoutCornerRadius, color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff}, draw.Over)

	items := f.service.List()
	title := fmt.Sprintf("Notifications (%d)", len(items))
	f.drawText(img, 12, 22, title, color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff})

	if len(items) > 0 {
		clearText := "Clear All"
		clearX := width - 82
		clearBtnRect := image.Rect(clearX, 6, width-8, 30)
		drawRoundedRect(img, clearBtnRect, 4, color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff}, draw.Over)
		f.drawText(img, clearX+8, 21, clearText, color.RGBA{R: 0xcc, G: 0xcc, B: 0xcc, A: 0xff})
		hits = append(hits, flyoutHit{rect: clearBtnRect, action: "clear_all"})
	}

	y := flyoutHeaderHeight + 8
	if len(items) == 0 {
		f.drawText(img, width/2-55, flyoutHeaderHeight+28, "No notifications", color.RGBA{R: 0x77, G: 0x77, B: 0x77, A: 0xff})
		f.mu.Lock()
		f.hits = hits
		f.mu.Unlock()
		return
	}

	for _, n := range items {
		if y+flyoutCardHeight > height-6 {
			break
		}

		cardRect := image.Rect(8, y, width-8, y+flyoutCardHeight)
		drawRoundedRect(img, cardRect, 6, color.RGBA{R: 0x24, G: 0x24, B: 0x24, A: 0xff}, draw.Over)

		appName := n.AppName
		if appName == "" {
			appName = "System"
		}
		timeStr := n.CreatedAt.Format("15:04")
		f.drawText(img, 14, y+16, fmt.Sprintf("%s • %s", appName, timeStr), color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff})

		dismissRect := image.Rect(width-28, y+4, width-8, y+24)
		f.drawText(img, width-22, y+17, "x", color.RGBA{R: 0x99, G: 0x99, B: 0x99, A: 0xff})
		hits = append(hits, flyoutHit{rect: dismissRect, action: "dismiss", notifID: n.ID})

		summary := truncateString(n.Summary, 32)
		f.drawText(img, 14, y+34, summary, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})

		body := truncateString(n.Body, 38)
		if body != "" {
			f.drawText(img, 14, y+50, body, color.RGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff})
		}

		hits = append(hits, flyoutHit{rect: cardRect, action: "dismiss", notifID: n.ID})
		y += flyoutCardHeight + flyoutCardGap
	}

	f.mu.Lock()
	f.hits = hits
	f.mu.Unlock()
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
	if !f.isOpen || !f.hover.inside {
		f.mu.Unlock()
		return false
	}

	if ev.Button != 0x110 || ev.State != client.WlPointerButtonStatePressed {
		f.mu.Unlock()
		return true
	}

	pt := image.Pt(f.hover.x, f.hover.y)
	hitsCopy := make([]flyoutHit, len(f.hits))
	copy(hitsCopy, f.hits)
	f.mu.Unlock()

	for _, hit := range hitsCopy {
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
