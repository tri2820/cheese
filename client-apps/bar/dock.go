package main

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/shm"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	dockSurfaceHeight = 112
	dockBaseIconSize  = 36.0
	dockPeakIconSize  = 54.0 // 1.5x base (90/60 in macos-dock-animation-with-css)
	dockNeighbor1Size = 48.0 // 1.333x base (80/60)
	dockNeighbor2Size = 42.0 // 1.167x base (70/60)
	dockMaxOffsetPx   = 3.0  // 5/60 * base
	dockPadTop        = 8
	dockPadBottom     = 10
	dockShelfHeight   = int(dockBaseIconSize) + dockPadTop + dockPadBottom // 54px
	dockBottomMargin  = 6
	dockWindowGap     = 6
	dockExclusiveZone = dockBottomMargin + dockShelfHeight + dockWindowGap // 66px
	dockShelfRadius   = 12.0
	dockPadX          = 12
	dockItemPadX      = 6
	dockGroupGap      = 12
	dockSourceIconPx  = 64
)

type dockEntry struct {
	window      NiriWindow
	workspaceID int64
	icon        image.Image
	groupEnd    bool
}

type dockSlotHit struct {
	rect  image.Rectangle
	entry dockEntry
	index int
}

type Dock struct {
	disp        *display.Display
	output      *display.Output
	niri        *NiriService
	icons       *IconResolver
	face        font.Face
	surface     *surface.Surface
	layer       *shell.LayerSurface
	frame       *shm.Frame
	unsubscribe func()

	mu          sync.Mutex
	entries     []dockEntry
	curSizes    []float64
	hover       flyoutPointerState
	hoveredIdx  int
	slots       []dockSlotHit
	pillRect    image.Rectangle
	hasExclZone bool
	closed      bool
	dirtyCh     chan struct{}
	stop        chan struct{}
	startTime   int64
}

func NewDock(disp *display.Display, output *display.Output, niri *NiriService, icons *IconResolver) (*Dock, error) {
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		return nil, err
	}

	layer, err := shell.NewLayer(surf, disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          "cheese-dock",
		Anchor:        shell.AnchorBottom | shell.AnchorLeft | shell.AnchorRight,
		Width:         0,
		Height:        dockSurfaceHeight,
		ExclusiveZone: int32(dockExclusiveZone),
		Output:        output.WlOutput(),
	})
	if err != nil {
		_ = surf.Close()
		return nil, err
	}

	frame, err := shm.NewFrame(disp.Shm(), layer, disp, shm.FrameConfig{
		Format:  client.WlShmFormatArgb8888,
		Buffers: 2,
	})
	if err != nil {
		_ = layer.Close()
		_ = surf.Close()
		return nil, err
	}

	d := &Dock{
		disp:        disp,
		output:      output,
		niri:        niri,
		icons:       icons,
		face:        loadBarFont(output),
		surface:     surf,
		layer:       layer,
		frame:       frame,
		hoveredIdx:  -1,
		hasExclZone: true,
		dirtyCh:     make(chan struct{}, 1),
		stop:        make(chan struct{}),
		startTime:   time.Now().UnixMilli(),
	}

	frame.SetRender(func(width, height int, frameTime uint32, pixels []byte) {
		d.draw(width, height, pixels)
	})
	frame.SetManualMode(true)

	layer.OnClose(func() {
		d.Close()
	})

	d.unsubscribe = niri.Subscribe(func(state NiriState) {
		d.updateEntries(state)
	})

	go d.renderLoop()
	d.markDirty()
	return d, nil
}

