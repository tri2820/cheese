package main

import (
	"bufio"
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
	"net"
	"os"
	"sync"
	"time"

	"github.com/tri2820/cheese/apps/common"
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/seat"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/shm"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/protocols/cursor_shape_v1"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
)

const (
	barHeight          = 28
	barTopMargin       = 6
	barName            = "cheese-client-bar"
	barSocket          = "/tmp/cheese-client-bar.sock"
	moduleGap          = 4
	barSidePad         = 12
	moduleCornerRadius = 6
)

func main() {
	log.Println("Starting client bar...")

	disp := display.MustConnect(display.Config{
		Required: display.RequiredGlobals{
			Compositor: true,
			Shm:        true,
			LayerShell: true,
			Seat:       true,
		},
	})

	if !disp.HasFormat(client.WlShmFormatArgb8888) {
		log.Fatal("ARGB8888 not supported")
	}

	app := NewApp()
	defer app.Close()
	app.BindPointer(disp.FirstSeat(), disp.CursorShape())
	if err := app.ListenCommands(barSocket); err != nil {
		log.Printf("Command socket error: %v", err)
	}

	disp.OnOutput(func(output *display.Output, added bool) {
		if added {
			app.AddBar(disp, output)
			return
		}
		app.RemoveBar(output)
	})

	for _, output := range disp.ReadyOutputs() {
		app.AddBar(disp, output)
	}

	log.Println("client bar running")

	if err := disp.Run(); err != nil {
		log.Printf("Dispatch error: %v", err)
	}
}

type App struct {
	mu        sync.Mutex
	bars      map[*client.WlOutput]*Bar
	listener  net.Listener
	audio     *AudioService
	network   *NetworkService
	bluetooth *BluetoothService
	niri      *NiriService
	icons     *IconResolver
}

func NewApp() *App {
	return &App{
		bars:      make(map[*client.WlOutput]*Bar),
		audio:     NewAudioService(),
		network:   NewNetworkService(),
		bluetooth: NewBluetoothService(),
		niri:      NewNiriService(),
		icons:     NewIconResolver(),
	}
}

func (a *App) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.listener != nil {
		_ = a.listener.Close()
		a.listener = nil
	}
	if a.audio != nil {
		a.audio.Close()
		a.audio = nil
	}
	if a.network != nil {
		a.network.Close()
		a.network = nil
	}
	if a.bluetooth != nil {
		a.bluetooth.Close()
		a.bluetooth = nil
	}
	if a.niri != nil {
		a.niri.Close()
		a.niri = nil
	}
}

func (a *App) ListenCommands(path string) error {
	_ = os.Remove(path)

	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	a.listener = ln

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go a.handleCommandConn(conn)
		}
	}()

	return nil
}

func (a *App) handleCommandConn(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		a.HandleCommand(scanner.Text())
	}
}

func (a *App) HandleCommand(cmd string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, bar := range a.bars {
		bar.HandleCommand(cmd)
	}
}

func (a *App) BindPointer(s *seat.Seat, manager *cursor_shape_v1.WpCursorShapeManagerV1) {
	if s == nil || s.Pointer() == nil {
		return
	}

	pointer := s.Pointer()
	var cursorDevice *cursor_shape_v1.WpCursorShapeDeviceV1
	if manager != nil {
		device, err := manager.GetPointer(pointer.WlPointer())
		if err != nil {
			log.Printf("Failed to get cursor shape device: %v", err)
		} else {
			cursorDevice = device
		}
	}
	pointer.OnEnter(func(ev client.WlPointerEnterEvent) {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, bar := range a.bars {
			bar.HandlePointerEnter(ev, cursorDevice)
		}
	})
	pointer.OnLeave(func(ev client.WlPointerLeaveEvent) {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, bar := range a.bars {
			bar.HandlePointerLeave(ev, cursorDevice)
		}
	})
	pointer.OnMotion(func(ev client.WlPointerMotionEvent) {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, bar := range a.bars {
			bar.HandlePointerMotion(ev, cursorDevice)
		}
	})
	pointer.OnButton(func(ev client.WlPointerButtonEvent) {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, bar := range a.bars {
			bar.HandlePointerButton(ev)
		}
	})
}

func (a *App) AddBar(disp *display.Display, output *display.Output) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.bars[output.WlOutput()]; ok {
		return
	}

	bar, err := NewBar(disp, output, a.audio, a.network, a.bluetooth, a.niri, a.icons)
	if err != nil {
		log.Printf("Failed to create bar for %s: %v", output.Name, err)
		return
	}

	a.bars[output.WlOutput()] = bar
	log.Printf("Added bar for output: %s", output.Name)
}

