package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/shm"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/protocols/cursor_shape_v1"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	flyoutWidth          = 340
	flyoutMaxHeight      = 420
	flyoutHeaderHeight   = 30
	flyoutOuterPad       = 8
	flyoutCardPadX       = 10
	flyoutCardPadY       = 8
	flyoutCardGap        = 6
	flyoutThumbGap       = 10
	flyoutUnreadDotWidth = 12
	flyoutAutoHideDelay  = 4 * time.Second
)

type flyoutPointerState struct {
	inside bool
	x      int
	y      int
	serial uint32
	shape  cursor_shape_v1.WpCursorShapeDeviceV1Shape
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
	disp             *display.Display
	output           *display.Output
	service          *NotificationService
	icons            *IconResolver
	face             font.Face
	onCloseAll       func()
	mu               sync.Mutex
	isOpen           bool
	autoOpened       bool
	triggeredByUser  bool
	autoHideDeadline time.Time
	surface          *surface.Surface
	layer            *shell.LayerSurface
	frame            *shm.Frame
	backdropSurf     *surface.Surface
	backdropLayer    *shell.LayerSurface
	backdropFrame    *shm.Frame
	backdropHover    bool
	closeSub         func()
	newSub           func()
	hits             []flyoutHit
	hover            flyoutPointerState
	startTime        int64
	dirtyCh          chan struct{}
	stop             chan struct{}
	currentHeight     int
	scrollOffset      float64
	draggingScrollbar bool
	dragStartMouseY   int
	dragStartScroll   float64
	scrollHitRect     image.Rectangle
	scrollThumbRect   image.Rectangle
	scrollTrackAvail  int
	scrollMax         float64
}

