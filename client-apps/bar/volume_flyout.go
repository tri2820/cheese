package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
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
	volFlyoutWidth  = 224
	volFlyoutHeight = 40
	volTrackMinX    = 38
	volTrackMaxX    = 166
	volTrackHeight  = 6
	volThumbRadius  = 6.5
	volMuteRectMaxX = 34
)

type VolumeFlyout struct {
	disp          *display.Display
	output        *display.Output
	audio         *AudioService
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
	closeSub      func()
	hover         flyoutPointerState
	dragging      bool
	startTime     int64
	dirtyCh       chan struct{}
	stop          chan struct{}
}

func NewVolumeFlyout(disp *display.Display, output *display.Output, audio *AudioService) *VolumeFlyout {
	return &VolumeFlyout{
		disp:      disp,
		output:    output,
		audio:     audio,
		face:      loadBarFont(output),
		startTime: time.Now().UnixMilli(),
	}
}

func (f *VolumeFlyout) SetBarWidth(w int) {
	f.mu.Lock()
	f.barWidth = w
	f.mu.Unlock()
}

func (f *VolumeFlyout) Destroy() {
	f.Close()
}

func (f *VolumeFlyout) IsOpen() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.isOpen
}

func (f *VolumeFlyout) Toggle(moduleRect image.Rectangle) {
	f.mu.Lock()
	open := f.isOpen
	f.mu.Unlock()

	if open {
		f.Close()
	} else {
		f.Open(moduleRect)
	}
}

func (f *VolumeFlyout) Open(moduleRect image.Rectangle) {
	f.mu.Lock()
	if f.isOpen {
		f.mu.Unlock()
		f.markDirty()
		return
	}

	// 1. Transparent click-outside backdrop on LayerPositionTop (below Overlay popup, above windows/wallpaper)
	bdSurf, err := surface.New(f.disp.Compositor())
	if err != nil {
		f.mu.Unlock()
		return
	}
	bdLayer, err := shell.NewLayer(bdSurf, f.disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          "cheese-volume-backdrop",
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
	bdFrame.SetRender(func(width, height int, frameTime uint32, pixels []byte) {
		// Newly allocated shm pages are already 0x00000000 (transparent ARGB8888)
	})
	bdFrame.SetManualMode(true)

	// 2. Volume slider popup on LayerPositionOverlay
	surf, err := surface.New(f.disp.Compositor())
	if err != nil {
		f.mu.Unlock()
		_ = bdFrame.Close()
		_ = bdLayer.Close()
		_ = bdSurf.Close()
		log.Printf("VolumeFlyout: Failed to create surface: %v", err)
		return
	}

	centerX := (moduleRect.Min.X + moduleRect.Max.X) / 2
	leftMargin := int32(centerX - volFlyoutWidth/2)
	if f.barWidth > 0 {
		maxLeft := int32(f.barWidth - volFlyoutWidth - barSidePad)
		if leftMargin > maxLeft {
			leftMargin = maxLeft
		}
	}
	if leftMargin < barSidePad {
		leftMargin = barSidePad
	}
	topMargin := int32(barTopMargin + barHeight + moduleGap)

	layer, err := shell.NewLayer(surf, f.disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionOverlay,
		Name:          "cheese-volume-flyout",
		Anchor:        shell.AnchorTop | shell.AnchorLeft,
		Width:         volFlyoutWidth,
		Height:        volFlyoutHeight,
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
		log.Printf("VolumeFlyout: Failed to create layer surface: %v", err)
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
	f.dragging = false
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
			fr := f.frame
			bdf := f.backdropFrame
			f.mu.Unlock()

			if !bdMapped && bdf != nil && bdf.Ready() {
				bdf.ManualRender(0)
				bdMapped = true
			}

			if !pending {
				continue
			}

			if fr != nil && fr.Ready() {
				fr.ManualRender(uint32(time.Now().UnixMilli() - f.startTime))
				pending = false
			}
		}
	}()

	f.mu.Unlock()

	closeSub := f.audio.Subscribe(func(_ AudioState) {
		f.markDirty()
	})
	f.mu.Lock()
	if f.isOpen {
		f.closeSub = closeSub
		f.mu.Unlock()
	} else {
		f.mu.Unlock()
		closeSub()
	}

	f.markDirty()
}

