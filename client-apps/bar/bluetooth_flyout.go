package main

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"os/exec"
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
	btFlyoutWidth  = 260
	btFlyoutHeight = 280
	btHeaderHeight = 38
	btRowHeight    = 28
)

type btHit struct {
	rect   image.Rectangle
	action string
	device BluetoothDevice
}

type BluetoothFlyout struct {
	disp          *display.Display
	output        *display.Output
	bluetooth     *BluetoothService
	face          font.Face
	mu            sync.Mutex
	isOpen        bool
	barWidth      int
	surface       *surface.Surface
	layer         *shell.LayerSurface
	frame         *shm.Frame
	backdropSurf  *surface.Surface
	backdropLayer *shell.LayerSurface
	backdropFrame *shm.Frame
	backdropHover bool
	hover         flyoutPointerState
	hits          []btHit
	devices       []BluetoothDevice
	powered       bool
	currentHeight int
	viewport      ScrollViewport
	startTime     int64
	dirtyCh       chan struct{}
	stop          chan struct{}
}

func NewBluetoothFlyout(disp *display.Display, output *display.Output, bluetooth *BluetoothService) *BluetoothFlyout {
	return &BluetoothFlyout{
		disp:      disp,
		output:    output,
		bluetooth: bluetooth,
		face:      loadBarFont(output),
		startTime: time.Now().UnixMilli(),
	}
}

func (f *BluetoothFlyout) SetBarWidth(w int) {
	f.mu.Lock()
	f.barWidth = w
	f.mu.Unlock()
}

func (f *BluetoothFlyout) IsOpen() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.isOpen
}

func (f *BluetoothFlyout) Destroy() {
	f.Close()
}

func (f *BluetoothFlyout) Toggle(moduleRect image.Rectangle) {
	f.mu.Lock()
	open := f.isOpen
	f.mu.Unlock()

	if open {
		f.Close()
	} else {
		f.Open(moduleRect)
	}
}

func (f *BluetoothFlyout) Open(moduleRect image.Rectangle) {
	f.mu.Lock()
	if f.isOpen {
		f.mu.Unlock()
		f.markDirty()
		return
	}

	bdSurf, err := surface.New(f.disp.Compositor())
	if err != nil {
		f.mu.Unlock()
		return
	}
	bdLayer, err := shell.NewLayer(bdSurf, f.disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          "cheese-bt-backdrop",
		Anchor:        shell.AnchorTop | shell.AnchorBottom | shell.AnchorLeft | shell.AnchorRight,
		Width:         0,
		Height:        0,
		ExclusiveZone: -1,
		MarginTop:     int32(barTopMargin + barHeight),
		MarginBottom:  int32(dockBottomMargin + dockShelfHeight + dockWindowGap),
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

	surf, err := surface.New(f.disp.Compositor())
	if err != nil {
		f.mu.Unlock()
		_ = bdFrame.Close()
		_ = bdLayer.Close()
		_ = bdSurf.Close()
		log.Printf("BluetoothFlyout: Failed to create surface: %v", err)
		return
	}

	centerX := (moduleRect.Min.X + moduleRect.Max.X) / 2
	leftMargin := int32(centerX - btFlyoutWidth/2)
	if f.barWidth > 0 {
		maxLeft := int32(f.barWidth - btFlyoutWidth - barSidePad)
		if leftMargin > maxLeft {
			leftMargin = maxLeft
		}
	}
	if leftMargin < barSidePad {
		leftMargin = barSidePad
	}
	topMargin := int32(barTopMargin + barHeight + moduleGap)

	devices := f.devices
	powered := f.powered
	if st := f.bluetooth.State(); st.Powered {
		powered = true
	}
	_, calcHeight := f.computeContentAndSurfaceHeight(devices, powered)

	layer, err := shell.NewLayer(surf, f.disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionOverlay,
		Name:          "cheese-bluetooth-flyout",
		Anchor:        shell.AnchorTop | shell.AnchorLeft,
		Width:         btFlyoutWidth,
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
		log.Printf("BluetoothFlyout: Failed to create layer surface: %v", err)
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
	f.devices = devices
	f.powered = powered
	f.currentHeight = calcHeight
	f.viewport.ScrollOffset = 0
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
		ticker := time.NewTicker(2 * time.Second)
		renderTicker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		defer renderTicker.Stop()
		pending := true
		bdMapped := false

		load := func() {
			devs, pwr := f.bluetooth.ListDevices()
			f.mu.Lock()
			f.devices = devs
			f.powered = pwr
			f.mu.Unlock()
		}
		load()

		for {
			select {
			case <-dirtyCh:
				pending = true
			case <-renderTicker.C:
			case <-ticker.C:
				load()
				pending = true
			case <-stopCh:
				return
			}

			f.mu.Lock()
			bdf := f.backdropFrame
			f.mu.Unlock()

			if !bdMapped && bdf != nil && bdf.Ready() {
				bdf.ManualRender(0)
				bdMapped = true
			}

			if !pending {
				continue
			}

			f.mu.Lock()
			fr := f.frame
			ly := f.layer
			sf := f.surface
			devs := f.devices
			pwr := f.powered
			curH := f.currentHeight
			f.mu.Unlock()

			_, desiredHeight := f.computeContentAndSurfaceHeight(devs, pwr)
			if ly != nil && sf != nil && desiredHeight != curH {
				f.mu.Lock()
				f.currentHeight = desiredHeight
				f.mu.Unlock()
				_ = ly.SetSize(btFlyoutWidth, uint32(desiredHeight))
				_ = sf.Commit()
				pending = false
				continue
			}

			if fr != nil && fr.Ready() {
				fr.ManualRender(uint32(time.Now().UnixMilli() - f.startTime))
				pending = false
			}
		}
	}()

	f.mu.Unlock()
	f.markDirty()
}

func (f *BluetoothFlyout) computeContentAndSurfaceHeight(devices []BluetoothDevice, powered bool) (contentHeight, surfaceHeight int) {
	if !powered {
		return 36, btHeaderHeight + 42
	}
	h := 4
	connectedCount := 0
	for _, d := range devices {
		if d.Connected {
			connectedCount++
		}
	}
	if connectedCount > 0 {
		h += 18 + connectedCount*btRowHeight + 6
	}
	otherCount := len(devices) - connectedCount
	if otherCount > 0 {
		h += 18 + otherCount*btRowHeight + 6
	} else if connectedCount == 0 {
		h += 34 + 6
	}
	h += btRowHeight + 6 // Bluetooth Settings row + padding
	contentHeight = h

	surfaceHeight = btHeaderHeight + 1 + contentHeight + 4
	if surfaceHeight > btFlyoutHeight {
		surfaceHeight = btFlyoutHeight
	}
	return contentHeight, surfaceHeight
}

func (f *BluetoothFlyout) Close() {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return
	}
	f.isOpen = false
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
	f.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
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
}