func (a *App) RemoveBar(output *display.Output) {
	a.mu.Lock()
	defer a.mu.Unlock()

	bar, ok := a.bars[output.WlOutput()]
	if !ok {
		return
	}

	bar.Close()
	delete(a.bars, output.WlOutput())
	log.Printf("Removed bar for output: %s", output.Name)
}

type Bar struct {
	output        *display.Output
	surface       *surface.Surface
	layer         *shell.LayerSurface
	frame         *shm.Frame
	face          font.Face
	leftModules   []Module
	centerModules []Module
	rightModules  []Module

	closeMu sync.Mutex
	closed  bool
	stop    chan struct{}
	dirtyCh chan struct{}
	drawMu  sync.RWMutex
	hits    []moduleHit
	hover   pointerState
}

type moduleHit struct {
	rect image.Rectangle
	mod  Module
}

type pointerState struct {
	inside bool
	x      int
	y      int
	serial uint32
	shape  cursor_shape_v1.WpCursorShapeDeviceV1Shape
	mod    Module
	rect   image.Rectangle
}

func NewBar(disp *display.Display, output *display.Output, audio *AudioService, network *NetworkService, bluetooth *BluetoothService, niri *NiriService, icons *IconResolver) (*Bar, error) {
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		return nil, err
	}

	layer, err := shell.NewLayer(surf, disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          barName,
		Anchor:        shell.AnchorTop | shell.AnchorLeft | shell.AnchorRight,
		Width:         0,
		Height:        barHeight,
		ExclusiveZone: barHeight + barTopMargin,
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
	if err := layer.SetMargin(barTopMargin, 0, 0, 0); err != nil {
		_ = frame.Close()
		_ = layer.Close()
		_ = surf.Close()
		return nil, err
	}

	b := &Bar{
		output:  output,
		surface: surf,
		layer:   layer,
		frame:   frame,
		face:    loadBarFont(output),
		stop:    make(chan struct{}),
		dirtyCh: make(chan struct{}, 1),
	}

	b.leftModules = []Module{
		NewTaskbarModule(output.Name, niri, icons),
	}
	b.centerModules = []Module{
		NewClockModule(),
	}
	b.rightModules = []Module{
		NewInputMethodModule(),
		NewMicModule(audio),
		NewVolumeModule(audio),
		NewBluetoothModule(bluetooth),
		NewWifiModule(network),
		NewBatteryModule(),
	}

	frame.SetRender(func(width, height int, frameTime uint32, pixels []byte) {
		b.draw(width, height, pixels)
	})
	frame.SetManualMode(true)
	frame.OnError(func(err error) {
		log.Printf("Bar frame error on %s: %v", output.Name, err)
	})

	layer.OnClose(func() {
		b.Close()
	})

	for _, mod := range append([]Module{}, b.leftModules...) {
		mod.Start(b.markDirty)
	}
	for _, mod := range append([]Module{}, b.centerModules...) {
		mod.Start(b.markDirty)
	}
	for _, mod := range append([]Module{}, b.rightModules...) {
		mod.Start(b.markDirty)
	}

	go b.renderLoop()

	return b, nil
}

func (b *Bar) markDirty() {
	select {
	case b.dirtyCh <- struct{}{}:
	default:
	}
}

func (b *Bar) renderLoop() {
	for {
		select {
		case <-b.dirtyCh:
			if b.frame.Ready() {
				b.frame.ManualRender(uint32(time.Now().UnixMilli()))
			}
		case <-b.stop:
			return
		}
	}
}

func (b *Bar) Close() {
	b.closeMu.Lock()
	defer b.closeMu.Unlock()

	if b.closed {
		return
	}
	b.closed = true
	close(b.stop)

	for _, mod := range append([]Module{}, b.leftModules...) {
		mod.Close()
	}
	for _, mod := range append([]Module{}, b.centerModules...) {
		mod.Close()
	}
	for _, mod := range append([]Module{}, b.rightModules...) {
		mod.Close()
	}

	if b.frame != nil {
		_ = b.frame.Close()
	}
	if b.layer != nil {
		_ = b.layer.Close()
	}
	if b.surface != nil {
		_ = b.surface.Close()
	}
}

func (b *Bar) HandleCommand(cmd string) {
	for _, mod := range b.leftModules {
		commandModule, ok := mod.(CommandModule)
		if !ok {
			continue
		}
		commandModule.HandleCommand(cmd)
	}
	for _, mod := range b.centerModules {
		commandModule, ok := mod.(CommandModule)
		if !ok {
			continue
		}
		commandModule.HandleCommand(cmd)
	}
	for _, mod := range b.rightModules {
		commandModule, ok := mod.(CommandModule)
		if !ok {
			continue
		}
		commandModule.HandleCommand(cmd)
	}
}

