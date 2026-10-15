package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"strings"
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
	flyoutWidth        = 340
	flyoutMaxHeight    = 420
	flyoutHeaderHeight = 30
	flyoutOuterPad     = 8
	flyoutCardPadX     = 10
	flyoutCardPadY     = 8
	flyoutCardGap      = 6
	flyoutThumbGap     = 10
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

type cardLayout struct {
	notif        Notification
	headerText   string
	summaryLines []string
	bodyLines    []string
	height       int
	hasThumb     bool
}

type NotificationFlyout struct {
	disp          *display.Display
	output        *display.Output
	service       *NotificationService
	icons         *IconResolver
	face          font.Face
	mu            sync.Mutex
	isOpen        bool
	surface       *surface.Surface
	layer         *shell.LayerSurface
	frame         *shm.Frame
	closeSub      func()
	newSub        func()
	hits          []flyoutHit
	hover         flyoutPointerState
	startTime     int64
	dirtyCh       chan struct{}
	stop          chan struct{}
	currentHeight int
}

func NewNotificationFlyout(disp *display.Display, output *display.Output, service *NotificationService, icons *IconResolver) *NotificationFlyout {
	f := &NotificationFlyout{
		disp:      disp,
		output:    output,
		service:   service,
		icons:     icons,
		face:      loadBarFont(output),
		startTime: time.Now().UnixMilli(),
	}

	// Automatically open the notification list whenever a new notification arrives
	f.newSub = service.SubscribeNew(func() {
		f.Open(int32(barSidePad))
	})

	return f
}