func (d *Dock) updateEntries(state NiriState) {
	groups := buildTaskbarGroups(state, d.output.Name, d.icons)
	next := make([]dockEntry, 0, 12)
	for gi, g := range groups {
		for ii, item := range g.items {
			hiResIcon := d.icons.ResolveAppIcon(item.window.AppID, dockSourceIconPx)
			if hiResIcon == nil {
				hiResIcon = item.icon
			}
			next = append(next, dockEntry{
				window:      item.window,
				workspaceID: g.workspaceID,
				icon:        hiResIcon,
				groupEnd:    gi < len(groups)-1 && ii == len(g.items)-1,
			})
		}
	}

	d.mu.Lock()
	oldSizes := make(map[int64]float64, len(d.entries))
	for i, e := range d.entries {
		if i < len(d.curSizes) {
			oldSizes[e.window.ID] = d.curSizes[i]
		}
	}
	newSizes := make([]float64, len(next))
	for i, e := range next {
		if sz, ok := oldSizes[e.window.ID]; ok && sz > 0 {
			newSizes[i] = sz
		} else {
			newSizes[i] = dockBaseIconSize
		}
	}
	d.entries = next
	d.curSizes = newSizes
	d.mu.Unlock()

	d.markDirty()
}

func (d *Dock) markDirty() {
	select {
	case d.dirtyCh <- struct{}{}:
	default:
	}
}

func (d *Dock) renderLoop() {
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()
	pending := true

	for {
		select {
		case <-d.dirtyCh:
			pending = true
		case <-ticker.C:
		case <-d.stop:
			return
		}

		animating := d.stepAnimation()
		if !pending && !animating {
			continue
		}

		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			return
		}
		hasItems := len(d.entries) > 0
		exclChanged := hasItems != d.hasExclZone
		d.hasExclZone = hasItems
		fr := d.frame
		ly := d.layer
		sf := d.surface
		d.mu.Unlock()

		if exclChanged && ly != nil {
			if hasItems {
				_ = ly.SetExclusiveZone(int32(dockExclusiveZone))
			} else {
				_ = ly.SetExclusiveZone(0)
			}
			if sf != nil {
				_ = sf.Commit()
			}
		}

		if fr != nil && fr.Ready() {
			fr.ManualRender(uint32(time.Now().UnixMilli() - d.startTime))
			pending = animating
		}
	}
}

// stepAnimation computes target sizes using the macOS dock magnification formula
// from frontendfyi/macos-dock-animation-with-css and interpolates curSizes smoothly.
func (d *Dock) stepAnimation() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	n := len(d.entries)
	if n == 0 {
		d.hoveredIdx = -1
		return false
	}
	if len(d.curSizes) != n {
		d.curSizes = make([]float64, n)
		for i := range d.curSizes {
			d.curSizes[i] = dockBaseIconSize
		}
	}

	hoveredIdx := -1
	cursorDistance := 0.5
	if d.hover.inside {
		for _, slot := range d.slots {
			if d.hover.x >= slot.rect.Min.X && d.hover.x < slot.rect.Max.X &&
				d.hover.y >= d.pillRect.Min.Y-24 && d.hover.y <= d.pillRect.Max.Y {
				hoveredIdx = slot.index
				slotW := float64(slot.rect.Dx())
				if slotW > 0 {
					cursorDistance = float64(d.hover.x-slot.rect.Min.X) / slotW
					if cursorDistance < 0 {
						cursorDistance = 0
					}
					if cursorDistance > 1 {
						cursorDistance = 1
					}
				}
				break
			}
		}
	}
	d.hoveredIdx = hoveredIdx

	// scaleValue(cursorDistance, [0, 1], [-dockMaxOffsetPx, +dockMaxOffsetPx])
	offsetPixels := (cursorDistance*2.0 - 1.0) * dockMaxOffsetPx
	offsetLeft := -offsetPixels
	offsetRight := offsetPixels

	animating := false
	for i := 0; i < n; i++ {
		target := dockBaseIconSize
		if hoveredIdx >= 0 {
			diff := i - hoveredIdx
			switch diff {
			case 0:
				target = dockPeakIconSize
			case 1:
				target = dockNeighbor1Size + offsetRight
			case 2:
				target = dockNeighbor2Size + offsetRight
			case -1:
				target = dockNeighbor1Size + offsetLeft
			case -2:
				target = dockNeighbor2Size + offsetLeft
			}
		}

		delta := target - d.curSizes[i]
		if math.Abs(delta) > 0.12 {
			// Smooth cubic-bezier(0.25, 1, 0.5, 1) 100ms ease-out step at 60fps
			d.curSizes[i] += delta * 0.32
			animating = true
		} else if d.curSizes[i] != target {
			d.curSizes[i] = target
			animating = true
		}
	}

	return animating
}