func (b *Bar) draw(width, height int, pixels []byte) {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))

	draw.Draw(canvas, canvas.Rect, image.NewUniform(color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00}), image.Point{}, draw.Src)

	hits := make([]moduleHit, 0, len(b.leftModules)+len(b.centerModules)+len(b.rightModules))
	hits = b.drawLeft(canvas, image.Rect(0, 0, width, height), b.leftModules, hits)
	hits = b.drawCentered(canvas, image.Rect(0, 0, width, height), b.centerModules, hits)
	hits = b.drawRight(canvas, image.Rect(0, 0, width, height), b.rightModules, hits)

	writeARGB8888(pixels, canvas)

	b.drawMu.Lock()
	b.hits = hits
	if b.hover.inside {
		b.updateHoveredModuleLocked()
	}
	b.drawMu.Unlock()
}

func (b *Bar) drawCentered(dst draw.Image, rect image.Rectangle, mods []Module, hits []moduleHit) []moduleHit {
	totalWidth := modulesWidth(mods, b.face, moduleGap)
	x := rect.Min.X + (rect.Dx()-totalWidth)/2
	for i, mod := range mods {
		w := mod.Width(b.face)
		moduleRect := image.Rect(x, rect.Min.Y, x+w, rect.Max.Y)
		drawModuleBackground(dst, moduleRect)
		if hoverRect, ok := b.currentHoverRect(mod, moduleRect); ok {
			drawHoverBackground(dst, hoverRect)
		}
		mod.Draw(dst, moduleRect, b.face)
		hits = append(hits, moduleHit{rect: moduleRect, mod: mod})
		x += w
		if i < len(mods)-1 {
			x += moduleGap
		}
	}
	return hits
}

func (b *Bar) drawRight(dst draw.Image, rect image.Rectangle, mods []Module, hits []moduleHit) []moduleHit {
	totalWidth := modulesWidth(mods, b.face, moduleGap)
	x := rect.Max.X - barSidePad - totalWidth
	for i, mod := range mods {
		w := mod.Width(b.face)
		moduleRect := image.Rect(x, rect.Min.Y, x+w, rect.Max.Y)
		drawModuleBackground(dst, moduleRect)
		if hoverRect, ok := b.currentHoverRect(mod, moduleRect); ok {
			drawHoverBackground(dst, hoverRect)
		}
		mod.Draw(dst, moduleRect, b.face)
		hits = append(hits, moduleHit{rect: moduleRect, mod: mod})
		x += w
		if i < len(mods)-1 {
			x += moduleGap
		}
	}
	return hits
}

func (b *Bar) HandlePointerEnter(ev client.WlPointerEnterEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	if ev.Surface != b.surface.WlSurface() {
		return
	}

	b.drawMu.Lock()
	b.hover.inside = true
	b.hover.x = int(ev.SurfaceX)
	b.hover.y = int(ev.SurfaceY)
	b.hover.serial = ev.Serial
	changed := b.updateHoveredModuleLocked()
	b.drawMu.Unlock()
	b.updateCursorShape(cursor)
	if changed {
		b.markDirty()
	}
}

func (b *Bar) HandlePointerLeave(ev client.WlPointerLeaveEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	if ev.Surface != b.surface.WlSurface() {
		return
	}

	b.drawMu.Lock()
	changed := b.hover.mod != nil
	b.hover = pointerState{}
	b.drawMu.Unlock()
	if cursor != nil {
		_ = cursor.SetShape(ev.Serial, cursor_shape_v1.WpCursorShapeDeviceV1ShapeDefault)
	}
	if changed {
		b.markDirty()
	}
}

func (b *Bar) HandlePointerMotion(ev client.WlPointerMotionEvent, cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	b.drawMu.Lock()
	if !b.hover.inside {
		b.drawMu.Unlock()
		return
	}
	b.hover.x = int(ev.SurfaceX)
	b.hover.y = int(ev.SurfaceY)
	changed := b.updateHoveredModuleLocked()
	b.drawMu.Unlock()
	b.updateCursorShape(cursor)
	if changed {
		b.markDirty()
	}
}