func (f *NotificationFlyout) Destroy() {
	f.mu.Lock()
	newSub := f.newSub
	f.newSub = nil
	f.mu.Unlock()
	if newSub != nil {
		newSub()
	}
	f.Close()
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
		f.markDirty()
		f.service.MarkAllRead()
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
	// ExclusiveZone: -1 anchors to raw output top (y=0).
	// Bar occupies y = barTopMargin (6) .. barTopMargin + barHeight (34).
	// Placing flyout at barTopMargin + barHeight + moduleGap (38) leaves a 4px gap below the bar.
	topMargin := int32(barTopMargin + barHeight + moduleGap)

	items := f.service.List()
	_, calcHeight := f.layoutCards(items, flyoutWidth)

	layer, err := shell.NewLayer(surf, f.disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionOverlay,
		Name:          "cheese-notification-flyout",
		Anchor:        shell.AnchorTop | shell.AnchorLeft,
		Width:         flyoutWidth,
		Height:        uint32(calcHeight),
		ExclusiveZone: -1,
		MarginTop:     topMargin,
		MarginLeft:    leftMargin,
		Output:        f.output.WlOutput(),
	})
	if err != nil {
		f.mu.Unlock()
		_ = surf.Close()
		log.Printf("NotificationFlyout: Failed to create layer surface: %v", err)
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
	f.currentHeight = calcHeight
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

			items := f.service.List()
			_, desiredHeight := f.layoutCards(items, flyoutWidth)

			f.mu.Lock()
			fr := f.frame
			ly := f.layer
			sf := f.surface
			curH := f.currentHeight
			if ly != nil && sf != nil && desiredHeight != curH {
				f.currentHeight = desiredHeight
				_ = ly.SetSize(flyoutWidth, uint32(desiredHeight))
				_ = sf.Commit()
				pending = false
				f.mu.Unlock()
				continue
			}
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

func (f *NotificationFlyout) lineMetrics() (ascent, descent, lineHeight int) {
	m := f.face.Metrics()
	ascent = m.Ascent.Ceil()
	descent = m.Descent.Ceil()
	lineHeight = ascent + descent + 3
	if lineHeight < 15 {
		lineHeight = 15
	}
	return ascent, descent, lineHeight
}

func (f *NotificationFlyout) layoutCards(items []Notification, width int) ([]cardLayout, int) {
	if len(items) == 0 {
		return nil, flyoutHeaderHeight + 44
	}

	_, _, lineHeight := f.lineMetrics()
	cardWidth := width - 2*flyoutOuterPad

	layouts := make([]cardLayout, 0, len(items))
	totalHeight := flyoutHeaderHeight + 4

	for _, n := range items {
		hasThumb := n.Thumbnail != nil
		innerTextWidth := cardWidth - 2*flyoutCardPadX
		if hasThumb {
			innerTextWidth -= notifThumbSize + flyoutThumbGap
		}
		headerMaxWidth := innerTextWidth - 22

		appName := n.AppName
		if appName == "" {
			appName = "System"
		}
		timeStr := n.CreatedAt.Format("15:04")
		headerRaw := fmt.Sprintf("%s • %s", appName, timeStr)
		headerText := truncateToWidth(f.face, headerRaw, headerMaxWidth)

		summary := n.Summary
		if summary == "" {
			summary = "(No title)"
		}
		summaryLines := wrapTextLines(f.face, summary, innerTextWidth, 2)

		var bodyLines []string
		if n.Body != "" {
			bodyLines = wrapTextLines(f.face, n.Body, innerTextWidth, 3)
		}

		linesCount := 1 + len(summaryLines) + len(bodyLines)
		cardH := 2*flyoutCardPadY + linesCount*lineHeight + 2
		if len(bodyLines) > 0 {
			cardH += 2
		}
		if hasThumb {
			minThumbCardH := 2*flyoutCardPadY + notifThumbSize
			if cardH < minThumbCardH {
				cardH = minThumbCardH
			}
		}

		if totalHeight+cardH+flyoutOuterPad > flyoutMaxHeight && len(layouts) > 0 {
			break
		}

		layouts = append(layouts, cardLayout{
			notif:        n,
			headerText:   headerText,
			summaryLines: summaryLines,
			bodyLines:    bodyLines,
			height:       cardH,
			hasThumb:     hasThumb,
		})
		totalHeight += cardH + flyoutCardGap
	}

	totalHeight = totalHeight - flyoutCardGap + flyoutOuterPad
	if totalHeight > flyoutMaxHeight {
		totalHeight = flyoutMaxHeight
	}
	return layouts, totalHeight
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
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Rect, image.NewUniform(color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00}), image.Point{}, draw.Src)

	containerRect := image.Rect(0, 0, width, height)
	drawRoundedRect(canvas, containerRect, moduleCornerRadius, color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xff}, draw.Src)

	hits := make([]flyoutHit, 0, 12)
	items := f.service.List()
	cards, _ := f.layoutCards(items, width)
	ascent, _, lineHeight := f.lineMetrics()

	// Header row
	headerRect := image.Rect(0, 0, width, flyoutHeaderHeight)
	title := fmt.Sprintf("Notifications (%d)", len(items))
	f.drawText(canvas, flyoutOuterPad+4, textBaseline(f.face, headerRect), title, color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff})

	if len(items) > 0 {
		clearText := "Clear"
		textW := font.MeasureString(f.face, clearText).Ceil()
		btnW := textW + 14
		btnH := 20
		btnX := width - flyoutOuterPad - btnW
		btnY := (flyoutHeaderHeight - btnH) / 2
		clearBtnRect := image.Rect(btnX, btnY, btnX+btnW, btnY+btnH)
		drawRoundedRect(canvas, clearBtnRect, 4, color.RGBA{R: 0x2a, G: 0x2a, B: 0x2a, A: 0xff}, draw.Over)
		f.drawText(canvas, btnX+7, textBaseline(f.face, clearBtnRect), clearText, color.RGBA{R: 0xcc, G: 0xcc, B: 0xcc, A: 0xff})
		hits = append(hits, flyoutHit{rect: clearBtnRect, action: "clear_all"})
	}

	if len(cards) == 0 {
		emptyText := "No notifications"
		emptyW := font.MeasureString(f.face, emptyText).Ceil()
		emptyRect := image.Rect(0, flyoutHeaderHeight, width, height)
		f.drawText(canvas, (width-emptyW)/2, textBaseline(f.face, emptyRect), emptyText, color.RGBA{R: 0x77, G: 0x77, B: 0x77, A: 0xff})
		writeARGB8888(pixels, canvas)
		f.mu.Lock()
		f.hits = hits
		f.mu.Unlock()
		return
	}

	y := flyoutHeaderHeight + 2
	for _, c := range cards {
		if y+c.height > height-4 {
			break
		}

		cardRect := image.Rect(flyoutOuterPad, y, width-flyoutOuterPad, y+c.height)
		drawRoundedRect(canvas, cardRect, moduleCornerRadius, color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff}, draw.Over)

		textX := cardRect.Min.X + flyoutCardPadX
		if c.hasThumb && c.notif.Thumbnail != nil {
			thumbX := cardRect.Min.X + flyoutCardPadX
			thumbY := cardRect.Min.Y + (cardRect.Dy()-notifThumbSize)/2
			thumbRect := image.Rect(thumbX, thumbY, thumbX+notifThumbSize, thumbY+notifThumbSize)
			draw.Draw(canvas, thumbRect, c.notif.Thumbnail, c.notif.Thumbnail.Bounds().Min, draw.Over)
			textX += notifThumbSize + flyoutThumbGap
		}

		curY := cardRect.Min.Y + flyoutCardPadY + ascent

		// 1. Header line (AppName • Time)
		f.drawText(canvas, textX, curY, c.headerText, color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff})

		// Dismiss '×' button
		dismissText := "×"
		dismissW := font.MeasureString(f.face, dismissText).Ceil()
		dismissRect := image.Rect(cardRect.Max.X-22, cardRect.Min.Y+4, cardRect.Max.X-4, cardRect.Min.Y+22)
		f.drawText(canvas, dismissRect.Min.X+(dismissRect.Dx()-dismissW)/2, curY, dismissText, color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff})
		hits = append(hits, flyoutHit{rect: dismissRect, action: "dismiss", notifID: c.notif.ID})

		curY += lineHeight + 1

		// 2. Summary lines
		for _, line := range c.summaryLines {
			f.drawText(canvas, textX, curY, line, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
			curY += lineHeight
		}

		// 3. Body lines
		if len(c.bodyLines) > 0 {
			curY += 1
			for _, line := range c.bodyLines {
				f.drawText(canvas, textX, curY, line, color.RGBA{R: 0xb0, G: 0xb0, B: 0xb0, A: 0xff})
				curY += lineHeight
			}
		}

		hits = append(hits, flyoutHit{rect: cardRect, action: "dismiss", notifID: c.notif.ID})
		y += c.height + flyoutCardGap
	}

	writeARGB8888(pixels, canvas)

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
				f.Close()
				return true
			}
			if hit.action == "dismiss" {
				f.service.Dismiss(hit.notifID)
				if f.service.Count() == 0 {
					f.Close()
				}
				return true
			}
		}
	}

	return true
}