func (d *Dock) updateInputRegion(rect image.Rectangle) {
	if d.surface == nil || d.disp == nil {
		return
	}
	region, err := d.disp.Compositor().CreateRegion()
	if err != nil {
		return
	}
	if !rect.Empty() {
		_ = region.Add(int32(rect.Min.X), int32(rect.Min.Y), int32(rect.Dx()), int32(rect.Dy()))
	}
	_ = d.surface.WlSurface().SetInputRegion(region)
	_ = region.Destroy()
}

func (d *Dock) draw(width, height int, pixels []byte) {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Rect, image.NewUniform(color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00}), image.Point{}, draw.Src)

	d.mu.Lock()
	entries := make([]dockEntry, len(d.entries))
	copy(entries, d.entries)
	sizes := make([]float64, len(d.curSizes))
	copy(sizes, d.curSizes)
	hoveredIdx := d.hoveredIdx
	d.mu.Unlock()

	if len(entries) == 0 {
		writeARGB8888(pixels, canvas)
		d.mu.Lock()
		d.slots = nil
		d.pillRect = image.Rectangle{}
		d.mu.Unlock()
		d.updateInputRegion(image.Rectangle{})
		return
	}

	// Calculate total width of dock shelf based on current animated icon sizes
	totalItemsWidth := 0
	for i, sz := range sizes {
		slotW := int(math.Round(sz)) + 2*dockItemPadX
		totalItemsWidth += slotW
		if entries[i].groupEnd {
			totalItemsWidth += dockGroupGap
		}
	}
	pillWidth := totalItemsWidth + 2*dockPadX
	pillX := (width - pillWidth) / 2
	pillBottom := height - dockBottomMargin
	pillTop := pillBottom - dockShelfHeight
	pillRect := image.Rect(pillX, pillTop, pillX+pillWidth, pillBottom)

	// Draw borderless dock shelf background (#181818) with continuous SDF anti-aliasing
	drawSmoothRoundedRect(canvas, pillRect, dockShelfRadius, color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xff}, draw.Src)

	slots := make([]dockSlotHit, 0, len(entries))
	x := pillX + dockPadX
	// Base icon bottom baseline sits dockPadBottom (12px) above the bottom of the dock shelf,
	// leaving dockPadTop (10px) above the resting icon.
	iconBottomY := pillBottom - dockPadBottom

	var tooltipTitle string
	var tooltipCenterX int
	var tooltipBottomY int

	for i, entry := range entries {
		iconSize := int(math.Round(sizes[i]))
		slotW := iconSize + 2*dockItemPadX
		iconX := x + dockItemPadX
		// Icons grow upward from iconBottomY (matching margin-top: -delta in index.css)
		iconY := iconBottomY - iconSize
		iconRect := image.Rect(iconX, iconY, iconX+iconSize, iconY+iconSize)

		if entry.icon != nil {
			xdraw.CatmullRom.Scale(canvas, iconRect, entry.icon, entry.icon.Bounds(), xdraw.Over, nil)
		} else {
			drawSmoothRoundedRect(canvas, iconRect, 8, color.RGBA{R: 0x44, G: 0x44, B: 0x44, A: 0xff}, draw.Over)
		}

		// Active / focused window indicator dot centered in the bottom padding
		dotCenterX := float64(x) + float64(slotW)/2.0
		dotCenterY := float64(pillBottom) - float64(dockPadBottom)/2.0
		if entry.window.IsFocused {
			drawSmoothCircle(canvas, dotCenterX, dotCenterY, 2.2, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
		} else {
			drawSmoothCircle(canvas, dotCenterX, dotCenterY, 1.6, color.RGBA{R: 0x77, G: 0x77, B: 0x77, A: 0xff})
		}

		slotRect := image.Rect(x, iconY, x+slotW, pillBottom)
		slots = append(slots, dockSlotHit{
			rect:  slotRect,
			entry: entry,
			index: i,
		})

		if i == hoveredIdx {
			tooltipTitle = entry.window.Title
			if tooltipTitle == "" {
				tooltipTitle = entry.window.AppID
			}
			tooltipCenterX = x + slotW/2
			tooltipBottomY = iconY - 6
		}

		x += slotW
		if entry.groupEnd {
			sepX := x + dockGroupGap/2
			sepRect := image.Rect(sepX, pillTop+12, sepX+1, pillBottom-12)
			draw.Draw(canvas, sepRect, image.NewUniform(color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff}), image.Point{}, draw.Over)
			x += dockGroupGap
		}
	}

	// Draw floating tooltip above hovered icon (matching .tooltip in index.css)
	if tooltipTitle != "" && tooltipBottomY > 4 {
		label := truncateToWidth(d.face, strings.TrimSpace(tooltipTitle), 240)
		textW := font.MeasureString(d.face, label).Ceil()
		tipW := textW + 16
		tipH := 24
		tipX := tooltipCenterX - tipW/2
		if tipX < 8 {
			tipX = 8
		}
		if tipX+tipW > width-8 {
			tipX = width - 8 - tipW
		}
		tipY := tooltipBottomY - tipH
		if tipY < 2 {
			tipY = 2
		}
		tipRect := image.Rect(tipX, tipY, tipX+tipW, tipY+tipH)
		drawSmoothRoundedRect(canvas, tipRect, 6.0, color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xf2}, draw.Over)
		drawer := font.Drawer{
			Dst:  canvas,
			Src:  image.NewUniform(color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}),
			Face: d.face,
			Dot:  fixed.P(tipX+8, textBaseline(d.face, tipRect)),
		}
		drawer.DrawString(label)
	}

	writeARGB8888(pixels, canvas)

	// Expand input region slightly upward when hovered so moving across magnified icons stays inside input region
	inputTop := pillTop
	if hoveredIdx >= 0 {
		inputTop = pillTop - 22
		if inputTop < 0 {
			inputTop = 0
		}
	}
	inputRect := image.Rect(pillRect.Min.X, inputTop, pillRect.Max.X, pillRect.Max.Y)

	d.mu.Lock()
	d.slots = slots
	d.pillRect = pillRect
	d.mu.Unlock()

	d.updateInputRegion(inputRect)
}