func NewNotificationFlyout(disp *display.Display, output *display.Output, service *NotificationService, icons *IconResolver, onCloseAll func()) *NotificationFlyout {
	f := &NotificationFlyout{
		disp:       disp,
		output:     output,
		service:    service,
		icons:      icons,
		face:       loadBarFont(output),
		onCloseAll: onCloseAll,
		startTime:  time.Now().UnixMilli(),
	}

	// Automatically open the notification list whenever a new notification arrives,
	// and auto-hide after flyoutAutoHideDelay without marking items as read.
	f.newSub = service.SubscribeNew(func() {
		f.mu.Lock()
		f.scrollOffset = 0
		f.mu.Unlock()
		f.OpenAuto(int32(barSidePad))
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

// Toggle is called when the user clicks the notification trigger on the top bar.
// Triggering means the user has read the notifications.
func (f *NotificationFlyout) Toggle(anchorX int32) {
	f.mu.Lock()
	open := f.isOpen
	f.mu.Unlock()

	if open {
		f.service.MarkAllRead()
		if f.onCloseAll != nil {
			f.onCloseAll()
		} else {
			f.Close()
		}
	} else {
		f.OpenManual(anchorX)
	}
}

func (f *NotificationFlyout) OpenAuto(anchorX int32) {
	f.openInternal(anchorX, true)
}

func (f *NotificationFlyout) OpenManual(anchorX int32) {
	f.openInternal(anchorX, false)
}

func (f *NotificationFlyout) Open(anchorX int32) {
	f.OpenManual(anchorX)
}

func (f *NotificationFlyout) openInternal(anchorX int32, auto bool) {
	f.mu.Lock()
	if f.isOpen {
		if auto {
			if f.autoOpened {
				f.autoHideDeadline = time.Now().Add(flyoutAutoHideDelay)
			}
		} else {
			f.autoOpened = false
			f.triggeredByUser = true
		}
		f.mu.Unlock()
		f.markDirty()
		return
	}

	// 1. Transparent click-outside backdrop on LayerPositionTop
	bdSurf, err := surface.New(f.disp.Compositor())
	if err != nil {
		f.mu.Unlock()
		return
	}
	bdLayer, err := shell.NewLayer(bdSurf, f.disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          "cheese-notif-backdrop",
		Anchor:        shell.AnchorTop | shell.AnchorBottom | shell.AnchorLeft | shell.AnchorRight,
		Width:         0,
		Height:        0,
		ExclusiveZone: -1,
		MarginTop:     int32(barTopMargin + barHeight),
		Output:        f.output.WlOutput(),
	})
	if err != nil {
		f.mu.Unlock()
		_ = bdSurf.Close()
		return
	}
	bdFrame, err := shm.NewFrame(f.disp.Shm(), bdLayer, f.disp, shm.FrameConfig{
		Format:  client.WlShmFormatArgb8888,
		Buffers: 1,
	})
	if err != nil {
		f.mu.Unlock()
		_ = bdLayer.Close()
		_ = bdSurf.Close()
		return
	}
	bdFrame.SetRender(func(width, height int, frameTime uint32, pixels []byte) {})
	bdFrame.SetManualMode(true)

	// 2. Notification flyout on LayerPositionOverlay
	surf, err := surface.New(f.disp.Compositor())
	if err != nil {
		f.mu.Unlock()
		_ = bdFrame.Close()
		_ = bdLayer.Close()
		_ = bdSurf.Close()
		log.Printf("NotificationFlyout: Failed to create surface: %v", err)
		return
	}

	leftMargin := anchorX
	if leftMargin < barSidePad {
		leftMargin = barSidePad
	}
	topMargin := int32(barTopMargin + barHeight + moduleGap)

	items := f.service.List()
	_, _, calcHeight := f.layoutCards(items, flyoutWidth)

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
		_ = bdFrame.Close()
		_ = bdLayer.Close()
		_ = bdSurf.Close()
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
		_ = bdFrame.Close()
		_ = bdLayer.Close()
		_ = bdSurf.Close()
		return
	}

	f.backdropSurf = bdSurf
	f.backdropLayer = bdLayer
	f.backdropFrame = bdFrame
	f.backdropHover = false
	f.surface = surf
	f.layer = layer
	f.frame = frame
	f.isOpen = true
	f.autoOpened = auto
	f.triggeredByUser = !auto
	if auto {
		f.autoHideDeadline = time.Now().Add(flyoutAutoHideDelay)
	}
	f.currentHeight = calcHeight
	f.scrollOffset = 0
	f.dirtyCh = make(chan struct{}, 1)
	f.stop = make(chan struct{})

	frame.SetRender(func(width, height int, frameTime uint32, pixels []byte) {
		f.draw(width, height, pixels)
	})
	frame.SetManualMode(true)

	layer.OnClose(func() {
		f.Close()
	})
	bdLayer.OnClose(func() {
		f.Close()
	})

	stopCh := f.stop
	dirtyCh := f.dirtyCh
	go func() {
		ticker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		pending := true
		bdMapped := false
		for {
			select {
			case <-dirtyCh:
				pending = true
			case <-ticker.C:
			case <-stopCh:
				return
			}

			f.mu.Lock()
			bdf := f.backdropFrame
			autoOpen := f.autoOpened
			inside := f.hover.inside
			if autoOpen && inside {
				f.autoHideDeadline = time.Now().Add(2 * time.Second)
			}
			expired := autoOpen && !inside && time.Now().After(f.autoHideDeadline)
			f.mu.Unlock()

			if expired {
				go f.Close()
				return
			}

			if !bdMapped && bdf != nil && bdf.Ready() {
				bdf.ManualRender(0)
				bdMapped = true
			}

			if !pending {
				continue
			}

			items := f.service.List()
			_, _, desiredHeight := f.layoutCards(items, flyoutWidth)

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

func (f *NotificationFlyout) layoutCards(items []Notification, width int) (layouts []cardLayout, contentHeight int, surfaceHeight int) {
	if len(items) == 0 {
		return nil, 0, flyoutHeaderHeight + 44
	}

	_, _, lineHeight := f.lineMetrics()
	cardWidth := width - 2*flyoutOuterPad

	layouts = make([]cardLayout, 0, len(items))
	contentHeight = 0

	for _, n := range items {
		hasThumb := n.Thumbnail != nil
		fullInnerWidth := cardWidth - 2*flyoutCardPadX
		headerMaxWidth := fullInnerWidth - 24
		if n.Unread {
			headerMaxWidth -= flyoutUnreadDotWidth
		}
		textMaxWidth := fullInnerWidth
		if hasThumb {
			textMaxWidth -= notifThumbSize + flyoutThumbGap
		}

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
		summaryLines := wrapTextLines(f.face, summary, textMaxWidth, 2)

		var bodyLines []string
		if n.Body != "" {
			bodyLines = wrapTextLines(f.face, n.Body, textMaxWidth, 3)
		}

		bodyBlockH := (len(summaryLines) + len(bodyLines)) * lineHeight
		if len(bodyLines) > 0 {
			bodyBlockH += 2
		}
		if hasThumb && bodyBlockH < notifThumbSize {
			bodyBlockH = notifThumbSize
		}
		cardH := 2*flyoutCardPadY + lineHeight + 4 + bodyBlockH

		layouts = append(layouts, cardLayout{
			notif:        n,
			headerText:   headerText,
			summaryLines: summaryLines,
			bodyLines:    bodyLines,
			height:       cardH,
			hasThumb:     hasThumb,
		})
		contentHeight += cardH + flyoutCardGap
	}

	if contentHeight > 0 {
		contentHeight -= flyoutCardGap
	}
	surfaceHeight = flyoutHeaderHeight + 2 + contentHeight + flyoutOuterPad
	if surfaceHeight > flyoutMaxHeight {
		surfaceHeight = flyoutMaxHeight
	}
	return layouts, contentHeight, surfaceHeight
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
	markRead := f.triggeredByUser
	f.triggeredByUser = false
	f.autoOpened = false
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
	bdFrame := f.backdropFrame
	f.backdropFrame = nil
	bdLayer := f.backdropLayer
	f.backdropLayer = nil
	bdSurf := f.backdropSurf
	f.backdropSurf = nil
	f.backdropHover = false
	f.hover = flyoutPointerState{}
	f.scrollOffset = 0
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
	if bdFrame != nil {
		_ = bdFrame.Close()
	}
	if bdLayer != nil {
		_ = bdLayer.Close()
	}
	if bdSurf != nil {
		_ = bdSurf.Close()
	}

	if markRead {
		f.service.MarkAllRead()
	}
}

func (f *NotificationFlyout) draw(width, height int, pixels []byte) {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Rect, image.NewUniform(color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00}), image.Point{}, draw.Src)

	containerRect := image.Rect(0, 0, width, height)
	drawSmoothRoundedRect(canvas, containerRect, float64(moduleCornerRadius), color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xff}, draw.Src)

	hits := make([]flyoutHit, 0, 16)
	items := f.service.List()
	cards, contentHeight, _ := f.layoutCards(items, width)
	ascent, _, lineHeight := f.lineMetrics()

	f.mu.Lock()
	hoverInside := f.hover.inside
	hoverPt := image.Pt(f.hover.x, f.hover.y)
	f.mu.Unlock()

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
		clearBg := color.RGBA{R: 0x2a, G: 0x2a, B: 0x2a, A: 0xff}
		clearFg := color.RGBA{R: 0xcc, G: 0xcc, B: 0xcc, A: 0xff}
		if hoverInside && hoverPt.In(clearBtnRect) {
			clearBg = color.RGBA{R: 0x3a, G: 0x3a, B: 0x3a, A: 0xff}
			clearFg = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		}
		drawSmoothRoundedRect(canvas, clearBtnRect, 4, clearBg, draw.Over)
		f.drawText(canvas, btnX+7, textBaseline(f.face, clearBtnRect), clearText, clearFg)
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

	viewportTop := flyoutHeaderHeight + 2
	viewportBottom := height - flyoutOuterPad
	if viewportBottom <= viewportTop {
		viewportBottom = height
	}
	viewportHeight := viewportBottom - viewportTop
	viewportRect := image.Rect(0, viewportTop, width, viewportBottom)

	maxScroll := float64(contentHeight - viewportHeight)
	if maxScroll < 0 {
		maxScroll = 0
	}

	f.mu.Lock()
	if f.scrollOffset < 0 {
		f.scrollOffset = 0
	}
	if f.scrollOffset > maxScroll {
		f.scrollOffset = maxScroll
	}
	scrollY := int(math.Round(f.scrollOffset))
	f.mu.Unlock()

	vpCanvas := image.NewRGBA(image.Rect(0, 0, width, viewportHeight))
	y := -scrollY

	cardRight := width - flyoutOuterPad
	if maxScroll > 0 {
		cardRight = width - 11
	}

	for _, c := range cards {
		if y+c.height <= 0 {
			y += c.height + flyoutCardGap
			continue
		}
		if y >= viewportHeight {
			break
		}

		cardRect := image.Rect(flyoutOuterPad, y, cardRight, y+c.height)
		drawSmoothRoundedRect(vpCanvas, cardRect, float64(moduleCornerRadius), color.RGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xff}, draw.Over)

		textX := cardRect.Min.X + flyoutCardPadX
		curY := cardRect.Min.Y + flyoutCardPadY + ascent

		// 1. Header line with blue unread indicator dot consistently aligned on the left
		headerX := textX
		if c.notif.Unread {
			dotX := math.Floor(float64(textX)+3.0) + 0.5
			glyphBounds, _ := font.BoundString(f.face, "A0")
			glyphCenterOffset := float64(glyphBounds.Min.Y+glyphBounds.Max.Y) / 128.0
			dotY := math.Floor(float64(curY)+glyphCenterOffset+1.0) + 0.5
			drawSmoothCircle(vpCanvas, dotX, dotY, 3.0, color.RGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff})
			headerX += flyoutUnreadDotWidth
		}
		f.drawText(vpCanvas, headerX, curY, c.headerText, color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff})

		// Top-right close button '×' with rounded square background on hover
		dismissText := "×"
		dismissW := font.MeasureString(f.face, dismissText).Ceil()
		dismissRect := image.Rect(cardRect.Max.X-24, cardRect.Min.Y+4, cardRect.Max.X-4, cardRect.Min.Y+24)
		surfDismissRect := dismissRect.Add(image.Pt(0, viewportTop)).Intersect(viewportRect)
		dismissCol := color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
		if hoverInside && !surfDismissRect.Empty() && hoverPt.In(surfDismissRect) {
			drawSmoothRoundedRect(vpCanvas, dismissRect, 4.0, color.RGBA{R: 0x38, G: 0x38, B: 0x38, A: 0xff}, draw.Over)
			dismissCol = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		}
		f.drawText(vpCanvas, dismissRect.Min.X+(dismissRect.Dx()-dismissW)/2, textBaseline(f.face, dismissRect), dismissText, dismissCol)
		if !surfDismissRect.Empty() {
			hits = append(hits, flyoutHit{rect: surfDismissRect, action: "dismiss", notifID: c.notif.ID})
		}

		// 2. Content area below header: summary/body on the left, thumbnail image on the right
		contentTopY := cardRect.Min.Y + flyoutCardPadY + lineHeight + 4
		if c.hasThumb && c.notif.Thumbnail != nil {
			thumbX := cardRect.Max.X - flyoutCardPadX - notifThumbSize
			thumbY := contentTopY
			thumbRect := image.Rect(thumbX, thumbY, thumbX+notifThumbSize, thumbY+notifThumbSize)
			draw.Draw(vpCanvas, thumbRect, c.notif.Thumbnail, c.notif.Thumbnail.Bounds().Min, draw.Over)
		}

		curY = contentTopY + ascent
		for _, line := range c.summaryLines {
			f.drawText(vpCanvas, textX, curY, line, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
			curY += lineHeight
		}

		if len(c.bodyLines) > 0 {
			curY += 2
			for _, line := range c.bodyLines {
				f.drawText(vpCanvas, textX, curY, line, color.RGBA{R: 0xb0, G: 0xb0, B: 0xb0, A: 0xff})
				curY += lineHeight
			}
		}

		surfCardRect := cardRect.Add(image.Pt(0, viewportTop)).Intersect(viewportRect)
		if !surfCardRect.Empty() {
			hits = append(hits, flyoutHit{rect: surfCardRect, action: "dismiss", notifID: c.notif.ID})
		}
		y += c.height + flyoutCardGap
	}

	draw.Draw(canvas, viewportRect, vpCanvas, image.Point{}, draw.Over)

	var scrollHitRect, scrollThumbRect image.Rectangle
	var scrollTrackAvail int
	if maxScroll > 0 && contentHeight > 0 {
		thumbH := int(math.Round(float64(viewportHeight) * float64(viewportHeight) / float64(contentHeight)))
		if thumbH < 24 {
			thumbH = 24
		}
		if thumbH > viewportHeight {
			thumbH = viewportHeight
		}
		trackAvail := viewportHeight - thumbH
		thumbOffset := 0
		if trackAvail > 0 {
			thumbOffset = int(math.Round((float64(scrollY) / maxScroll) * float64(trackAvail)))
		}
		scrollTrackAvail = trackAvail
		scrollHitRect = image.Rect(width-12, viewportTop, width, viewportBottom)
		scrollThumbRect = image.Rect(width-12, viewportTop+thumbOffset, width, viewportTop+thumbOffset+thumbH)

		f.mu.Lock()
		dragging := f.draggingScrollbar
		f.mu.Unlock()
		scrollHovered := dragging || (hoverInside && hoverPt.In(scrollHitRect))

		if scrollHovered {
			trackDrawRect := image.Rect(width-8, viewportTop, width-2, viewportBottom)
			drawSmoothRoundedRect(canvas, trackDrawRect, 3.0, color.RGBA{R: 0x26, G: 0x26, B: 0x26, A: 0xff}, draw.Over)
			thumbCol := color.RGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff}
			if dragging {
				thumbCol = color.RGBA{R: 0xd0, G: 0xd0, B: 0xd0, A: 0xff}
			}
			barRect := image.Rect(width-8, viewportTop+thumbOffset, width-2, viewportTop+thumbOffset+thumbH)
			drawSmoothRoundedRect(canvas, barRect, 3.0, thumbCol, draw.Over)
		} else {
			barRect := image.Rect(width-5, viewportTop+thumbOffset, width-2, viewportTop+thumbOffset+thumbH)
			drawSmoothRoundedRect(canvas, barRect, 1.5, color.RGBA{R: 0x66, G: 0x66, B: 0x66, A: 0xff}, draw.Over)
		}
	}

	writeARGB8888(pixels, canvas)

	f.mu.Lock()
	f.hits = hits
	f.scrollHitRect = scrollHitRect
	f.scrollThumbRect = scrollThumbRect
	f.scrollTrackAvail = scrollTrackAvail
	f.scrollMax = maxScroll
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

