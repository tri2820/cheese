package main

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"sync"
	"time"

	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/shm"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	barHeight = 28
	barName   = "cheese-client-bar"
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
	output  *display.Output
	surface *surface.Surface
	layer   *shell.LayerSurface
	frame   *shm.Frame
	face    font.Face

	closeMu sync.Mutex
	closed  bool
	stop    chan struct{}
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
		face:    basicfont.Face7x13,
		stop:    make(chan struct{}),
	}

	frame.SetRender(func(width, height int, frameTime uint32, pixels []byte) {
		b.draw(width, height, pixels, time.UnixMilli(int64(frameTime)))
	})
	frame.SetManualMode(true)
	frame.OnResize(func(width, height int) {
		b.frame.ManualRender(uint32(time.Now().UnixMilli()))
	})
	frame.OnError(func(err error) {
		log.Printf("Bar frame error on %s: %v", output.Name, err)
	})

	layer.OnClose(func() {
		b.Close()
	})

	go b.tick()

	return b, nil
}

func (b *Bar) tick() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	b.frame.ManualRender(uint32(time.Now().UnixMilli()))

	for {
		select {
		case t := <-ticker.C:
			if b.frame.Ready() {
				b.frame.ManualRender(uint32(t.UnixMilli()))
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

func (b *Bar) draw(width, height int, pixels []byte, now time.Time) {
	dst := &image.RGBA{
		Pix:    pixels,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}

	draw.Draw(dst, dst.Rect, image.NewUniform(argb(0x00, 0x00, 0x00, 0xff)), image.Point{}, draw.Src)

	timeText := now.Format("15:04:05")
	textWidth := font.MeasureString(b.face, timeText).Round()
	metrics := b.face.Metrics()
	textX := (width - textWidth) / 2
	textY := (height + metrics.Ascent.Ceil() - metrics.Descent.Ceil()) / 2

	drawer := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(argb(0xff, 0xff, 0xff, 0xff)),
		Face: b.face,
		Dot:  fixed.P(textX, textY),
	}
	drawer.DrawString(timeText)
}

func argb(r, g, b, a uint8) color.RGBA {
	return color.RGBA{R: b, G: g, B: r, A: a}
}