// drawSmoothRoundedRect renders a rounded rectangle using continuous analytic SDF coverage
// (256 sub-pixel alpha levels) for smooth, non-rasterized corners.
func drawSmoothRoundedRect(dst *image.RGBA, rect image.Rectangle, radius float64, col color.RGBA, op draw.Op) {
	if rect.Empty() {
		return
	}
	hx := float64(rect.Dx()) / 2.0
	hy := float64(rect.Dy()) / 2.0
	r := radius
	if r > hx {
		r = hx
	}
	if r > hy {
		r = hy
	}
	if r <= 0 {
		draw.Draw(dst, rect, image.NewUniform(col), image.Point{}, op)
		return
	}

	cx := float64(rect.Min.X) + hx
	cy := float64(rect.Min.Y) + hy
	innerHx := hx - r
	innerHy := hy - r
	src := color.NRGBAModel.Convert(col).(color.NRGBA)

	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		py := float64(y) + 0.5
		qy := math.Abs(py-cy) - innerHy
		for x := rect.Min.X; x < rect.Max.X; x++ {
			px := float64(x) + 0.5
			qx := math.Abs(px-cx) - innerHx

			var dist float64
			if qx <= 0 && qy <= 0 {
				// Strictly inside the non-corner region
				dist = math.Max(qx, qy) - r
			} else {
				outX := math.Max(qx, 0)
				outY := math.Max(qy, 0)
				dist = math.Hypot(outX, outY) - r
			}

			coverage := 0.5 - dist
			if coverage <= 0 {
				continue
			}
			if coverage > 1 {
				coverage = 1
			}
			drawRoundedRectPixel(dst, x, y, src, coverage, op)
		}
	}
}