func (f *NotificationFlyout) updateCursorShapeLocked(cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	if cursor == nil || !f.hover.inside || f.hover.serial == 0 {
		return
	}
	pt := image.Pt(f.hover.x, f.hover.y)
	clickable := f.draggingScrollbar
	if !clickable && f.scrollMax > 0 && !f.scrollHitRect.Empty() && pt.In(f.scrollHitRect) {
		clickable = true
	}
	if !clickable {
		for _, hit := range f.hits {
			if pt.In(hit.rect) {
				clickable = true
				break
			}
		}
	}
	shape := cursor_shape_v1.WpCursorShapeDeviceV1ShapeDefault
	if clickable {
		shape = cursor_shape_v1.WpCursorShapeDeviceV1ShapePointer
	}
	if shape != f.hover.shape {
		_ = cursor.SetShape(f.hover.serial, shape)
		f.hover.shape = shape
	}
}

func (f *NotificationFlyout) HandlePointerEnter(ev client.WlPointerEnterEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return
	}
	if f.backdropSurf != nil && ev.Surface == f.backdropSurf.WlSurface() {
		f.backdropHover = true
		f.mu.Unlock()
		if cursor != nil {
			_ = cursor.SetShape(ev.Serial, cursor_shape_v1.WpCursorShapeDeviceV1ShapeDefault)
		}
		return
	}
	if f.surface == nil || ev.Surface != f.surface.WlSurface() {
		f.mu.Unlock()
		return
	}
	f.hover.inside = true
	f.hover.x = int(ev.SurfaceX)
	f.hover.y = int(ev.SurfaceY)
	f.hover.serial = ev.Serial
	f.hover.shape = 0
	f.updateCursorShapeLocked(cursor)
	f.mu.Unlock()
	f.markDirty()
}