func (f *VolumeFlyout) markDirty() {
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

func (f *VolumeFlyout) Close() {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return
	}
	f.isOpen = false
	f.dragging = false
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
}

func (f *VolumeFlyout) draw(width, height int, pixels []byte) {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Rect, image.NewUniform(color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00}), image.Point{}, draw.Src)

	containerRect := image.Rect(0, 0, width, height)
	drawRoundedRect(canvas, containerRect, moduleCornerRadius, color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xff}, draw.Src)

	state := f.audio.State()
	vol := state.SinkVolume
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}

	// 1. Left speaker/mute button icon
	icon := volumeIcon(vol, state.SinkMuted)
	iconCol := color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	if state.SinkMuted {
		iconCol = color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
	}
	iconW := font.MeasureString(f.face, icon).Ceil()
	iconX := (volMuteRectMaxX - iconW) / 2
	if iconX < 10 {
		iconX = 10
	}
	f.drawText(canvas, iconX, textBaseline(f.face, containerRect), icon, iconCol)

	// 2. Slider track
	trackY := (height - volTrackHeight) / 2
	trackRect := image.Rect(volTrackMinX, trackY, volTrackMaxX, trackY+volTrackHeight)
	drawRoundedRect(canvas, trackRect, volTrackHeight/2, color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff}, draw.Over)

	// Active fill
	trackW := volTrackMaxX - volTrackMinX
	fillW := int(math.Round(float64(vol) / 100.0 * float64(trackW)))
	fillCol := color.RGBA{R: 0xe0, G: 0xe0, B: 0xe0, A: 0xff}
	if state.SinkMuted {
		fillCol = color.RGBA{R: 0x66, G: 0x66, B: 0x66, A: 0xff}
	}
	if fillW > 0 {
		if fillW < volTrackHeight {
			fillW = volTrackHeight
		}
		fillRect := image.Rect(volTrackMinX, trackY, volTrackMinX+fillW, trackY+volTrackHeight)
		drawRoundedRect(canvas, fillRect, volTrackHeight/2, fillCol, draw.Over)
	}

	// Anti-aliased circular thumb
	thumbCenterX := float64(volTrackMinX) + (float64(vol)/100.0)*float64(trackW)
	thumbCenterY := float64(height) / 2.0
	thumbCol := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	if state.SinkMuted {
		thumbCol = color.RGBA{R: 0x99, G: 0x99, B: 0x99, A: 0xff}
	}
	drawSmoothCircle(canvas, thumbCenterX, thumbCenterY, volThumbRadius, thumbCol)

	// 3. Right percentage text
	pctText := fmt.Sprintf("%d%%", vol)
	pctW := font.MeasureString(f.face, pctText).Ceil()
	pctX := width - 12 - pctW
	f.drawText(canvas, pctX, textBaseline(f.face, containerRect), pctText, iconCol)

	writeARGB8888(pixels, canvas)
}

func drawSmoothCircle(dst *image.RGBA, cx, cy, radius float64, col color.RGBA) {
	minX := int(math.Floor(cx - radius - 1))
	maxX := int(math.Ceil(cx + radius + 1))
	minY := int(math.Floor(cy - radius - 1))
	maxY := int(math.Ceil(cy + radius + 1))
	b := dst.Bounds()
	if minX < b.Min.X {
		minX = b.Min.X
	}
	if maxX > b.Max.X {
		maxX = b.Max.X
	}
	if minY < b.Min.Y {
		minY = b.Min.Y
	}
	if maxY > b.Max.Y {
		maxY = b.Max.Y
	}

	src := color.NRGBAModel.Convert(col).(color.NRGBA)
	const samples = 4
	invSamples := 1.0 / float64(samples*samples)
	r2 := radius * radius

	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			coverage := 0.0
			for sy := 0; sy < samples; sy++ {
				py := float64(y) + (float64(sy)+0.5)/float64(samples) - cy
				for sx := 0; sx < samples; sx++ {
					px := float64(x) + (float64(sx)+0.5)/float64(samples) - cx
					if px*px+py*py <= r2 {
						coverage += invSamples
					}
				}
			}
			if coverage <= 0 {
				continue
			}
			drawRoundedRectPixel(dst, x, y, src, coverage, draw.Over)
		}
	}
}

