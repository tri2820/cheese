package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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

	app := NewApp(func() {
		_ = disp.Close()
	})
	defer app.Close()
	app.BindPointer(disp.FirstSeat(), disp.CursorShape())
	if err := app.ListenCommands(barSocket); err != nil {
		log.Printf("Command socket error: %v", err)
	}

	disp.OnOutput(func(output *display.Output, added bool) {
		app.SyncOutputs(disp)
	})

	app.StartDDCMonitor(disp)
	app.SyncOutputs(disp)

	log.Println("client bar running")

	if err := disp.Run(); err != nil && !app.ShuttingDown() {
		log.Printf("Dispatch error: %v", err)
	}
}

type App struct {
	mu            sync.Mutex
	bars          map[*client.WlOutput]*Bar
	listener      net.Listener
	socketPath    string
	audio         *AudioService
	network       *NetworkService
	bluetooth     *BluetoothService
	niri          *NiriService
	icons         *IconResolver
	notifications *NotificationService
	ddc           *DDCMonitor
	shutdown      func()
	shutdownOnce  sync.Once
	shuttingDown  bool
	closed        bool
}

func NewApp(shutdown func()) *App {
	return &App{
		bars:          make(map[*client.WlOutput]*Bar),
		audio:         NewAudioService(),
		network:       NewNetworkService(),
		bluetooth:     NewBluetoothService(),
		niri:          NewNiriService(),
		icons:         NewIconResolver(),
		notifications: NewNotificationService(),
		shutdown:      shutdown,
	}
}

func (a *App) Close() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true

	listener := a.listener
	a.listener = nil
	socketPath := a.socketPath
	a.socketPath = ""

	bars := make([]*Bar, 0, len(a.bars))
	for output, bar := range a.bars {
		bars = append(bars, bar)
		delete(a.bars, output)
	}

	audio := a.audio
	a.audio = nil
	network := a.network
	a.network = nil
	bluetooth := a.bluetooth
	a.bluetooth = nil
	niri := a.niri
	a.niri = nil
	notifications := a.notifications
	a.notifications = nil
	ddc := a.ddc
	a.ddc = nil
	a.mu.Unlock()

	if ddc != nil {
		ddc.Close()
	}

	if listener != nil {
		_ = listener.Close()
	}
	if socketPath != "" {
		_ = os.Remove(socketPath)
	}
	for _, bar := range bars {
		bar.Close()
	}
	if audio != nil {
		audio.Close()
	}
	if network != nil {
		network.Close()
	}
	if bluetooth != nil {
		bluetooth.Close()
	}
	if niri != nil {
		niri.Close()
	}
	if notifications != nil {
		notifications.Close()
	}
}