func (f *NotificationFlyout) HandlePointerLeave(ev client.WlPointerLeaveEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return
	}
	if f.backdropSurf != nil && ev.Surface == f.backdropSurf.WlSurface() {
		f.backdropHover = false
		f.mu.Unlock()
		return
	}
	if f.surface == nil || ev.Surface != f.surface.WlSurface() {
		f.mu.Unlock()
		return
	}
	f.hover.inside = false
	f.hover.shape = 0
	f.draggingScrollbar = false
	f.mu.Unlock()
	if cursor != nil {
		_ = cursor.SetShape(ev.Serial, cursor_shape_v1.WpCursorShapeDeviceV1ShapeDefault)
	}
	f.markDirty()
}

func (f *NotificationFlyout) HandlePointerMotion(ev client.WlPointerMotionEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	f.mu.Lock()
	if !f.isOpen || !f.hover.inside {
		f.mu.Unlock()
		return
	}
	f.hover.x = int(ev.SurfaceX)
	f.hover.y = int(ev.SurfaceY)
	if f.draggingScrollbar && f.scrollTrackAvail > 0 && f.scrollMax > 0 {
		dy := f.hover.y - f.dragStartMouseY
		next := f.dragStartScroll + (float64(dy)/float64(f.scrollTrackAvail))*f.scrollMax
		if next < 0 {
			next = 0
		}
		if next > f.scrollMax {
			next = f.scrollMax
		}
		f.scrollOffset = next
	}
	f.updateCursorShapeLocked(cursor)
	f.mu.Unlock()
	f.markDirty()
}