func (f *VolumeFlyout) drawText(dst draw.Image, x, y int, text string, col color.Color) {
	drawer := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(col),
		Face: f.face,
		Dot:  fixed.P(x, y),
	}
	drawer.DrawString(text)
}

func (f *VolumeFlyout) setVolumeFromX(x int) {
	trackW := volTrackMaxX - volTrackMinX
	if trackW <= 0 {
		return
	}
	pct := int(math.Round(float64(x-volTrackMinX) / float64(trackW) * 100.0))
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	_ = f.audio.SetSinkVolume(pct)
}

func (f *VolumeFlyout) updateCursorShapeLocked(cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	if cursor == nil || !f.hover.inside || f.hover.serial == 0 {
		return
	}
	x := f.hover.x
	clickable := f.dragging || (x >= 6 && x <= volMuteRectMaxX) || (x >= volTrackMinX-8 && x <= volTrackMaxX+8)
	shape := cursor_shape_v1.WpCursorShapeDeviceV1ShapeDefault
	if clickable {
		shape = cursor_shape_v1.WpCursorShapeDeviceV1ShapePointer
	}
	if shape != f.hover.shape {
		_ = cursor.SetShape(f.hover.serial, shape)
		f.hover.shape = shape
	}
}

func (f *VolumeFlyout) HandlePointerEnter(ev client.WlPointerEnterEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.isOpen {
		return
	}
	if f.backdropSurf != nil && ev.Surface == f.backdropSurf.WlSurface() {
		f.backdropHover = true
		if cursor != nil {
			_ = cursor.SetShape(ev.Serial, cursor_shape_v1.WpCursorShapeDeviceV1ShapeDefault)
		}
		return
	}
	if f.surface == nil || ev.Surface != f.surface.WlSurface() {
		return
	}
	f.hover.inside = true
	f.hover.x = int(ev.SurfaceX)
	f.hover.y = int(ev.SurfaceY)
	f.hover.serial = ev.Serial
	f.hover.shape = 0
	f.updateCursorShapeLocked(cursor)
}

func (f *VolumeFlyout) HandlePointerLeave(ev client.WlPointerLeaveEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.isOpen {
		return
	}
	if f.backdropSurf != nil && ev.Surface == f.backdropSurf.WlSurface() {
		f.backdropHover = false
		return
	}
	if f.surface == nil || ev.Surface != f.surface.WlSurface() {
		return
	}
	f.hover.inside = false
	f.hover.shape = 0
	f.dragging = false
	if cursor != nil {
		_ = cursor.SetShape(ev.Serial, cursor_shape_v1.WpCursorShapeDeviceV1ShapeDefault)
	}
}

func (f *VolumeFlyout) HandlePointerMotion(ev client.WlPointerMotionEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	f.mu.Lock()
	if !f.isOpen || !f.hover.inside {
		f.mu.Unlock()
		return
	}
	f.hover.x = int(ev.SurfaceX)
	f.hover.y = int(ev.SurfaceY)
	dragging := f.dragging
	x := f.hover.x
	f.updateCursorShapeLocked(cursor)
	f.mu.Unlock()

	if dragging {
		f.setVolumeFromX(x)
	}
}

func (f *VolumeFlyout) HandlePointerButton(ev client.WlPointerButtonEvent) bool {
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
		f.dragging = false
		f.mu.Unlock()
		return true
	}

	if ev.Button != 0x110 || ev.State != client.WlPointerButtonStatePressed {
		f.mu.Unlock()
		return true
	}

	x := f.hover.x
	if x < volMuteRectMaxX {
		f.mu.Unlock()
		_ = f.audio.ToggleSinkMute()
		return true
	}

	f.dragging = true
	f.mu.Unlock()
	f.setVolumeFromX(x)
	return true
}

func (f *VolumeFlyout) HandlePointerAxis(ev client.WlPointerAxisEvent) {
	if ev.Axis != client.WlPointerAxisVerticalScroll {
		return
	}
	f.mu.Lock()
	active := f.isOpen && f.hover.inside
	f.mu.Unlock()
	if !active {
		return
	}

	state := f.audio.State()
	step := -int(math.Round(ev.Value / 5.0))
	if step == 0 {
		if ev.Value > 0 {
			step = -2
		} else if ev.Value < 0 {
			step = 2
		}
	}
	_ = f.audio.SetSinkVolume(state.SinkVolume + step)
}