func (d *Dock) HandlePointerEnter(ev client.WlPointerEnterEvent) {
	d.mu.Lock()
	if d.closed || d.surface == nil || ev.Surface != d.surface.WlSurface() {
		d.mu.Unlock()
		return
	}
	d.hover.inside = true
	d.hover.x = int(ev.SurfaceX)
	d.hover.y = int(ev.SurfaceY)
	d.mu.Unlock()
	d.markDirty()
}

func (d *Dock) HandlePointerLeave(ev client.WlPointerLeaveEvent) {
	d.mu.Lock()
	if d.closed || d.surface == nil || ev.Surface != d.surface.WlSurface() {
		d.mu.Unlock()
		return
	}
	d.hover.inside = false
	d.hoveredIdx = -1
	d.mu.Unlock()
	d.markDirty()
}

func (d *Dock) HandlePointerMotion(ev client.WlPointerMotionEvent) {
	d.mu.Lock()
	if d.closed || !d.hover.inside {
		d.mu.Unlock()
		return
	}
	d.hover.x = int(ev.SurfaceX)
	d.hover.y = int(ev.SurfaceY)
	d.mu.Unlock()
	d.markDirty()
}

func (d *Dock) HandlePointerButton(ev client.WlPointerButtonEvent) bool {
	d.mu.Lock()
	if d.closed || !d.hover.inside {
		d.mu.Unlock()
		return false
	}
	if ev.Button != 0x110 || ev.State != client.WlPointerButtonStatePressed {
		d.mu.Unlock()
		return true
	}

	pt := image.Pt(d.hover.x, d.hover.y)
	var targetID int64 = -1
	for _, slot := range d.slots {
		if pt.X >= slot.rect.Min.X && pt.X < slot.rect.Max.X && pt.Y >= slot.rect.Min.Y && pt.Y <= slot.rect.Max.Y {
			targetID = slot.entry.window.ID
			break
		}
	}
	d.mu.Unlock()

	if targetID >= 0 {
		go func(id int64) {
			cmd := exec.Command("niri", "msg", "action", "focus-window", "--id", strconv.FormatInt(id, 10))
			if err := cmd.Run(); err != nil {
				log.Printf("Dock focus click error: id=%d err=%v", id, err)
			}
		}(targetID)
	}
	return true
}

// SimulateHover sets the simulated hover position on the dock (used for testing/previewing magnification).
func (d *Dock) SimulateHover(slotIdx int) {
	d.mu.Lock()
	if slotIdx < 0 || slotIdx >= len(d.slots) {
		d.hover.inside = false
		d.hoveredIdx = -1
		d.mu.Unlock()
		d.markDirty()
		return
	}
	slot := d.slots[slotIdx]
	d.hover.inside = true
	d.hover.x = (slot.rect.Min.X + slot.rect.Max.X) / 2
	d.hover.y = (d.pillRect.Min.Y + d.pillRect.Max.Y) / 2
	d.mu.Unlock()
	d.markDirty()
}

func (d *Dock) Close() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	close(d.stop)
	unsub := d.unsubscribe
	d.unsubscribe = nil
	fr := d.frame
	d.frame = nil
	ly := d.layer
	d.layer = nil
	sf := d.surface
	d.surface = nil
	d.mu.Unlock()

	if unsub != nil {
		unsub()
	}
	if fr != nil {
		_ = fr.Close()
	}
	if ly != nil {
		_ = ly.Close()
	}
	if sf != nil {
		_ = sf.Close()
	}
}