func (f *NotificationFlyout) SimulateHover(x, y int) {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return
	}
	if x < 0 || y < 0 {
		f.hover.inside = false
	} else {
		f.hover.inside = true
		f.hover.x = x
		f.hover.y = y
	}
	f.mu.Unlock()
	f.markDirty()
}

func (f *NotificationFlyout) ScrollBy(delta float64) {
	items := f.service.List()
	_, contentHeight, surfHeight := f.layoutCards(items, flyoutWidth)
	viewportHeight := (surfHeight - flyoutOuterPad) - (flyoutHeaderHeight + 2)
	maxScroll := float64(contentHeight - viewportHeight)
	if maxScroll < 0 {
		maxScroll = 0
	}

	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return
	}
	next := f.scrollOffset + delta
	if next < 0 {
		next = 0
	}
	if next > maxScroll {
		next = maxScroll
	}
	changed := math.Abs(next-f.scrollOffset) > 0.1
	f.scrollOffset = next
	f.mu.Unlock()

	if changed {
		f.markDirty()
	}
}

func (f *NotificationFlyout) HandlePointerAxis(ev client.WlPointerAxisEvent) {
	if ev.Axis != client.WlPointerAxisVerticalScroll {
		return
	}
	f.mu.Lock()
	active := f.isOpen && f.hover.inside
	f.mu.Unlock()
	if !active {
		return
	}
	f.ScrollBy(ev.Value * 2.0)
}