func (a *App) ListenCommands(path string) error {
	_ = os.Remove(path)

	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	a.listener = ln
	a.socketPath = path

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
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	if cmd == "kill" {
		a.requestShutdown()
		return
	}
	if cmd == "notif-toggle" {
		for _, bar := range a.bars {
			if bar.flyout != nil {
				anchorX := int32(barSidePad)
				if bar.output != nil && bar.output.ModeWidth > flyoutWidth {
					anchorX = int32((bar.output.ModeWidth - flyoutWidth) / 2)
				}
				bar.flyout.Toggle(anchorX)
			}
		}
		return
	}
	if cmd == "vol-toggle" {
		for _, bar := range a.bars {
			if bar.volFlyout != nil {
				rect := image.Rect(1100, 0, 1160, barHeight)
				bar.drawMu.RLock()
				for _, hit := range bar.hits {
					if _, ok := hit.mod.(*VolumeModule); ok {
						rect = hit.rect
						break
					}
				}
				bar.drawMu.RUnlock()
				bar.volFlyout.Toggle(rect)
			}
		}
		return
	}
	if cmd == "wifi-toggle" {
		for _, bar := range a.bars {
			if bar.wifiFlyout != nil {
				rect := image.Rect(1200, 0, 1260, barHeight)
				bar.drawMu.RLock()
				for _, hit := range bar.hits {
					if _, ok := hit.mod.(*WifiModule); ok {
						rect = hit.rect
						break
					}
				}
				bar.drawMu.RUnlock()
				bar.wifiFlyout.Toggle(rect)
			}
		}
		return
	}
	if strings.HasPrefix(cmd, "notif-scroll ") {
		var delta float64
		_, _ = fmt.Sscanf(strings.TrimPrefix(cmd, "notif-scroll "), "%f", &delta)
		for _, bar := range a.bars {
			if bar.flyout != nil {
				bar.flyout.ScrollBy(delta)
			}
		}
		return
	}
	if strings.HasPrefix(cmd, "dock-hover ") {
		var idx int
		_, _ = fmt.Sscanf(strings.TrimPrefix(cmd, "dock-hover "), "%d", &idx)
		for _, bar := range a.bars {
			if bar.dock != nil {
				bar.dock.SimulateHover(idx)
			}
		}
		return
	}
	if cmd == "dock-autohide" || strings.HasPrefix(cmd, "dock-autohide ") {
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "dock-autohide"))
		a.mu.Lock()
		bars := make([]*Bar, 0, len(a.bars))
		for _, bar := range a.bars {
			bars = append(bars, bar)
		}
		a.mu.Unlock()

		for _, bar := range bars {
			if bar.dock == nil {
				continue
			}
			switch strings.ToLower(arg) {
			case "on", "1", "true":
				bar.dock.SetAutoHide(true)
			case "off", "0", "false":
				bar.dock.SetAutoHide(false)
			default:
				bar.dock.ToggleAutoHide()
			}
		}
		return
	}
	if strings.HasPrefix(cmd, "bar-hover ") {
		var idx int
		_, _ = fmt.Sscanf(strings.TrimPrefix(cmd, "bar-hover "), "%d", &idx)
		for _, bar := range a.bars {
			bar.SimulateHoverModule(idx)
		}
		return
	}
	if strings.HasPrefix(cmd, "notif-hover ") {
		var hx, hy int
		_, _ = fmt.Sscanf(strings.TrimPrefix(cmd, "notif-hover "), "%d %d", &hx, &hy)
		for _, bar := range a.bars {
			if bar.flyout != nil {
				bar.flyout.SimulateHover(hx, hy)
			}
		}
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, bar := range a.bars {
		bar.HandleCommand(cmd)
	}
}

func (a *App) ShuttingDown() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.shuttingDown
}

func (a *App) requestShutdown() {
	a.shutdownOnce.Do(func() {
		a.mu.Lock()
		a.shuttingDown = true
		listener := a.listener
		a.listener = nil
		socketPath := a.socketPath
		a.socketPath = ""
		shutdown := a.shutdown
		a.mu.Unlock()

		if listener != nil {
			_ = listener.Close()
		}
		if socketPath != "" {
			_ = os.Remove(socketPath)
		}
		if shutdown != nil {
			shutdown()
		}
	})
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
		bars := make([]*Bar, 0, len(a.bars))
		for _, bar := range a.bars {
			bars = append(bars, bar)
		}
		a.mu.Unlock()

		for _, bar := range bars {
			bar.HandlePointerEnter(ev, cursorDevice)
		}
	})
	pointer.OnLeave(func(ev client.WlPointerLeaveEvent) {
		a.mu.Lock()
		bars := make([]*Bar, 0, len(a.bars))
		for _, bar := range a.bars {
			bars = append(bars, bar)
		}
		a.mu.Unlock()

		for _, bar := range bars {
			bar.HandlePointerLeave(ev, cursorDevice)
		}
	})
	pointer.OnMotion(func(ev client.WlPointerMotionEvent) {
		a.mu.Lock()
		bars := make([]*Bar, 0, len(a.bars))
		for _, bar := range a.bars {
			bars = append(bars, bar)
		}
		a.mu.Unlock()

		for _, bar := range bars {
			bar.HandlePointerMotion(ev, cursorDevice)
		}
	})
	pointer.OnButton(func(ev client.WlPointerButtonEvent) {
		a.mu.Lock()
		bars := make([]*Bar, 0, len(a.bars))
		for _, bar := range a.bars {
			bars = append(bars, bar)
		}
		a.mu.Unlock()

		for _, bar := range bars {
			bar.HandlePointerButton(ev)
		}
	})
	pointer.OnAxis(func(ev client.WlPointerAxisEvent) {
		a.mu.Lock()
		bars := make([]*Bar, 0, len(a.bars))
		for _, bar := range a.bars {
			bars = append(bars, bar)
		}
		a.mu.Unlock()

		for _, bar := range bars {
			bar.HandlePointerAxis(ev)
		}
	})
}