func (f *BluetoothFlyout) markDirty() {
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

func (f *BluetoothFlyout) draw(width, height int, pixels []byte) {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Rect, image.NewUniform(color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00}), image.Point{}, draw.Src)

	containerRect := image.Rect(0, 0, width, height)
	drawSmoothRoundedRect(canvas, containerRect, float64(moduleCornerRadius), color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xff}, draw.Src)

	f.mu.Lock()
	hoverInside := f.hover.inside
	hoverPt := image.Pt(f.hover.x, f.hover.y)
	devices := f.devices
	powered := f.powered
	f.mu.Unlock()

	hits := make([]btHit, 0, 16)

	// 1. Header row: "Bluetooth" on the left, macOS toggle switch on the right
	headerRect := image.Rect(0, 0, width, btHeaderHeight)
	f.drawText(canvas, 14, textBaseline(f.face, headerRect), "Bluetooth", color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})

	switchW := 34
	switchH := 20
	switchX := width - 14 - switchW
	switchY := (btHeaderHeight - switchH) / 2
	switchRect := image.Rect(switchX, switchY, switchX+switchW, switchY+switchH)

	switchHovered := hoverInside && hoverPt.In(switchRect)
	switchBg := color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff}
	knobCenterX := float64(switchX + 10)
	if powered {
		switchBg = color.RGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff}
		if switchHovered {
			switchBg = color.RGBA{R: 0x5a, G: 0x9b, B: 0xf8, A: 0xff}
		}
		knobCenterX = float64(switchX + switchW - 10)
	} else if switchHovered {
		switchBg = color.RGBA{R: 0x48, G: 0x48, B: 0x48, A: 0xff}
	}
	drawSmoothRoundedRect(canvas, switchRect, float64(switchH)/2.0, switchBg, draw.Over)
	drawSmoothCircle(canvas, knobCenterX, float64(switchY+switchH/2), 7.0, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	hits = append(hits, btHit{rect: switchRect, action: "toggle_power"})

	// Divider below header
	drawDivider(canvas, 14, btHeaderHeight, width-14)

	// 2. Scrollable Viewport below header
	viewportTop := btHeaderHeight + 2
	viewportBottom := height - 4
	contentH, _ := f.computeContentAndSurfaceHeight(devices, powered)

	f.mu.Lock()
	f.viewport.Setup(viewportTop, viewportBottom, contentH, width)
	f.mu.Unlock()

	cardRight := width - 8
	if f.viewport.MaxScroll > 0 {
		cardRight = width - 11
	}

	f.viewport.Render(canvas, width, hoverInside, hoverPt, func(vpCanvas *image.RGBA, scrollY int) {
		y := 4 - scrollY

		if !powered {
			offRect := image.Rect(14, y, width-14, y+30)
			f.drawText(vpCanvas, 14, textBaseline(f.face, offRect), "Bluetooth is turned off", color.RGBA{R: 0x77, G: 0x77, B: 0x77, A: 0xff})
			return
		}

		var connectedDevs, otherDevs []BluetoothDevice
		for _, d := range devices {
			if d.Connected {
				connectedDevs = append(connectedDevs, d)
			} else {
				otherDevs = append(otherDevs, d)
			}
		}

		if len(connectedDevs) > 0 {
			lblRect := image.Rect(14, y, width-14, y+18)
			f.drawText(vpCanvas, 14, textBaseline(f.face, lblRect), "Connected Devices", color.RGBA{R: 0x77, G: 0x77, B: 0x77, A: 0xff})
			y += 18

			for _, d := range connectedDevs {
				rowRect := image.Rect(8, y, cardRight, y+btRowHeight)
				if surfRow, ok := f.viewport.ClipHit(rowRect); ok {
					if hoverInside && hoverPt.In(surfRow) {
						drawSmoothRoundedRect(vpCanvas, rowRect, 6, color.RGBA{R: 0x36, G: 0x36, B: 0x36, A: 0xff}, draw.Over)
					}
					hits = append(hits, btHit{rect: surfRow, action: "toggle_device", device: d})
				}

				// Blue checkmark
				f.drawText(vpCanvas, 14, textBaseline(f.face, rowRect), "✓", color.RGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff})

				// Device icon
				devIcon := btDeviceIcon(d.Icon)
				f.drawText(vpCanvas, 28, textBaseline(f.face, rowRect), devIcon, color.RGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff})

				// Device name
				nameText := truncateToWidth(f.face, d.Name, cardRight-56)
				f.drawText(vpCanvas, 46, textBaseline(f.face, rowRect), nameText, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})

				y += btRowHeight
			}
			y += 4
			drawDivider(vpCanvas, 14, y, width-14)
			y += 6
		}

		if len(otherDevs) > 0 {
			lblRect := image.Rect(14, y, width-14, y+18)
			f.drawText(vpCanvas, 14, textBaseline(f.face, lblRect), "Devices", color.RGBA{R: 0x77, G: 0x77, B: 0x77, A: 0xff})
			y += 18

			for _, d := range otherDevs {
				rowRect := image.Rect(8, y, cardRight, y+btRowHeight)
				rowHovered := false
				if surfRow, ok := f.viewport.ClipHit(rowRect); ok {
					rowHovered = hoverInside && hoverPt.In(surfRow)
					if rowHovered {
						drawSmoothRoundedRect(vpCanvas, rowRect, 6, color.RGBA{R: 0x36, G: 0x36, B: 0x36, A: 0xff}, draw.Over)
					}
					hits = append(hits, btHit{rect: surfRow, action: "toggle_device", device: d})
				}

				devIcon := btDeviceIcon(d.Icon)
				f.drawText(vpCanvas, 14, textBaseline(f.face, rowRect), devIcon, color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff})

				nameText := truncateToWidth(f.face, d.Name, cardRight-42)
				nameCol := color.RGBA{R: 0xdd, G: 0xdd, B: 0xdd, A: 0xff}
				if rowHovered {
					nameCol = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
				}
				f.drawText(vpCanvas, 32, textBaseline(f.face, rowRect), nameText, nameCol)

				y += btRowHeight
			}
			y += 4
			drawDivider(vpCanvas, 14, y, width-14)
			y += 6
		} else if len(connectedDevs) == 0 {
			emptyRect := image.Rect(14, y, width-14, y+30)
			f.drawText(vpCanvas, 14, textBaseline(f.face, emptyRect), "No devices found", color.RGBA{R: 0x77, G: 0x77, B: 0x77, A: 0xff})
			y += 34
			drawDivider(vpCanvas, 14, y, width-14)
			y += 6
		}

		// Bottom Row: "Bluetooth Settings..."
		settingsRect := image.Rect(8, y, cardRight, y+btRowHeight)
		settingsHovered := false
		if surfSettings, ok := f.viewport.ClipHit(settingsRect); ok {
			settingsHovered = hoverInside && hoverPt.In(surfSettings)
			if settingsHovered {
				drawSmoothRoundedRect(vpCanvas, settingsRect, 6, color.RGBA{R: 0x36, G: 0x36, B: 0x36, A: 0xff}, draw.Over)
			}
			hits = append(hits, btHit{rect: surfSettings, action: "settings"})
		}
		settingsCol := color.RGBA{R: 0xcc, G: 0xcc, B: 0xcc, A: 0xff}
		if settingsHovered {
			settingsCol = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		}
		f.drawText(vpCanvas, 14, textBaseline(f.face, settingsRect), "Bluetooth Settings...", settingsCol)
	})

	writeARGB8888(pixels, canvas)

	f.mu.Lock()
	f.hits = hits
	f.mu.Unlock()
}

