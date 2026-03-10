package main

import (
	"bufio"
	"image"
	"image/color"
	"image/draw"
	"log"
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
	barHeight  = 28
	barName    = "cheese-client-bar"
	barSocket  = "/tmp/cheese-client-bar.sock"
	moduleGap  = 12
	barSidePad = 12
	centerGap  = 12
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
	mu       sync.Mutex
	bars     map[*client.WlOutput]*Bar
	listener net.Listener
	audio    *AudioService
}

func NewApp() *App {
	return &App{
		bars:  make(map[*client.WlOutput]*Bar),
		audio: NewAudioService(),
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

	bar, err := NewBar(disp, output, a.audio)
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
}

func NewBar(disp *display.Display, output *display.Output, audio *AudioService) (*Bar, error) {
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
		ExclusiveZone: barHeight,
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

	b := &Bar{
		output:  output,
		surface: surf,
		layer:   layer,
		frame:   frame,
		face:    loadBarFont(output),
		stop:    make(chan struct{}),
		dirtyCh: make(chan struct{}, 1),
	}

	b.centerModules = []Module{
		NewClockModule(),
	}
	b.rightModules = []Module{
		NewInputMethodModule(),
		NewMicModule(audio),
		NewVolumeModule(audio),
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
	dst := &image.RGBA{
		Pix:    pixels,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}

	draw.Draw(dst, dst.Rect, image.NewUniform(argb(0x00, 0x00, 0x00, 0xff)), image.Point{}, draw.Src)

	hits := make([]moduleHit, 0, len(b.centerModules)+len(b.rightModules))
	hits = b.drawCentered(dst, image.Rect(0, 0, width, height), b.centerModules, hits)
	hits = b.drawRight(dst, image.Rect(0, 0, width, height), b.rightModules, hits)

	b.drawMu.Lock()
	b.hits = hits
	b.drawMu.Unlock()
}

func (b *Bar) drawCentered(dst draw.Image, rect image.Rectangle, mods []Module, hits []moduleHit) []moduleHit {
	totalWidth := modulesWidth(mods, b.face, centerGap)
	x := rect.Min.X + (rect.Dx()-totalWidth)/2
	for i, mod := range mods {
		w := mod.Width(b.face)
		moduleRect := image.Rect(x, rect.Min.Y, x+w, rect.Max.Y)
		if b.isHovered(mod) {
			drawHoverBackground(dst, moduleRect)
		}
		mod.Draw(dst, moduleRect, b.face)
		drawDebugOverlay(dst, mod, moduleRect)
		hits = append(hits, moduleHit{rect: moduleRect, mod: mod})
		x += w
		if i < len(mods)-1 {
			x += centerGap
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
		if b.isHovered(mod) {
			drawHoverBackground(dst, moduleRect)
		}
		mod.Draw(dst, moduleRect, b.face)
		drawDebugOverlay(dst, mod, moduleRect)
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
	if _, ok := b.hover.mod.(ClickableModule); ok {
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

func (b *Bar) updateHoveredModuleLocked() bool {
	var hovered Module
	point := image.Pt(b.hover.x, b.hover.y)
	for _, hit := range b.hits {
		if !point.In(hit.rect) {
			continue
		}
		if _, ok := hit.mod.(ClickableModule); ok {
			hovered = hit.mod
		}
		break
	}

	if b.hover.mod == hovered {
		return false
	}
	b.hover.mod = hovered
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
	draw.Draw(dst, rect, image.NewUniform(argb(0x20, 0x20, 0x20, 0xff)), image.Point{}, draw.Src)
}

func drawDebugOverlay(dst draw.Image, mod Module, rect image.Rectangle) {
	// if _, ok := mod.(*InputMethodModule); !ok {
	return
	// }

	leftPadRect := image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+textModulePadX, rect.Max.Y)
	rightPadRect := image.Rect(rect.Max.X-textModulePadX, rect.Min.Y, rect.Max.X, rect.Max.Y)
	topBorderRect := image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+1)
	bottomBorderRect := image.Rect(rect.Min.X, rect.Max.Y-1, rect.Max.X, rect.Max.Y)

	draw.Draw(dst, leftPadRect, image.NewUniform(argb(0x00, 0x00, 0xff, 0x80)), image.Point{}, draw.Src)
	draw.Draw(dst, rightPadRect, image.NewUniform(argb(0x00, 0x00, 0xff, 0x80)), image.Point{}, draw.Src)
	draw.Draw(dst, topBorderRect, image.NewUniform(argb(0xff, 0x00, 0x00, 0xff)), image.Point{}, draw.Src)
	draw.Draw(dst, bottomBorderRect, image.NewUniform(argb(0xff, 0x00, 0x00, 0xff)), image.Point{}, draw.Src)
}

func argb(r, g, b, a uint8) color.RGBA {
	return color.RGBA{R: b, G: g, B: r, A: a}
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
