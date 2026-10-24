package main

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
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
	ccWidth     = 320
	ccMaxHeight = 540
	ccPad       = 10
	ccRowH      = 46
	ccCircleD   = 38
	ccGap       = 8
	ccSep       = 12 // gap(6) + divider(1) + gap(5) between segments
	ccListRow   = 26
	ccSectionH  = 16
	ccListTop   = 8 // spacing above the device list
	ccThumbR    = 10.0
	ccVolIcon   = 26
	ccVolRowH   = 38
	ccIconSize  = 15

	ccPillLabelX = 8  // gap between a pill's glyph and the name drawn inside the track
	ccBrListH    = 38 // per-monitor slider pill (same height as the volume pill)
	ccBrListGap  = 8
	ccBrListTop  = 6
	ccChevD      = 20 // chevron button diameter inside the brightness pill

	// ccBrightnessGlyph is the brightness (sun) glyph on every brightness slider.
	ccBrightnessGlyph = "\uf185"
)

var (
	ccTextCol   = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	ccDimCol    = color.RGBA{R: 0x99, G: 0x99, B: 0x9c, A: 0xff}
	ccAccentCol = color.RGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff}
	ccRowHover  = color.RGBA{R: 0x2e, G: 0x2e, B: 0x32, A: 0xff}
	ccChevWell  = color.RGBA{R: 0x1e, G: 0x1e, B: 0x20, A: 0x66}
	ccChevGlyph = color.RGBA{R: 0xd8, G: 0xd8, B: 0xda, A: 0xff}
)

type ccHit struct {
	rect   image.Rectangle
	action string
	device BluetoothDevice
}

// ccSlider is a draggable slider: either the volume pill or one monitor's
// brightness pill. x0..x1 is the coordinate span mapped to 0..100%.
type ccSlider struct {
	rect       image.Rectangle
	x0, x1     int
	volume     bool
	displayKey string
}

// ControlCenterFlyout is the merged mic/volume/bluetooth dropdown.
type ControlCenterFlyout struct {
	disp      *display.Display
	output    *display.Output
	bluetooth *BluetoothService
	audio     *AudioService
	bright    *BrightnessService
	face      font.Face
	iconFace  font.Face

	mu       sync.Mutex
	isOpen   bool
	barWidth int

	surface *surface.Surface
	layer   *shell.LayerSurface
	frame   *shm.Frame

	backdropSurf  *surface.Surface
	backdropLayer *shell.LayerSurface
	backdropFrame *shm.Frame
	backdropHover bool
	closeSubs     []func()

	hover flyoutPointerState

	powered    bool
	devices    []BluetoothDevice
	audioState AudioState
	displays   []BrightnessDisplay

	btExpanded bool
	brExpanded bool

	btToggle  float64
	btTarget  float64
	micToggle float64
	micTarget float64
	volHover  float64
	volTarget float64
	brHover   float64
	brTarget  float64
	brHoverK  string

	dragging   bool
	dragSlider ccSlider

	hits    []ccHit
	sliders []ccSlider

	currentHeight int
	viewport      ScrollViewport
	startTime     int64
	dirtyCh       chan struct{}
	stop          chan struct{}
}

func NewControlCenterFlyout(disp *display.Display, output *display.Output, bluetooth *BluetoothService, audio *AudioService, bright *BrightnessService) *ControlCenterFlyout {
	return &ControlCenterFlyout{
		disp:      disp,
		output:    output,
		bluetooth: bluetooth,
		audio:     audio,
		bright:    bright,
		face:      loadBarFont(output),
		iconFace:  loadBarFontSize(output, ccIconSize),
		startTime: time.Now().UnixMilli(),
	}
}

func (f *ControlCenterFlyout) SetBarWidth(w int) {
	f.mu.Lock()
	f.barWidth = w
	f.mu.Unlock()
}

func (f *ControlCenterFlyout) IsOpen() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.isOpen
}

func (f *ControlCenterFlyout) Destroy() {
	f.Close()
}

func (f *ControlCenterFlyout) Toggle(moduleRect image.Rectangle) {
	f.mu.Lock()
	open := f.isOpen
	f.mu.Unlock()
	if open {
		f.Close()
	} else {
		f.Open(moduleRect)
	}
}