func (a *App) StartDDCMonitor(disp *display.Display) {
	a.mu.Lock()
	if a.ddc != nil {
		a.mu.Unlock()
		return
	}
	a.ddc = NewDDCMonitor(disp, func() {
		a.SyncOutputs(disp)
	})
	a.mu.Unlock()
}

func pickPrimaryOutput(outputs []*display.Output, ddc *DDCMonitor) *display.Output {
	var best *display.Output
	var bestArea int64 = -1
	for _, o := range outputs {
		if o == nil {
			continue
		}
		if ddc != nil && ddc.IsPoweredOff(o.Name) {
			continue
		}
		area := int64(o.ModeWidth) * int64(o.ModeHeight)
		if best == nil || area > bestArea || (area == bestArea && !strings.HasPrefix(o.Name, "eDP") && strings.HasPrefix(best.Name, "eDP")) {
			best = o
			bestArea = area
		}
	}
	if best == nil && len(outputs) > 0 {
		return outputs[0]
	}
	return best
}

func (a *App) SyncOutputs(disp *display.Display) {
	ready := disp.ReadyOutputs()
	a.mu.Lock()
	ddc := a.ddc
	a.mu.Unlock()
	primary := pickPrimaryOutput(ready, ddc)
	readySet := make(map[*client.WlOutput]*display.Output, len(ready))
	for _, o := range ready {
		readySet[o.WlOutput()] = o
	}

	a.mu.Lock()
	var toClose []*Bar
	for wlOut, bar := range a.bars {
		out, stillReady := readySet[wlOut]
		wantTopBar := stillReady && primary != nil && out.WlOutput() == primary.WlOutput()
		if !stillReady || bar.showTopBar != wantTopBar {
			toClose = append(toClose, bar)
			delete(a.bars, wlOut)
		}
	}
	a.mu.Unlock()

	for _, bar := range toClose {
		bar.Close()
	}

	closeAllNotifs := func() {
		a.mu.Lock()
		bars := make([]*Bar, 0, len(a.bars))
		for _, b := range a.bars {
			bars = append(bars, b)
		}
		a.mu.Unlock()
		for _, b := range bars {
			if b.flyout != nil {
				b.flyout.Close()
			}
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	for _, output := range ready {
		if _, exists := a.bars[output.WlOutput()]; exists {
			continue
		}
		wantTopBar := primary != nil && output.WlOutput() == primary.WlOutput()
		bar, err := NewBar(disp, output, wantTopBar, a.audio, a.network, a.bluetooth, a.niri, a.icons, a.notifications, closeAllNotifs)
		if err != nil {
			log.Printf("Failed to create bar for %s: %v", output.Name, err)
			continue
		}
		a.bars[output.WlOutput()] = bar
		log.Printf("Configured output %s (topBar=%v)", output.Name, wantTopBar)
	}
}

type Bar struct {
	output        *display.Output
	showTopBar    bool
	surface       *surface.Surface
	layer         *shell.LayerSurface
	frame         *shm.Frame
	face          font.Face
	flyout        *NotificationFlyout
	volFlyout     *VolumeFlyout
	wifiFlyout    *WifiFlyout
	dock          *Dock
	leftModules   []Module
	centerModules []Module
	rightModules  []Module

	closeMu   sync.Mutex
	closed    bool
	stop      chan struct{}
	dirtyCh   chan struct{}
	drawMu    sync.RWMutex
	hits      []moduleHit
	hover     pointerState
	renders   uint64
	startTime int64 // UnixMilli at app start, for relative frame timestamps
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

func NewBar(disp *display.Display, output *display.Output, showTopBar bool, audio *AudioService, network *NetworkService, bluetooth *BluetoothService, niri *NiriService, icons *IconResolver, notifications *NotificationService, closeAllNotifs func()) (*Bar, error) {
	b := &Bar{
		output:     output,
		showTopBar: showTopBar,
		face:       loadBarFont(output),
		stop:       make(chan struct{}),
		dirtyCh:    make(chan struct{}, 1),
		startTime:  time.Now().UnixMilli(),
	}

	dock, err := NewDock(disp, output, niri, icons)
	if err != nil {
		log.Printf("Failed to create dock for %s: %v", output.Name, err)
	} else {
		b.dock = dock
	}

	if !showTopBar {
		surf, err := surface.New(disp.Compositor())
		if err == nil {
			layer, err := shell.NewLayer(surf, disp.LayerShell(), shell.LayerConfig{
				Layer:         shell.LayerPositionTop,
				Name:          "cheese-top-spacer",
				Anchor:        shell.AnchorTop | shell.AnchorLeft | shell.AnchorRight,
				Width:         0,
				Height:        1,
				ExclusiveZone: barTopMargin,
				Output:        output.WlOutput(),
			})
			if err == nil {
				frame, err := shm.NewFrame(disp.Shm(), layer, disp, shm.FrameConfig{
					Format:  client.WlShmFormatArgb8888,
					Buffers: 1,
				})
				if err == nil {
					frame.SetRender(func(w, h int, t uint32, pixels []byte) {
						if reg, err := disp.Compositor().CreateRegion(); err == nil {
							_ = surf.WlSurface().SetInputRegion(reg)
							_ = reg.Destroy()
						}
						atomic.AddUint64(&b.renders, 1)
					})
					frame.SetManualMode(true)
					b.surface = surf
					b.layer = layer
					b.frame = frame
					go b.renderLoop()
					b.markDirty()
				}
			}
		}
		return b, nil
	}

	surf, err := surface.New(disp.Compositor())
	if err != nil {
		if b.dock != nil {
			b.dock.Close()
		}
		return nil, err
	}

	layer, err := shell.NewLayer(surf, disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          barName,
		Anchor:        shell.AnchorTop | shell.AnchorLeft | shell.AnchorRight,
		Width:         0,
		Height:        barHeight,
		ExclusiveZone: barHeight + barTopMargin,
		MarginTop:     barTopMargin,
		Output:        output.WlOutput(),
	})
	if err != nil {
		_ = surf.Close()
		if b.dock != nil {
			b.dock.Close()
		}
		return nil, err
	}

	frame, err := shm.NewFrame(disp.Shm(), layer, disp, shm.FrameConfig{
		Format:  client.WlShmFormatArgb8888,
		Buffers: 2,
	})
	if err != nil {
		_ = layer.Close()
		_ = surf.Close()
		if b.dock != nil {
			b.dock.Close()
		}
		return nil, err
	}

	b.surface = surf
	b.layer = layer
	b.frame = frame

	flyout := NewNotificationFlyout(disp, output, notifications, icons, closeAllNotifs)
	b.flyout = flyout
	volFlyout := NewVolumeFlyout(disp, output, audio)
	b.volFlyout = volFlyout
	wifiFlyout := NewWifiFlyout(disp, output, network)
	b.wifiFlyout = wifiFlyout

	b.leftModules = nil
	b.centerModules = []Module{
		NewClockModule(flyout),
	}
	b.rightModules = []Module{
		NewInputMethodModule(),
		NewMicModule(audio),
		NewVolumeModule(audio, volFlyout),
		NewBluetoothModule(bluetooth),
		NewWifiModule(network, wifiFlyout),
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
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()

	pending := false
	for {
		select {
		case <-b.dirtyCh:
			pending = true
		case <-ticker.C:
		case <-b.stop:
			return
		}

		if !pending || !b.frame.Ready() {
			continue
		}
		if b.tryRender() {
			pending = false
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

	if b.flyout != nil {
		b.flyout.Destroy()
	}
	if b.volFlyout != nil {
		b.volFlyout.Destroy()
	}
	if b.wifiFlyout != nil {
		b.wifiFlyout.Destroy()
	}
	if b.dock != nil {
		b.dock.Close()
	}

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
	if b.volFlyout != nil {
		b.volFlyout.SetBarWidth(width)
	}
	if b.wifiFlyout != nil {
		b.wifiFlyout.SetBarWidth(width)
	}
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
	atomic.AddUint64(&b.renders, 1)
}

func (b *Bar) tryRender() bool {
	before := atomic.LoadUint64(&b.renders)
	b.frame.ManualRender(uint32(time.Now().UnixMilli() - b.startTime))
	return atomic.LoadUint64(&b.renders) != before
}

func (b *Bar) drawCentered(dst draw.Image, rect image.Rectangle, mods []Module, hits []moduleHit) []moduleHit {
	totalWidth := modulesWidth(mods, b.face, moduleGap)
	x := rect.Min.X + (rect.Dx()-totalWidth)/2
	for i, mod := range mods {
		w := mod.Width(b.face)
		moduleRect := image.Rect(x, rect.Min.Y, x+w, rect.Max.Y)
		if !moduleDrawsOwnBackground(mod) {
			drawModuleBackground(dst, moduleRect)
		}
		if b.isModuleActive(mod) {
			drawActiveBackground(dst, moduleRect)
		}
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
		if !moduleDrawsOwnBackground(mod) {
			drawModuleBackground(dst, moduleRect)
		}
		if b.isModuleActive(mod) {
			drawActiveBackground(dst, moduleRect)
		}
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
	if b.flyout != nil {
		b.flyout.HandlePointerEnter(ev, cursor)
	}
	if b.volFlyout != nil {
		b.volFlyout.HandlePointerEnter(ev, cursor)
	}
	if b.wifiFlyout != nil {
		b.wifiFlyout.HandlePointerEnter(ev, cursor)
	}
	if b.dock != nil {
		b.dock.HandlePointerEnter(ev, cursor)
	}

	if b.surface == nil || ev.Surface != b.surface.WlSurface() {
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
	if b.flyout != nil {
		b.flyout.HandlePointerLeave(ev, cursor)
	}
	if b.volFlyout != nil {
		b.volFlyout.HandlePointerLeave(ev, cursor)
	}
	if b.wifiFlyout != nil {
		b.wifiFlyout.HandlePointerLeave(ev, cursor)
	}
	if b.dock != nil {
		b.dock.HandlePointerLeave(ev, cursor)
	}

	if b.surface == nil || ev.Surface != b.surface.WlSurface() {
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
	if b.flyout != nil {
		b.flyout.HandlePointerMotion(ev, cursor)
	}
	if b.volFlyout != nil {
		b.volFlyout.HandlePointerMotion(ev, cursor)
	}
	if b.wifiFlyout != nil {
		b.wifiFlyout.HandlePointerMotion(ev, cursor)
	}
	if b.dock != nil {
		b.dock.HandlePointerMotion(ev, cursor)
	}

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
	if b.flyout != nil && b.flyout.HandlePointerButton(ev) {
		return
	}
	if b.volFlyout != nil && b.volFlyout.HandlePointerButton(ev) {
		return
	}
	if b.wifiFlyout != nil && b.wifiFlyout.HandlePointerButton(ev) {
		return
	}
	if b.dock != nil && b.dock.HandlePointerButton(ev) {
		if ev.State == client.WlPointerButtonStatePressed {
			if b.flyout != nil && b.flyout.IsOpen() {
				b.flyout.Close()
			}
			if b.volFlyout != nil && b.volFlyout.IsOpen() {
				b.volFlyout.Close()
			}
			if b.wifiFlyout != nil && b.wifiFlyout.IsOpen() {
				b.wifiFlyout.Close()
			}
		}
		return
	}

	if ev.State != client.WlPointerButtonStatePressed {
		return
	}

	b.drawMu.RLock()
	defer b.drawMu.RUnlock()
	if !b.hover.inside {
		return
	}

	point := image.Pt(b.hover.x, b.hover.y)
	var clickedMod Module
	for _, hit := range b.hits {
		if point.In(hit.rect) {
			clickedMod = hit.mod
			break
		}
	}

	if _, isClock := clickedMod.(*ClockModule); !isClock && b.flyout != nil && b.flyout.IsOpen() {
		b.flyout.Close()
	}
	if _, isVol := clickedMod.(*VolumeModule); !isVol && b.volFlyout != nil && b.volFlyout.IsOpen() {
		b.volFlyout.Close()
	}
	if _, isWifi := clickedMod.(*WifiModule); !isWifi && b.wifiFlyout != nil && b.wifiFlyout.IsOpen() {
		b.wifiFlyout.Close()
	}

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

func (b *Bar) HandlePointerAxis(ev client.WlPointerAxisEvent) {
	if b.flyout != nil {
		b.flyout.HandlePointerAxis(ev)
	}
	if b.volFlyout != nil {
		b.volFlyout.HandlePointerAxis(ev)
	}
	if b.wifiFlyout != nil {
		b.wifiFlyout.HandlePointerAxis(ev)
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
	if rgba, ok := dst.(*image.RGBA); ok {
		drawSmoothRoundedRect(rgba, rect, float64(moduleCornerRadius), color.RGBA{R: 0x36, G: 0x36, B: 0x36, A: 0xff}, draw.Src)
		return
	}
	drawRoundedRect(dst, rect, moduleCornerRadius, color.RGBA{R: 0x36, G: 0x36, B: 0x36, A: 0xff}, draw.Src)
}

func drawActiveBackground(dst draw.Image, rect image.Rectangle) {
	if rgba, ok := dst.(*image.RGBA); ok {
		drawSmoothRoundedRect(rgba, rect, float64(moduleCornerRadius), color.RGBA{R: 0x2a, G: 0x2a, B: 0x2a, A: 0xff}, draw.Src)
		return
	}
	drawRoundedRect(dst, rect, moduleCornerRadius, color.RGBA{R: 0x2a, G: 0x2a, B: 0x2a, A: 0xff}, draw.Src)
}

func drawModuleBackground(dst draw.Image, rect image.Rectangle) {
	if rgba, ok := dst.(*image.RGBA); ok {
		drawSmoothRoundedRect(rgba, rect, float64(moduleCornerRadius), color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xff}, draw.Src)
		return
	}
	drawRoundedRect(dst, rect, moduleCornerRadius, color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xff}, draw.Src)
}

func (b *Bar) isModuleActive(mod Module) bool {
	if _, ok := mod.(*ClockModule); ok && b.flyout != nil {
		return b.flyout.IsOpen()
	}
	if _, ok := mod.(*VolumeModule); ok && b.volFlyout != nil {
		return b.volFlyout.IsOpen()
	}
	if _, ok := mod.(*WifiModule); ok && b.wifiFlyout != nil {
		return b.wifiFlyout.IsOpen()
	}
	return false
}

func (b *Bar) SimulateHoverModule(idx int) {
	b.drawMu.Lock()
	if idx < 0 || idx >= len(b.hits) {
		b.hover = pointerState{}
		b.drawMu.Unlock()
		b.markDirty()
		return
	}
	hit := b.hits[idx]
	b.hover.inside = true
	b.hover.x = (hit.rect.Min.X + hit.rect.Max.X) / 2
	b.hover.y = (hit.rect.Min.Y + hit.rect.Max.Y) / 2
	b.updateHoveredModuleLocked()
	b.drawMu.Unlock()
	b.markDirty()
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
		if w <= 0 {
			continue
		}
		moduleRect := image.Rect(x, rect.Min.Y, x+w, rect.Max.Y)
		if !moduleDrawsOwnBackground(mod) {
			drawModuleBackground(dst, moduleRect)
		}
		if b.isModuleActive(mod) {
			drawActiveBackground(dst, moduleRect)
		}
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

func moduleDrawsOwnBackground(mod Module) bool {
	if controlled, ok := mod.(ModuleBackgroundControl); ok {
		return controlled.DrawOwnBackground()
	}
	return false
}