func (f *NotificationFlyout) HandlePointerButton(ev client.WlPointerButtonEvent) bool {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return false
	}
	if f.backdropHover {
		f.mu.Unlock()
		if ev.State == client.WlPointerButtonStatePressed {
			if f.onCloseAll != nil {
				f.onCloseAll()
			} else {
				f.Close()
			}
		}
		return true
	}
	if !f.hover.inside {
		f.mu.Unlock()
		return false
	}

	if ev.Button == 0x110 && ev.State == client.WlPointerButtonStateReleased {
		wasDragging := f.draggingScrollbar
		f.draggingScrollbar = false
		f.mu.Unlock()
		if wasDragging {
			f.markDirty()
		}
		return true
	}

	if ev.Button != 0x110 || ev.State != client.WlPointerButtonStatePressed {
		f.mu.Unlock()
		return true
	}

	pt := image.Pt(f.hover.x, f.hover.y)
	if f.scrollMax > 0 && !f.scrollHitRect.Empty() && pt.In(f.scrollHitRect) {
		if !pt.In(f.scrollThumbRect) && f.scrollTrackAvail > 0 {
			thumbH := f.scrollThumbRect.Dy()
			relY := float64(pt.Y - f.scrollHitRect.Min.Y - thumbH/2)
			next := (relY / float64(f.scrollTrackAvail)) * f.scrollMax
			if next < 0 {
				next = 0
			}
			if next > f.scrollMax {
				next = f.scrollMax
			}
			f.scrollOffset = next
		}
		f.draggingScrollbar = true
		f.dragStartMouseY = pt.Y
		f.dragStartScroll = f.scrollOffset
		f.mu.Unlock()
		f.markDirty()
		return true
	}

	hitsCopy := make([]flyoutHit, len(f.hits))
	copy(hitsCopy, f.hits)
	f.mu.Unlock()

	for _, hit := range hitsCopy {
		if pt.In(hit.rect) {
			if hit.action == "clear_all" {
				f.service.ClearAll()
				if f.onCloseAll != nil {
					f.onCloseAll()
				} else {
					f.Close()
				}
				return true
			}
			if hit.action == "dismiss" {
				f.service.Dismiss(hit.notifID)
				if f.service.Count() == 0 {
					if f.onCloseAll != nil {
						f.onCloseAll()
					} else {
						f.Close()
					}
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