func (b *Bar) HandlePointerButton(ev client.WlPointerButtonEvent) {
	if ev.State != client.WlPointerButtonStatePressed {
		return
	}

	b.drawMu.RLock()
	defer b.drawMu.RUnlock()
	if !b.hover.inside {
		return
	}

	point := image.Pt(b.hover.x, b.hover.y)
	for _, hit := range b.hits {
		if !point.In(hit.rect) {
			continue
		}
		if pointing, ok := hit.mod.(PointingModule); ok {
			if pointing.OnClickAt(ev.Button, hit.rect, point) {
				return
			}
		}
		clickable, ok := hit.mod.(ClickableModule)
		if !ok {
			return
		}
		clickable.OnClick(ev.Button)
		return
	}
}

func (b *Bar) updateCursorShape(cursor *cursor_shape_v1.WpCursorShapeDeviceV1) {
	if cursor == nil {
		return
	}

	b.drawMu.Lock()
	defer b.drawMu.Unlock()
	if !b.hover.inside || b.hover.serial == 0 {
		return
	}

	shape := cursor_shape_v1.WpCursorShapeDeviceV1ShapeDefault
	if b.hover.mod != nil {
		shape = cursor_shape_v1.WpCursorShapeDeviceV1ShapePointer
	}

	if shape == b.hover.shape {
		return
	}
	if err := cursor.SetShape(b.hover.serial, shape); err != nil {
		log.Printf("Failed to set cursor shape on %s: %v", b.output.Name, err)
		return
	}
	b.hover.shape = shape
}

func (b *Bar) isHovered(mod Module) bool {
	b.drawMu.RLock()
	defer b.drawMu.RUnlock()
	return b.hover.mod == mod
}

func (b *Bar) hoverRect(mod Module) (image.Rectangle, bool) {
	b.drawMu.RLock()
	defer b.drawMu.RUnlock()
	if b.hover.mod != mod {
		return image.Rectangle{}, false
	}
	return b.hover.rect, true
}

func (b *Bar) currentHoverRect(mod Module, moduleRect image.Rectangle) (image.Rectangle, bool) {
	b.drawMu.RLock()
	defer b.drawMu.RUnlock()
	if !b.hover.inside || b.hover.mod != mod {
		return image.Rectangle{}, false
	}
	if pointing, ok := mod.(PointingModule); ok {
		if rect, ok := pointing.HoverRect(moduleRect, image.Pt(b.hover.x, b.hover.y)); ok {
			return rect, true
		}
		return image.Rectangle{}, false
	}
	return moduleRect, true
}

func (b *Bar) updateHoveredModuleLocked() bool {
	var hovered Module
	var hoveredRect image.Rectangle
	point := image.Pt(b.hover.x, b.hover.y)
	for _, hit := range b.hits {
		if !point.In(hit.rect) {
			continue
		}
		if pointing, ok := hit.mod.(PointingModule); ok {
			if rect, ok := pointing.HoverRect(hit.rect, point); ok {
				hovered = hit.mod
				hoveredRect = rect
			}
			break
		}
		if _, ok := hit.mod.(ClickableModule); ok {
			hovered = hit.mod
			hoveredRect = hit.rect
		}
		break
	}

	if b.hover.mod == hovered && b.hover.rect == hoveredRect {
		return false
	}
	b.hover.mod = hovered
	b.hover.rect = hoveredRect
	return true
}

func modulesWidth(mods []Module, face font.Face, gap int) int {
	total := 0
	for i, mod := range mods {
		total += mod.Width(face)
		if i < len(mods)-1 {
			total += gap
		}
	}
	return total
}

func textBaseline(face font.Face, rect image.Rectangle) int {
	metrics := face.Metrics()
	return rect.Min.Y + (rect.Dy()+metrics.Ascent.Ceil()-metrics.Descent.Ceil())/2
}

func drawHoverBackground(dst draw.Image, rect image.Rectangle) {
	drawRoundedRect(dst, rect, moduleCornerRadius, color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff}, draw.Over)
}

func drawModuleBackground(dst draw.Image, rect image.Rectangle) {
	drawRoundedRect(dst, rect, moduleCornerRadius, color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}, draw.Src)
}

func drawRoundedRect(dst draw.Image, rect image.Rectangle, radius int, col color.Color, op draw.Op) {
	if rect.Empty() {
		return
	}

	r := radius
	if maxR := rect.Dx() / 2; r > maxR {
		r = maxR
	}
	if maxR := rect.Dy() / 2; r > maxR {
		r = maxR
	}
	if r <= 0 {
		draw.Draw(dst, rect, image.NewUniform(col), image.Point{}, op)
		return
	}

	src := color.NRGBAModel.Convert(col).(color.NRGBA)
	const samples = 4
	invSamples := 1.0 / float64(samples*samples)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			coverage := 0.0
			for sy := 0; sy < samples; sy++ {
				py := float64(y) + (float64(sy)+0.5)/float64(samples)
				for sx := 0; sx < samples; sx++ {
					px := float64(x) + (float64(sx)+0.5)/float64(samples)
					if pointInRoundedRect(px, py, rect, float64(r)) {
						coverage += invSamples
					}
				}
			}
			if coverage == 0 {
				continue
			}
			drawRoundedRectPixel(dst, x, y, src, coverage, op)
		}
	}
}