func (f *ControlCenterFlyout) Open(moduleRect image.Rectangle) {
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
		Name:          "cheese-cc-backdrop",
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
		log.Printf("ControlCenterFlyout: Failed to create surface: %v", err)
		return
	}

	centerX := (moduleRect.Min.X + moduleRect.Max.X) / 2
	leftMargin := int32(centerX - ccWidth/2)
	if f.barWidth > 0 {
		maxLeft := int32(f.barWidth - ccWidth - barSidePad)
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
	audioState := f.audio.State()
	displays := f.bright.Displays()
	_, calcHeight := f.computeHeights(devices, f.btExpanded, displays, f.brExpanded)

	layer, err := shell.NewLayer(surf, f.disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionOverlay,
		Name:          "cheese-control-center",
		Anchor:        shell.AnchorTop | shell.AnchorLeft,
		Width:         ccWidth,
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
		log.Printf("ControlCenterFlyout: Failed to create layer surface: %v", err)
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
	f.powered = powered
	f.devices = devices
	f.audioState = audioState
	f.displays = f.orderDisplays(displays)
	f.brExpanded = false
	f.btToggle = boolTo01(powered)
	f.btTarget = f.btToggle
	f.micToggle = boolTo01(!audioState.SourceMuted)
	f.micTarget = f.micToggle
	f.volHover = 0
	f.volTarget = 0
	f.brHover = 0
	f.brTarget = 0
	f.brHoverK = ""
	f.dragging = false
	f.dragSlider = ccSlider{}
	f.currentHeight = calcHeight
	f.viewport.ScrollOffset = 0
	f.dirtyCh = make(chan struct{}, 1)
	f.stop = make(chan struct{})

	// Device polling runs on its own goroutine so a slow scan never blocks rendering.
	load := func() {
		devs, pwr := f.bluetooth.ListDevices()
		f.mu.Lock()
		f.devices = devs
		if pwr != f.powered {
			f.powered = pwr
			f.btTarget = boolTo01(pwr)
		}
		f.mu.Unlock()
		f.markDirty()
	}

	frame.SetRender(func(width, height int, frameTime uint32, pixels []byte) {
		f.draw(width, height, pixels)
	})
	frame.SetManualMode(true)

	layer.OnClose(func() { f.Close() })
	bdLayer.OnClose(func() { f.Close() })

	stopCh := f.stop
	dirtyCh := f.dirtyCh
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		renderTicker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		defer renderTicker.Stop()
		pending := true
		bdMapped := false

		go load()

		for {
			select {
			case <-dirtyCh:
				pending = true
			case <-renderTicker.C:
			case <-ticker.C:
				go load()
			case <-stopCh:
				return
			}

			f.mu.Lock()
			animating := f.stepAnimationsLocked()
			desired := f.surfaceHeightLocked()
			ly, sf, fr := f.layer, f.surface, f.frame
			bdf := f.backdropFrame
			curH := f.currentHeight
			if ly != nil && sf != nil && desired != curH {
				f.currentHeight = desired
				f.mu.Unlock()
				_ = ly.SetSize(ccWidth, uint32(desired))
				_ = sf.Commit()
				pending = true
				continue
			}
			f.mu.Unlock()

			if !bdMapped && bdf != nil && bdf.Ready() {
				bdf.ManualRender(0)
				bdMapped = true
			}

			if animating {
				pending = true
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

	closeSub := f.audio.Subscribe(func(state AudioState) {
		f.mu.Lock()
		if state.SourceMuted != f.audioState.SourceMuted {
			f.micTarget = boolTo01(!state.SourceMuted)
		}
		f.audioState = state
		f.mu.Unlock()
		f.markDirty()
	})
	brSub := f.bright.Subscribe(func(list []BrightnessDisplay) {
		f.mu.Lock()
		f.displays = f.orderDisplays(list)
		f.mu.Unlock()
		f.markDirty()
	})
	f.mu.Lock()
	if f.isOpen {
		f.closeSubs = []func(){closeSub, brSub}
		f.mu.Unlock()
	} else {
		f.mu.Unlock()
		closeSub()
		brSub()
	}

	f.bluetooth.StartDiscovery()
	f.markDirty()
}

// orderDisplays put the monitor backing this bar's output first: it owns the
// main (labelled) brightness pill, every other display is behind the chevron.
func (f *ControlCenterFlyout) orderDisplays(list []BrightnessDisplay) []BrightnessDisplay {
	if f.output == nil || len(list) < 2 {
		return list
	}
	name := f.output.Name
	for i, d := range list {
		if displayMatchesOutput(d, name) {
			if i == 0 {
				return list
			}
			out := make([]BrightnessDisplay, 0, len(list))
			out = append(out, d)
			out = append(out, list[:i]...)
			out = append(out, list[i+1:]...)
			return out
		}
	}
	return list
}

// displayMatchesOutput reports whether a brightness target belongs to a niri
// output. The built-in panel's backlight device name is not the connector name,
// so internal displays match any eDP output.
func displayMatchesOutput(d BrightnessDisplay, outputName string) bool {
	if outputName == "" {
		return false
	}
	if d.Key == outputName {
		return true
	}
	return d.Internal && strings.HasPrefix(outputName, "eDP")
}

func ccListHeight(devices []BluetoothDevice) int {
	connected, other := 0, 0
	for _, d := range devices {
		if d.Connected {
			connected++
		} else {
			other++
		}
	}
	h := ccListTop
	if connected > 0 {
		h += ccSectionH + connected*ccListRow + 4 + 1 + 6
	}
	if other > 0 {
		h += ccSectionH + other*ccListRow + 4 + 1 + 6
	} else if connected == 0 {
		h += 26 + 4 + 1 + 6
	}
	h += ccListRow
	return h
}

// ccDisplayListHeight is the height of the expanded per-monitor list. The first
// display is not listed: it already owns the main brightness pill.
func ccDisplayListHeight(displays []BrightnessDisplay) int {
	rest := ccOtherDisplays(displays)
	if len(rest) == 0 {
		return 0
	}
	h := ccBrListTop
	for i := range rest {
		h += ccBrListH
		if i < len(rest)-1 {
			h += ccBrListGap
		}
	}
	return h + 2
}

// ccOtherDisplays returns every display except the primary one, which owns the
// main brightness pill.
func ccOtherDisplays(displays []BrightnessDisplay) []BrightnessDisplay {
	if len(displays) < 2 {
		return nil
	}
	return displays[1:]
}

func (f *ControlCenterFlyout) computeHeights(devices []BluetoothDevice, expanded bool, displays []BrightnessDisplay, brExpanded bool) (content, surface int) {
	h := ccPad
	h += ccRowH
	if expanded {
		h += ccListHeight(devices)
	}
	h += ccGap
	h += ccVolRowH
	h += ccGap
	h += ccVolRowH // brightness pill (name drawn inside the track)
	if brExpanded {
		h += ccDisplayListHeight(displays)
	}
	h += ccGap
	h += ccRowH
	h += ccPad
	content = h
	surface = h
	if surface > ccMaxHeight {
		surface = ccMaxHeight
	}
	return content, surface
}

func (f *ControlCenterFlyout) surfaceHeightLocked() int {
	_, surface := f.computeHeights(f.devices, f.btExpanded, f.displays, f.brExpanded)
	return surface
}

func (f *ControlCenterFlyout) stepAnimationsLocked() bool {
	animating := false
	step := func(p *float64, target, rate float64) {
		d := target - *p
		if math.Abs(d) < 0.01 {
			if *p != target {
				*p = target
				animating = true
			}
			return
		}
		*p += d * rate
		animating = true
	}
	step(&f.btToggle, f.btTarget, 0.22)
	step(&f.micToggle, f.micTarget, 0.22)
	step(&f.volHover, f.volTarget, 0.15)
	step(&f.brHover, f.brTarget, 0.15)
	return animating
}

func (f *ControlCenterFlyout) Close() {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return
	}
	f.isOpen = false
	closeSubs := f.closeSubs
	f.closeSubs = nil
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
	f.dragging = false
	f.hover = flyoutPointerState{}
	f.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}
	for _, unsub := range closeSubs {
		if unsub != nil {
			unsub()
		}
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

	f.bluetooth.StopDiscovery()
}

func (f *ControlCenterFlyout) markDirty() {
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

func (f *ControlCenterFlyout) draw(width, height int, pixels []byte) {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Rect, image.NewUniform(color.RGBA{}), image.Point{}, draw.Src)
	drawSmoothRoundedRect(canvas, image.Rect(0, 0, width, height), float64(moduleCornerRadius), color.RGBA{R: 0x18, G: 0x18, B: 0x1a, A: 0xff}, draw.Src)

	f.mu.Lock()
	hoverInside := f.hover.inside
	hoverPt := image.Pt(f.hover.x, f.hover.y)
	powered := f.powered
	devices := f.devices
	ast := f.audioState
	expanded := f.btExpanded
	btAnim := f.btToggle
	micAnim := f.micToggle
	volHover := f.volHover
	displays := f.displays
	brExpanded := f.brExpanded
	brHover := f.brHover
	brHoverK := f.brHoverK
	f.mu.Unlock()

	hits := make([]ccHit, 0, 32)
	sliders := make([]ccSlider, 0, 4)

	contentH, _ := f.computeHeights(devices, expanded, displays, brExpanded)
	f.mu.Lock()
	f.viewport.Setup(0, height, contentH, width)
	f.mu.Unlock()

	hovered := func(rect image.Rectangle) bool { return hoverInside && hoverPt.In(rect) }

	f.viewport.Render(canvas, width, hoverInside, hoverPt, func(vp *image.RGBA, scrollY int) {
		y := ccPad - scrollY

		// ================= Bluetooth segment =================
		btRow := image.Rect(ccPad, y, width-ccPad, y+ccRowH)
		btCy := btRow.Min.Y + ccRowH/2
		btCx := ccPad + ccCircleD/2
		btCircle := image.Rect(btCx-ccCircleD/2, btCy-ccCircleD/2, btCx+ccCircleD/2, btCy+ccCircleD/2)
		drawCircleButton(vp, btCx, btCy, float64(ccCircleD)/2, btAnim, hovered(btCircle))
		btIcon, btIconCol := "󰂲", ccDimCol
		if powered {
			btIcon, btIconCol = "󰂯", ccTextCol
		}
		f.drawCentered(vp, btCx, btCy, btIcon, btIconCol)
		hits = append(hits, ccHit{rect: btCircle, action: "bt_power"})

		lineY := textBaseline(f.face, btRow)
		f.drawText(vp, btCircle.Max.X+10, lineY, "Bluetooth", ccTextCol)
		drawChevron(vp, btRow.Max.X-ccPad-6, btRow.Min.Y+ccRowH/2, 5, 3, !expanded, ccDimCol)
		hits = append(hits, ccHit{rect: image.Rect(btCircle.Max.X, btRow.Min.Y, btRow.Max.X, btRow.Max.Y), action: "bt_expand"})
		y += ccRowH

		if expanded {
			y = f.drawBtList(vp, width, y, devices, hovered, &hits)
		}
		y += ccGap

		// ================= Volume: big pill with the speaker button inside =================
		pill := image.Rect(ccPad, y, width-ccPad, y+ccVolRowH)
		vol := ast.SinkVolume
		if vol < 0 {
			vol = 0
		}
		if vol > 100 {
			vol = 100
		}
		volIconCircle := f.drawPillSlider(vp, pill, vol, volHover, volumeIcon(vol, ast.SinkMuted), ast.SinkMuted, hoverPt, hoverInside, true)
		hits = append(hits, ccHit{rect: volIconCircle, action: "vol_mute"})
		sliders = append(sliders, ccSlider{
			rect:   image.Rect(volIconCircle.Max.X, pill.Min.Y, pill.Max.X, pill.Max.Y),
			x0:     pill.Min.X,
			x1:     pill.Max.X,
			volume: true,
		})

		y += ccVolRowH
		y += ccGap

		// ================= Brightness: labelled pill + chevron -> other monitors =================
		// The primary display owns this pill (its label sits above it); the chevron
		// lists every *other* monitor, one identical pill slider each.
		brPct, brKey := 0, ""
		brLabel := "Brightness"
		brAvailable := true
		if len(displays) > 0 {
			brPct = displays[0].Percent
			brKey = displays[0].Key
			brLabel = displays[0].Label
			brAvailable = displays[0].Available
		}
		rest := ccOtherDisplays(displays)

		brPill := image.Rect(ccPad, y, width-ccPad, y+ccVolRowH)
		brRowHover := 0.0
		if brKey != "" && brKey == brHoverK {
			brRowHover = brHover
		}
		// Labels react immediately to the pointer (no fade lag) and stay visible
		// for every slider while the monitor list is expanded.
		brLabelA := 0.0
		if brExpanded || hovered(brPill) {
			brLabelA = 1
		}
		brIconCircle := f.drawPillSlider(vp, brPill, brPct, brRowHover, ccBrightnessGlyph, !brAvailable, hoverPt, hoverInside, false)

		chevCx := brPill.Max.X - 7 - ccChevD/2
		chevCy := brPill.Min.Y + ccVolRowH/2
		chevRect := image.Rect(chevCx-ccChevD/2, chevCy-ccChevD/2, chevCx+ccChevD/2, chevCy+ccChevD/2)
		sliderEndX := brPill.Max.X
		if len(rest) > 0 {
			// Fixed dark well + light chevron: constant contrast whether the fill
			// reaches the pill's right end or not (no colour flip as it moves).
			drawSmoothCircle(vp, float64(chevCx), float64(chevCy), float64(ccChevD)/2, ccChevWell)
			if hovered(chevRect) {
				drawSmoothCircle(vp, float64(chevCx), float64(chevCy), float64(ccChevD)/2, color.RGBA{R: 0x16, G: 0x16, B: 0x18, A: 0x16})
			}
			drawChevron(vp, chevCx, chevCy, 4, 2, !brExpanded, ccChevGlyph)
			hits = append(hits, ccHit{rect: chevRect, action: "br_expand"})
			sliderEndX = chevRect.Min.X - 2
		}

		// The glyph is not a button, so the whole track drags (chevron aside).
		f.drawPillLabel(vp, brPill, brIconCircle, brLabel, brPill.Max.X-sliderEndX+8, !brAvailable, brLabelA)
		if brKey != "" {
			sliders = append(sliders, ccSlider{
				rect:       image.Rect(brPill.Min.X, brPill.Min.Y, sliderEndX, brPill.Max.Y),
				x0:         brPill.Min.X,
				x1:         brPill.Max.X,
				displayKey: brKey,
			})
		}
		y += ccVolRowH

		if brExpanded {
			y += ccBrListTop
			for i, d := range rest {
				rowPill := image.Rect(ccPad, y, width-ccPad, y+ccBrListH)
				rowHover := 0.0
				if d.Key == brHoverK {
					rowHover = brHover
				}
				rowIcon := f.drawPillSlider(vp, rowPill, d.Percent, rowHover, ccBrightnessGlyph, !d.Available, hoverPt, hoverInside, false)
				rowLabelA := 0.0
				if brExpanded || hovered(rowPill) {
					rowLabelA = 1
				}
				f.drawPillLabel(vp, rowPill, rowIcon, d.Label, 8, !d.Available, rowLabelA)
				sliders = append(sliders, ccSlider{
					rect:       rowPill,
					x0:         rowPill.Min.X,
					x1:         rowPill.Max.X,
					displayKey: d.Key,
				})
				y += ccBrListH
				if i < len(rest)-1 {
					y += ccBrListGap
				}
			}
			y += 2
		}
		y += ccGap

		// ================= Microphone segment =================
		micRow := image.Rect(ccPad, y, width-ccPad, y+ccRowH)
		micCy := micRow.Min.Y + ccRowH/2
		micCx := ccPad + ccCircleD/2
		micCircle := image.Rect(micCx-ccCircleD/2, micCy-ccCircleD/2, micCx+ccCircleD/2, micCy+ccCircleD/2)
		drawCircleButton(vp, micCx, micCy, float64(ccCircleD)/2, micAnim, hovered(micCircle))
		micIcon, micIconCol := "󰍬", ccTextCol
		if ast.SourceMuted {
			micIcon, micIconCol = "󰍭", ccDimCol
		}
		f.drawCentered(vp, micCx, micCy, micIcon, micIconCol)
		hits = append(hits, ccHit{rect: micCircle, action: "mic_toggle"})
		y += ccRowH
	})

	writeARGB8888(pixels, canvas)

	brHoverKey := ""
	volHovered := false
	if hoverInside {
		for _, s := range sliders {
			if !hoverPt.In(s.rect) {
				continue
			}
			if s.volume {
				volHovered = true
			} else if brHoverKey == "" {
				brHoverKey = s.displayKey
			}
		}
	}

	f.mu.Lock()
	f.hits = hits
	f.sliders = sliders
	if brHoverKey != f.brHoverK {
		f.brHoverK = brHoverKey
		f.brHover = 0
	}
	f.brTarget = boolTo01(brHoverKey != "")
	f.volTarget = boolTo01(volHovered)
	f.mu.Unlock()
}

// drawBtList draws the expanded bluetooth device list and returns the new y.
func (f *ControlCenterFlyout) drawBtList(vp *image.RGBA, width, y int, devices []BluetoothDevice, hovered func(image.Rectangle) bool, hits *[]ccHit) int {
	var connected, other []BluetoothDevice
	for _, d := range devices {
		if d.Connected {
			connected = append(connected, d)
		} else {
			other = append(other, d)
		}
	}
	rowRight := width - ccPad
	y += ccListTop

	if len(connected) > 0 {
		lbl := image.Rect(ccPad+4, y, rowRight, y+ccSectionH)
		f.drawText(vp, lbl.Min.X, textBaseline(f.face, lbl), "Connected Devices", ccDimCol)
		y += ccSectionH
		for _, d := range connected {
			row := image.Rect(ccPad, y, rowRight, y+ccListRow)
			if hovered(row) {
				drawSmoothRoundedRect(vp, row, 6, ccRowHover, draw.Over)
			}
			f.drawIconText(vp, row.Min.X+12, textBaseline(f.iconFace, row), btDeviceIcon(d.Icon), ccAccentCol)
			f.drawText(vp, row.Min.X+38, textBaseline(f.face, row), truncateToWidth(f.face, d.Name, row.Dx()-84), ccTextCol)
			check := "✓"
			cw := font.MeasureString(f.face, check).Ceil()
			f.drawText(vp, row.Max.X-14-cw, textBaseline(f.face, row), check, ccAccentCol)
			*hits = append(*hits, ccHit{rect: row, action: "bt_device", device: d})
			y += ccListRow
		}
		y += 4
		drawDivider(vp, ccPad, y, width-ccPad)
		y += 6
	}

	if len(other) > 0 {
		lbl := image.Rect(ccPad+4, y, rowRight, y+ccSectionH)
		f.drawText(vp, lbl.Min.X, textBaseline(f.face, lbl), "Devices", ccDimCol)
		y += ccSectionH
		for _, d := range other {
			row := image.Rect(ccPad, y, rowRight, y+ccListRow)
			rh := hovered(row)
			if rh {
				drawSmoothRoundedRect(vp, row, 6, ccRowHover, draw.Over)
			}
			iconCol := color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
			if d.Paired {
				iconCol = color.RGBA{R: 0xbb, G: 0xbb, B: 0xbf, A: 0xff}
			}
			f.drawIconText(vp, row.Min.X+12, textBaseline(f.iconFace, row), btDeviceIcon(d.Icon), iconCol)
			col := color.RGBA{R: 0xdd, G: 0xdd, B: 0xdd, A: 0xff}
			if rh {
				col = ccTextCol
			}
			f.drawText(vp, row.Min.X+38, textBaseline(f.face, row), truncateToWidth(f.face, d.Name, row.Dx()-56), col)
			*hits = append(*hits, ccHit{rect: row, action: "bt_device", device: d})
			y += ccListRow
		}
		y += 4
		drawDivider(vp, ccPad, y, width-ccPad)
		y += 6
	} else if len(connected) == 0 {
		empty := image.Rect(ccPad+4, y, rowRight, y+26)
		f.drawText(vp, empty.Min.X, textBaseline(f.face, empty), "No devices found", ccDimCol)
		y += 26
		drawDivider(vp, ccPad, y, width-ccPad)
		y += 6
	}

	settings := image.Rect(ccPad, y, rowRight, y+ccListRow)
	sh := hovered(settings)
	if sh {
		drawSmoothRoundedRect(vp, settings, 6, ccRowHover, draw.Over)
	}
	scol := color.RGBA{R: 0xcc, G: 0xcc, B: 0xcc, A: 0xff}
	if sh {
		scol = ccTextCol
	}
	f.drawText(vp, settings.Min.X+12, textBaseline(f.face, settings), "Bluetooth Settings...", scol)
	*hits = append(*hits, ccHit{rect: settings, action: "bt_settings"})
	y += ccListRow
	return y
}

func (f *ControlCenterFlyout) drawText(dst draw.Image, x, y int, text string, col color.Color) {
	f.drawTextFace(dst, f.face, x, y, text, col)
}

func (f *ControlCenterFlyout) drawIconText(dst draw.Image, x, y int, text string, col color.Color) {
	f.drawTextFace(dst, f.iconFace, x, y, text, col)
}

func (f *ControlCenterFlyout) drawTextFace(dst draw.Image, face font.Face, x, y int, text string, col color.Color) {
	drawer := font.Drawer{Dst: dst, Src: image.NewUniform(col), Face: face, Dot: fixed.P(x, y)}
	drawer.DrawString(text)
}

func (f *ControlCenterFlyout) drawCentered(dst draw.Image, cx, cy int, text string, col color.Color) {
	bounds, _ := font.BoundString(f.iconFace, text)
	w := (bounds.Max.X - bounds.Min.X).Ceil()
	x := cx - w/2 - bounds.Min.X.Floor()
	m := f.iconFace.Metrics()
	drawer := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(col),
		Face: f.iconFace,
		Dot:  fixed.P(x, cy+(m.Ascent.Ceil()-m.Descent.Ceil())/2),
	}
	drawer.DrawString(text)
}

// SimulateHover drives the pointer position from the command socket so a flyout
// state can be captured/inspected without a real pointer.
func (f *ControlCenterFlyout) SimulateHover(x, y int) {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return
	}
	f.hover.inside = true
	f.hover.x = x
	f.hover.y = y
	f.mu.Unlock()
	f.markDirty()
}