func btDeviceIcon(icon string) string {
	switch {
	case strings.Contains(icon, "mouse"):
		return "󰍽"
	case strings.Contains(icon, "keyboard"):
		return "󰌌"
	case strings.Contains(icon, "headset") || strings.Contains(icon, "headphones") || strings.Contains(icon, "audio"):
		return "󰋋"
	case strings.Contains(icon, "gamepad") || strings.Contains(icon, "gaming"):
		return "󰊴"
	case strings.Contains(icon, "phone"):
		return "󰏲"
	default:
		return "󰂯"
	}
}

func (f *BluetoothFlyout) drawText(dst draw.Image, x, y int, text string, col color.Color) {
	drawer := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(col),
		Face: f.face,
		Dot:  fixed.P(x, y),
	}
	drawer.DrawString(text)
}

func (f *BluetoothFlyout) updateCursorShapeLocked(cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	if cursor == nil || !f.hover.inside || f.hover.serial == 0 {
		return
	}
	pt := image.Pt(f.hover.x, f.hover.y)
	clickable := f.viewport.IsScrollbarHovered(f.hover.inside, pt)
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

func (f *BluetoothFlyout) HandlePointerEnter(ev client.WlPointerEnterEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
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

func (f *BluetoothFlyout) HandlePointerLeave(ev client.WlPointerLeaveEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
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
	f.viewport.HandleButtonRelease()
	f.mu.Unlock()
	if cursor != nil {
		_ = cursor.SetShape(ev.Serial, cursor_shape_v1.WpCursorShapeDeviceV1ShapeDefault)
	}
	f.markDirty()
}

func (f *BluetoothFlyout) HandlePointerMotion(ev client.WlPointerMotionEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	f.mu.Lock()
	if !f.isOpen || !f.hover.inside {
		f.mu.Unlock()
		return
	}
	f.hover.x = int(ev.SurfaceX)
	f.hover.y = int(ev.SurfaceY)
	_ = f.viewport.HandleMotion(f.hover.y)
	f.updateCursorShapeLocked(cursor)
	f.mu.Unlock()
	f.markDirty()
}

func (f *BluetoothFlyout) ScrollBy(delta float64) {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return
	}
	changed := f.viewport.ScrollBy(delta)
	f.mu.Unlock()

	if changed {
		f.markDirty()
	}
}

func (f *BluetoothFlyout) HandlePointerAxis(ev client.WlPointerAxisEvent) {
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

func (f *BluetoothFlyout) HandlePointerButton(ev client.WlPointerButtonEvent) bool {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return false
	}
	if f.backdropHover {
		f.mu.Unlock()
		if ev.State == client.WlPointerButtonStatePressed {
			f.Close()
		}
		return true
	}
	if !f.hover.inside {
		f.mu.Unlock()
		return false
	}

	if ev.Button == 0x110 && ev.State == client.WlPointerButtonStateReleased {
		wasDragging := f.viewport.HandleButtonRelease()
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
	if f.viewport.HandleButtonPress(pt) {
		f.mu.Unlock()
		f.markDirty()
		return true
	}

	hitsCopy := make([]btHit, len(f.hits))
	copy(hitsCopy, f.hits)
	pwr := f.powered
	f.mu.Unlock()

	for _, hit := range hitsCopy {
		if pt.In(hit.rect) {
			if hit.action == "toggle_power" {
				_ = f.bluetooth.SetPowered(!pwr)
				f.markDirty()
				return true
			}
			if hit.action == "toggle_device" && hit.device.Address != "" {
				f.bluetooth.ToggleDevice(hit.device)
				f.markDirty()
				return true
			}
			if hit.action == "settings" {
				f.Close()
				go func() {
					_ = exec.Command("ghostty", "-e", "bluetui").Start()
				}()
				return true
			}
		}
	}

	return true
}