func pointInRoundedRect(px, py float64, rect image.Rectangle, radius float64) bool {
	minX := float64(rect.Min.X)
	minY := float64(rect.Min.Y)
	maxX := float64(rect.Max.X)
	maxY := float64(rect.Max.Y)

	if px < minX || px >= maxX || py < minY || py >= maxY {
		return false
	}
	if px >= minX+radius && px < maxX-radius {
		return true
	}
	if py >= minY+radius && py < maxY-radius {
		return true
	}

	cx := minX + radius
	if px >= maxX-radius {
		cx = maxX - radius
	}
	cy := minY + radius
	if py >= maxY-radius {
		cy = maxY - radius
	}
	dx := px - cx
	dy := py - cy
	return dx*dx+dy*dy <= radius*radius
}

func drawRoundedRectPixel(dst draw.Image, x, y int, src color.NRGBA, coverage float64, op draw.Op) {
	srcAlpha := (float64(src.A) / 255.0) * coverage
	if srcAlpha <= 0 {
		return
	}

	switch op {
	case draw.Src:
		dst.Set(x, y, color.NRGBA{
			R: src.R,
			G: src.G,
			B: src.B,
			A: uint8(math.Round(srcAlpha * 255)),
		})
	case draw.Over:
		dstColor := color.NRGBAModel.Convert(dst.At(x, y)).(color.NRGBA)
		dstAlpha := float64(dstColor.A) / 255.0
		outAlpha := srcAlpha + dstAlpha*(1-srcAlpha)
		if outAlpha <= 0 {
			dst.Set(x, y, color.NRGBA{})
			return
		}
		srcR := float64(src.R) / 255.0
		srcG := float64(src.G) / 255.0
		srcB := float64(src.B) / 255.0
		dstR := float64(dstColor.R) / 255.0
		dstG := float64(dstColor.G) / 255.0
		dstB := float64(dstColor.B) / 255.0
		outR := (srcR*srcAlpha + dstR*dstAlpha*(1-srcAlpha)) / outAlpha
		outG := (srcG*srcAlpha + dstG*dstAlpha*(1-srcAlpha)) / outAlpha
		outB := (srcB*srcAlpha + dstB*dstAlpha*(1-srcAlpha)) / outAlpha
		dst.Set(x, y, color.NRGBA{
			R: uint8(math.Round(outR * 255)),
			G: uint8(math.Round(outG * 255)),
			B: uint8(math.Round(outB * 255)),
			A: uint8(math.Round(outAlpha * 255)),
		})
	}
}

func writeARGB8888(dst []byte, src *image.RGBA) {
	copyLen := len(src.Pix)
	if len(dst) < copyLen {
		copyLen = len(dst)
	}
	for i := 0; i < copyLen; i += 4 {
		r := src.Pix[i+0]
		g := src.Pix[i+1]
		b := src.Pix[i+2]
		a := src.Pix[i+3]
		dst[i+0] = b
		dst[i+1] = g
		dst[i+2] = r
		dst[i+3] = a
	}
}

func loadBarFont(output *display.Output) font.Face {
	dpi := output.DPIOrDefault()
	face, err := common.LoadFont("JetBrainsMono Nerd Font", 10, dpi)
	if err != nil {
		log.Printf("Falling back to basic font on %s: %v", output.Name, err)
		return basicfont.Face7x13
	}
	return face
}
func (b *Bar) drawLeft(dst draw.Image, rect image.Rectangle, mods []Module, hits []moduleHit) []moduleHit {
	x := rect.Min.X + barSidePad
	for i, mod := range mods {
		w := mod.Width(b.face)
		moduleRect := image.Rect(x, rect.Min.Y, x+w, rect.Max.Y)
		drawModuleBackground(dst, moduleRect)
		if hoverRect, ok := b.currentHoverRect(mod, moduleRect); ok {
			drawHoverBackground(dst, hoverRect)
		}
		mod.Draw(dst, moduleRect, b.face)
		hits = append(hits, moduleHit{rect: moduleRect, mod: mod})
		x += w
		if i < len(mods)-1 {
			x += moduleGap
		}
	}
	return hits
}