func (f *ControlCenterFlyout) setBrightnessExpanded(expanded bool) {
	f.mu.Lock()
	if !f.isOpen {
		f.mu.Unlock()
		return
	}
	f.brExpanded = expanded
	f.brHover = 0
	f.brHoverK = ""
	f.mu.Unlock()
	f.markDirty()
}

func (f *ControlCenterFlyout) updateCursorShapeLocked(cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	if cursor == nil || !f.hover.inside || f.hover.serial == 0 {
		return
	}
	pt := image.Pt(f.hover.x, f.hover.y)
	clickable := f.dragging || f.viewport.IsScrollbarHovered(f.hover.inside, pt)
	if !clickable {
		for _, s := range f.sliders {
			if pt.In(s.rect) {
				clickable = true
				break
			}
		}
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

func (f *ControlCenterFlyout) HandlePointerEnter(ev client.WlPointerEnterEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
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

func (f *ControlCenterFlyout) HandlePointerLeave(ev client.WlPointerLeaveEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
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
	f.dragging = false
	f.dragSlider = ccSlider{}
	f.viewport.HandleButtonRelease()
	f.mu.Unlock()
	if cursor != nil {
		_ = cursor.SetShape(ev.Serial, cursor_shape_v1.WpCursorShapeDeviceV1ShapeDefault)
	}
	f.markDirty()
}

func (f *ControlCenterFlyout) HandlePointerMotion(ev client.WlPointerMotionEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	f.mu.Lock()
	if !f.isOpen || !f.hover.inside {
		f.mu.Unlock()
		return
	}
	f.hover.x = int(ev.SurfaceX)
	f.hover.y = int(ev.SurfaceY)
	_ = f.viewport.HandleMotion(f.hover.y)
	dragging := f.dragging
	dragSlider := f.dragSlider
	x := f.hover.x
	f.updateCursorShapeLocked(cursor)
	f.mu.Unlock()

	if dragging {
		f.applySlider(dragSlider, x)
	}
	f.markDirty()
}

func (f *ControlCenterFlyout) ScrollBy(delta float64) {
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

func (f *ControlCenterFlyout) HandlePointerAxis(ev client.WlPointerAxisEvent) {
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

func (f *ControlCenterFlyout) HandlePointerButton(ev client.WlPointerButtonEvent) bool {
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
		f.dragSlider = ccSlider{}
		f.mu.Unlock()
		return true
	}

	if ev.Button != 0x110 || ev.State != client.WlPointerButtonStatePressed {
		f.mu.Unlock()
		return true
	}

	pt := image.Pt(f.hover.x, f.hover.y)
	slidersCopy := make([]ccSlider, len(f.sliders))
	copy(slidersCopy, f.sliders)
	for _, s := range slidersCopy {
		if !s.rect.Empty() && pt.In(s.rect) {
			f.dragging = true
			f.dragSlider = s
			f.mu.Unlock()
			f.applySlider(s, pt.X)
			return true
		}
	}

	hitsCopy := make([]ccHit, len(f.hits))
	copy(hitsCopy, f.hits)
	powered := f.powered
	srcMuted := f.audioState.SourceMuted
	f.mu.Unlock()

	for _, hit := range hitsCopy {
		if !pt.In(hit.rect) {
			continue
		}
		switch hit.action {
		case "bt_power":
			_ = f.bluetooth.SetPowered(!powered)
			f.mu.Lock()
			f.btTarget = boolTo01(!powered)
			f.mu.Unlock()
			f.markDirty()
			return true
		case "bt_expand":
			f.mu.Lock()
			f.btExpanded = !f.btExpanded
			f.mu.Unlock()
			f.markDirty()
			return true
		case "br_expand":
			f.mu.Lock()
			f.brExpanded = !f.brExpanded
			f.brHover = 0
			f.brHoverK = ""
			f.mu.Unlock()
			f.markDirty()
			return true
		case "bt_device":
			f.bluetooth.ToggleDevice(hit.device)
			f.markDirty()
			return true
		case "bt_settings":
			f.Close()
			go func() { _ = exec.Command("ghostty", "-e", "bluetui").Start() }()
			return true
		case "mic_toggle":
			_ = f.audio.ToggleSourceMute()
			f.mu.Lock()
			f.micTarget = boolTo01(srcMuted)
			f.mu.Unlock()
			f.markDirty()
			return true
		case "vol_mute":
			_ = f.audio.ToggleSinkMute()
			f.markDirty()
			return true
		}
	}
	return true
}

// drawPillSlider draws the bar's standard pill slider — subtle-white track,
// fill masked by the pill, glyph in a circle at the left, thumb-less — and
// returns the glyph's circle so callers can attach a hit target to it or
// exclude it from the draggable span.
func (f *ControlCenterFlyout) drawPillSlider(dst *image.RGBA, pill image.Rectangle, pct int, hover float64, glyph string, dim bool, hoverPt image.Point, hoverInside, iconButton bool) image.Rectangle {
	pillR := float64(pill.Dy()) / 2
	drawSmoothRoundedRect(dst, pill, pillR, whiteAlpha(0x40), draw.Over)

	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	endX := int(math.Round(float64(pill.Min.X) + float64(pct)/100.0*float64(pill.Dx())))
	// Fill: soft white at rest, brighter white on hover; dimmed when inactive.
	fillA := uint8(0xd0 + int(0x2f*hover))
	if dim {
		fillA = 0x80
	}
	if endX > pill.Min.X {
		drawFillInPill(dst, pill, pillR, endX, whiteAlpha(fillA))
	}

	iconSize := int(math.Round(float64(pill.Dy()) * 0.68))
	if iconSize < 8 {
		iconSize = 8
	}
	iconR := iconSize / 2
	iconCx := pill.Min.X + (pill.Dy()-iconSize)/2 + iconR
	iconCy := pill.Min.Y + pill.Dy()/2
	iconCircle := image.Rect(iconCx-iconR, iconCy-iconR, iconCx+iconR, iconCy+iconR)
	iconCol := color.RGBA{R: 0x24, G: 0x24, B: 0x27, A: 0xff}
	if dim {
		iconCol = color.RGBA{R: 0x55, G: 0x55, B: 0x58, A: 0xff}
	}
	// Only a clickable glyph (the speaker = mute) gets the button hover chrome;
	// an inert glyph must not look like a button.
	if iconButton && hoverInside && hoverPt.In(iconCircle) {
		drawSmoothCircle(dst, float64(iconCx), float64(iconCy), float64(iconR), color.RGBA{A: 0x22})
	}
	f.drawCentered(dst, iconCx, iconCy, glyph, iconCol)
	return iconCircle
}

// drawPillLabel draws a slider's name inside its track, right of the glyph, when
// that slider is hovered (or the list is expanded). The name keeps one fixed
// colour so it never flips as the fill passes under it; reservedRight keeps it
// clear of the chevron / pill edge.
func (f *ControlCenterFlyout) drawPillLabel(dst *image.RGBA, pill, iconCircle image.Rectangle, label string, reservedRight int, dim bool, alpha float64) {
	if label == "" || alpha <= 0.02 {
		return
	}
	x := iconCircle.Max.X + ccPillLabelX
	maxW := pill.Max.X - x - reservedRight
	if maxW < 24 {
		return
	}
	base := color.RGBA{R: 0x24, G: 0x24, B: 0x27, A: 0xff}
	if dim {
		base = color.RGBA{R: 0x55, G: 0x55, B: 0x58, A: 0xff}
	}
	a := uint8(math.Round(255 * alpha))
	// color.RGBA is alpha-premultiplied: scale RGB by the alpha.
	col := color.RGBA{
		R: uint8(uint32(base.R) * uint32(a) / 255),
		G: uint8(uint32(base.G) * uint32(a) / 255),
		B: uint8(uint32(base.B) * uint32(a) / 255),
		A: a,
	}
	box := image.Rect(x, pill.Min.Y, x+maxW, pill.Max.Y)
	f.drawText(dst, box.Min.X, textBaseline(f.face, box), truncateToWidth(f.face, label, maxW), col)
}

// applySlider maps a pointer x to 0..100% and pushes it to the slider's target.
func (f *ControlCenterFlyout) applySlider(s ccSlider, x int) {
	if s.x1 <= s.x0 {
		return
	}
	pct := int(math.Round(float64(x-s.x0) / float64(s.x1-s.x0) * 100.0))
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	if s.volume {
		_ = f.audio.SetSinkVolume(pct)
		return
	}
	if s.displayKey != "" {
		f.bright.Set(s.displayKey, pct)
	}
}

// drawFillInPill fills the pill shape only up to rightX, so the fill's right
// edge is a straight vertical cut (a rectangle) while its left follows the
// pill's rounded end.
func drawFillInPill(dst *image.RGBA, pill image.Rectangle, radius float64, rightX int, col color.RGBA) {
	if pill.Empty() || rightX <= pill.Min.X {
		return
	}
	if rightX > pill.Max.X {
		rightX = pill.Max.X
	}
	src := color.NRGBAModel.Convert(col).(color.NRGBA)
	hx := float64(pill.Dx()) / 2.0
	hy := float64(pill.Dy()) / 2.0
	r := radius
	if r > hx {
		r = hx
	}
	if r > hy {
		r = hy
	}
	if r <= 0 {
		draw.Draw(dst, image.Rect(pill.Min.X, pill.Min.Y, rightX, pill.Max.Y).Intersect(dst.Bounds()), image.NewUniform(col), image.Point{}, draw.Over)
		return
	}
	cx := float64(pill.Min.X) + hx
	cy := float64(pill.Min.Y) + hy
	innerHx := hx - r
	innerHy := hy - r
	b := dst.Bounds()
	for y := pill.Min.Y; y < pill.Max.Y; y++ {
		if y < b.Min.Y || y >= b.Max.Y {
			continue
		}
		py := float64(y) + 0.5
		qy := math.Abs(py-cy) - innerHy
		for x := pill.Min.X; x < rightX; x++ {
			if x < b.Min.X {
				continue
			}
			px := float64(x) + 0.5
			qx := math.Abs(px-cx) - innerHx
			var dist float64
			if qx <= 0 && qy <= 0 {
				dist = math.Max(qx, qy) - r
			} else {
				dist = math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) - r
			}
			coverage := 0.5 - dist
			if coverage <= 0 {
				continue
			}
			if coverage > 1 {
				coverage = 1
			}
			drawRoundedRectPixel(dst, x, y, src, coverage, draw.Over)
		}
	}
}

func drawCircleButton(dst *image.RGBA, cx, cy int, radius, t float64, hover bool) {
	off := color.RGBA{R: 0x3a, G: 0x3a, B: 0x3d, A: 0xff}
	on := color.RGBA{R: 0x0a, G: 0x84, B: 0xff, A: 0xff}
	col := lerpColor(off, on, t)
	if hover {
		col = lerpColor(col, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, 0.16)
	}
	drawSmoothCircle(dst, float64(cx), float64(cy), radius, col)
}

// drawChevron draws a small vector chevron (cleaner than a glyph at this size).
func drawChevron(dst *image.RGBA, cx, cy, halfW, halfH int, down bool, col color.RGBA) {
	stroke := func(x0, y0, x1, y1 int) {
		steps := halfW * 2
		if steps < 1 {
			steps = 1
		}
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps)
			x := float64(x0) + (float64(x1)-float64(x0))*t
			y := float64(y0) + (float64(y1)-float64(y0))*t
			drawSmoothCircle(dst, x, y, 1.0, col)
		}
	}
	top := cy - halfH
	bottom := cy + halfH
	if down {
		stroke(cx-halfW, top, cx, bottom)
		stroke(cx, bottom, cx+halfW, top)
	} else {
		stroke(cx-halfW, bottom, cx, top)
		stroke(cx, top, cx+halfW, bottom)
	}
}

func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	lerp := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t)
	}
	return color.RGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: lerp(a.A, b.A)}
}

func boolTo01(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// whiteAlpha returns a *premultiplied* white: color.RGBA is alpha-premultiplied,
// so for white at alpha a the RGB channels must equal a. Building
// color.RGBA{255,255,255,a} would be invalid and overflow when un-premultiplied
// (rendering near-black).
func whiteAlpha(a uint8) color.RGBA {
	return color.RGBA{R: a, G: a, B: a, A: a}
}
