package main

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"sync"
	"time"

	"github.com/tri2820/cheese/apps/common"
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/shm"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
)

const (
	barHeight  = 28
	barName    = "cheese-client-bar"
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
		},
	})

	if !disp.HasFormat(client.WlShmFormatArgb8888) {
		log.Fatal("ARGB8888 not supported")
	}

	app := NewApp()

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
	mu   sync.Mutex
	bars map[*client.WlOutput]*Bar
}

func NewApp() *App {
	return &App{
		bars: make(map[*client.WlOutput]*Bar),
	}
}

func (a *App) AddBar(disp *display.Display, output *display.Output) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.bars[output.WlOutput()]; ok {
		return
	}

	bar, err := NewBar(disp, output)
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
}

func NewBar(disp *display.Display, output *display.Output) (*Bar, error) {
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
		NewVolumeModule(output),
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

func (b *Bar) draw(width, height int, pixels []byte) {
	dst := &image.RGBA{
		Pix:    pixels,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}

	draw.Draw(dst, dst.Rect, image.NewUniform(argb(0x00, 0x00, 0x00, 0xff)), image.Point{}, draw.Src)

	b.drawCentered(dst, image.Rect(0, 0, width, height), b.centerModules)
	b.drawRight(dst, image.Rect(0, 0, width, height), b.rightModules)
}

func (b *Bar) drawCentered(dst draw.Image, rect image.Rectangle, mods []Module) {
	totalWidth := modulesWidth(mods, b.face, centerGap)
	x := rect.Min.X + (rect.Dx()-totalWidth)/2
	for i, mod := range mods {
		w := mod.Width(b.face)
		mod.Draw(dst, image.Rect(x, rect.Min.Y, x+w, rect.Max.Y), b.face)
		x += w
		if i < len(mods)-1 {
			x += centerGap
		}
	}
}

func (b *Bar) drawRight(dst draw.Image, rect image.Rectangle, mods []Module) {
	totalWidth := modulesWidth(mods, b.face, moduleGap)
	x := rect.Max.X - barSidePad - totalWidth
	for i, mod := range mods {
		w := mod.Width(b.face)
		mod.Draw(dst, image.Rect(x, rect.Min.Y, x+w, rect.Max.Y), b.face)
		x += w
		if i < len(mods)-1 {
			x += moduleGap
		}
	}
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
