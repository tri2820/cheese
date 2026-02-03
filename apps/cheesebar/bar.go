package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"sync"
	"time"

	"github.com/tri2820/cheese/apps/common"
	"github.com/tri2820/cheese/client-toolkit/buffer"
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Bar represents a single status bar on one monitor
type Bar struct {
	disp     *display.Display
	output   *display.Output
	surface  *surface.Surface
	layer    *shell.LayerSurface
	renderer *buffer.Renderer
	fontFace font.Face
	closeMu  sync.Mutex
	closed   bool
	stop     chan struct{}
}

func NewBar(disp *display.Display, output *display.Output) (*Bar, error) {
	dpi := output.DPI()
	if dpi == 0 {
		dpi = 96 // fallback
		log.Printf("Output %s has no DPI info, using default 96", output.Name)
	} else {
		log.Printf("Creating bar for output: %s (%.1f DPI)", output.Name, dpi)
	}

	// Load font
	fontFace, err := common.LoadFont("Liberation Sans", 12, dpi)
	if err != nil {
		return nil, fmt.Errorf("failed to load TTF font: %w", err)
	}

	// Create surface
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		return nil, fmt.Errorf("failed to create surface: %w", err)
	}

	// Create layer surface (bar at top of screen) for this specific output
	barLayer, err := shell.NewLayer(surf, disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          "cheesebar",
		Anchor:        shell.AnchorTop | shell.AnchorLeft | shell.AnchorRight,
		Width:         0,  // 0 = full width
		Height:        24, // Fixed height
		ExclusiveZone: 24,
		Output:        output.WlOutput(), // Bind to this specific output
	})
	if err != nil {
		surf.Close()
		return nil, fmt.Errorf("failed to create layer surface: %w", err)
	}

	b := &Bar{
		disp:     disp,
		output:   output,
		surface:  surf,
		layer:    barLayer,
		fontFace: fontFace,
		stop:     make(chan struct{}),
	}

	// Create renderer with ARGB8888
	renderer, err := buffer.NewRenderer(buffer.RendererConfig{
		Shm:     disp.Shm(),
		Target:  barLayer,
		Format:  client.WlShmFormatArgb8888,
		Buffers: 2,
	})
	if err != nil {
		barLayer.Close()
		surf.Close()
		return nil, fmt.Errorf("failed to create renderer: %w", err)
	}
	b.renderer = renderer

	// Set up render callback BEFORE enabling manual mode
	renderer.OnRender(func(w, h int, frameTime uint32, pixels []byte) {
		b.draw(w, h, pixels, time.UnixMilli(int64(frameTime)))
	})

	// Enable manual mode AFTER setting up OnRender (so configure events work)
	renderer.SetManualMode(true)

	// Set up close handler
	barLayer.SetCloseHandler(func() {
		b.Close()
	})

	// Start update loop - render every 5 seconds
	go b.updateLoop()

	log.Printf("Bar created for output: %s", output.Name)
	return b, nil
}

// updateLoop runs in a goroutine and renders every 1 second
func (b *Bar) updateLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Wait for initial configure before starting
	for !b.renderer.Ready() {
		time.Sleep(100 * time.Millisecond)
	}
	b.renderer.ManualRender(uint32(time.Now().UnixMilli()))

	for {
		select {
		case <-ticker.C:
			b.renderer.ManualRender(uint32(time.Now().UnixMilli()))
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

	close(b.stop) // Stop the update loop

	log.Printf("Closing bar for output: %s", b.output.Name)

	if b.renderer != nil {
		b.renderer.Close()
	}
	if b.layer != nil {
		b.layer.Close()
	}
	if b.surface != nil {
		b.surface.Close()
	}
}

func (b *Bar) draw(width, height int, pixels []byte, t time.Time) {
	// Wrap pixels as RGBA image (we'll swap colors for ARGB format)
	dstImg := &image.RGBA{
		Pix:    pixels,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}

	// Draw background (dark gray, 50% transparent)
	draw.Draw(dstImg, dstImg.Rect, image.NewUniform(argb(0x30, 0x30, 0x30, 0x7f)), image.Point{}, draw.Src)

	// Draw time string centered
	weekday := t.Weekday().String()[:3]
	timeStr := fmt.Sprintf("%s  %02d:%02d:%02d", weekday, t.Hour(), t.Minute(), t.Second())

	// Measure and center text
	adv := font.MeasureString(b.fontFace, timeStr)
	x := max((width-adv.Ceil())/2, 0)
	m := b.fontFace.Metrics()
	y := (height + m.Ascent.Ceil() - m.Descent.Ceil()) / 2

	// Draw text (white, fully opaque for clarity)
	drawer := &font.Drawer{
		Dst:  dstImg,
		Src:  image.NewUniform(argb(0xff, 0xff, 0xff, 0xff)),
		Face: b.fontFace,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)},
	}
	drawer.DrawString(timeStr)
}

// argb swaps R and B for ARGB8888 format (memory: B,G,R,A on little-endian)
func argb(r, g, b, a uint8) color.RGBA {
	return color.RGBA{R: b, G: g, B: r, A: a}
}