func truncateToWidth(face font.Face, s string, maxWidth int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if maxWidth <= 0 || s == "" {
		return ""
	}
	if font.MeasureString(face, s).Ceil() <= maxWidth {
		return s
	}
	ellipsis := "..."
	ellipsisW := font.MeasureString(face, ellipsis).Ceil()
	targetW := maxWidth - ellipsisW
	if targetW <= 0 {
		return ellipsis
	}

	runes := []rune(s)
	for i := len(runes) - 1; i > 0; i-- {
		candidate := strings.TrimSpace(string(runes[:i]))
		if font.MeasureString(face, candidate).Ceil() <= targetW {
			return candidate + ellipsis
		}
	}
	return ellipsis
}

func wrapTextLines(face font.Face, text string, maxWidth int, maxLines int) []string {
	if maxWidth <= 0 || maxLines <= 0 {
		return nil
	}
	paragraphs := strings.Split(text, "\n")
	var lines []string

	for _, para := range paragraphs {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}
		current := ""
		for _, word := range words {
			if font.MeasureString(face, word).Ceil() > maxWidth {
				if current != "" {
					lines = append(lines, current)
					current = ""
				}
				for _, part := range splitLongWord(face, word, maxWidth) {
					if current == "" {
						current = part
					} else {
						lines = append(lines, current)
						current = part
					}
				}
				continue
			}

			candidate := word
			if current != "" {
				candidate = current + " " + word
			}
			if font.MeasureString(face, candidate).Ceil() <= maxWidth {
				current = candidate
			} else {
				lines = append(lines, current)
				current = word
			}
		}
		if current != "" {
			lines = append(lines, current)
		}
	}

	if len(lines) > maxLines {
		remaining := strings.Join(lines[maxLines-1:], " ")
		lines = lines[:maxLines]
		lines[maxLines-1] = truncateToWidth(face, remaining, maxWidth)
	}
	return lines
}

func splitLongWord(face font.Face, word string, maxWidth int) []string {
	var parts []string
	var cur strings.Builder
	for _, r := range word {
		cand := cur.String() + string(r)
		if cur.Len() > 0 && font.MeasureString(face, cand).Ceil() > maxWidth {
			parts = append(parts, cur.String())
			cur.Reset()
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}
