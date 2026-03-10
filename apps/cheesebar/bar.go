package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/tri2820/cheese/apps/common"
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/shm"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Bar represents a single status bar on one monitor
type Bar struct {
	disp       *display.Display
	output     *display.Output
	surface    *surface.Surface
	layer      *shell.LayerSurface
	frame      *shm.Frame
	fontFace   font.Face
	icon       image.Image
	iconRegion *client.WlRegion // Input region for the icon
	closeMu    sync.Mutex
	closed     bool
	stop       chan struct{}

	// Hover state
	hovered bool
	hoverMu sync.Mutex

	// Mouse position (surface-local coordinates)
	mouseX  float64
	mouseY  float64
	mouseMu sync.Mutex
}

func NewBar(disp *display.Display, output *display.Output) (*Bar, error) {
	dpi := output.DPI()
	if dpi == 0 {
		dpi = 96 // fallback
		log.Printf("Output %s has no DPI info, using default 96", output.Name)
	} else {
		log.Printf("Creating bar for output: %s (%.1f DPI)", output.Name, dpi)
	}

	// Create surface
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		return nil, fmt.Errorf("failed to create surface: %w", err)
	}

	MAX_HEIGHT := 100

	// Load font
	fontFace, err := common.LoadFont("Liberation Sans", 12, dpi)
	if err != nil {
		return nil, fmt.Errorf("failed to load TTF font: %w", err)
	}

	// Load icon (resize to 16px height for 24px bar)
	icon, err := loadIcon("/home/tri/cheese/apps/cheesebar/wikipedia.png", MAX_HEIGHT)
	if err != nil {
		log.Printf("Warning: failed to load icon: %v", err)
	}

	// Create input region for the icon (will be configured after layer setup)
	var iconRegion *client.WlRegion
	if icon != nil {
		iconRegion, err = disp.Compositor().CreateRegion()
		if err != nil {
			surf.Close()
			return nil, fmt.Errorf("failed to create region: %w", err)
		}
		// Icon is at x=8, height spans full bar height
		iconBounds := icon.Bounds()
		iconWidth := int32(iconBounds.Dx())
		iconHeight := int32(MAX_HEIGHT) // full bar height
		if err := iconRegion.Add(8, 0, iconWidth, iconHeight); err != nil {
			iconRegion.Destroy()
			surf.Close()
			return nil, fmt.Errorf("failed to add region: %w", err)
		}
	}

	// Create layer surface (bar at top of screen) for this specific outpuz
	barLayer, err := shell.NewLayer(surf, disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          "cheesebar",
		Anchor:        shell.AnchorTop | shell.AnchorLeft | shell.AnchorRight,
		Width:         0,                  // 0 = full width
		Height:        uint32(MAX_HEIGHT), // Fixed height
		ExclusiveZone: 24,
		Output:        output.WlOutput(), // Bind to this specific output
	})
	if err != nil {
		surf.Close()
		return nil, fmt.Errorf("failed to create layer surface: %w", err)
	}

	b := &Bar{
		disp:       disp,
		output:     output,
		surface:    surf,
		layer:      barLayer,
		fontFace:   fontFace,
		icon:       icon,
		iconRegion: iconRegion,
		stop:       make(chan struct{}),
	}

	// Create frame with ARGB8888
	frame, err := shm.NewFrame(disp.Shm(), barLayer, disp, shm.FrameConfig{
		Format:  client.WlShmFormatArgb8888,
		Buffers: 2,
	})
	if err != nil {
		barLayer.Close()
		surf.Close()
		return nil, fmt.Errorf("failed to create frame: %w", err)
	}
	b.frame = frame

	frame.OnError(func(err error) {
		log.Printf("bar frame error on %s: %v", output.Name, err)
	})

	// Set the render delegate BEFORE enabling manual mode
	frame.SetRender(func(w, h int, frameTime uint32, pixels []byte) {
		b.draw(w, h, pixels, time.UnixMilli(int64(frameTime)))
	})

	// Enable manual mode AFTER setting the render delegate (so configure events work)
	frame.SetManualMode(true)

	// Set up close handler
	barLayer.OnClose(func() {
		b.Close()
	})

	// Set up input region (icon only, bar is click-through)
	// This must be done after the layer surface is configured
	if b.iconRegion != nil {
		if err := surf.WlSurface().SetInputRegion(b.iconRegion); err != nil {
			log.Printf("Warning: failed to set input region: %v", err)
		}
	}

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
	for !b.frame.Ready() {
		time.Sleep(100 * time.Millisecond)
	}
	b.frame.ManualRender(uint32(time.Now().UnixMilli()))

	for {
		select {
		case <-ticker.C:
			b.frame.ManualRender(uint32(time.Now().UnixMilli()))
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

	if b.frame != nil {
		b.frame.Close()
	}
	if b.iconRegion != nil {
		b.iconRegion.Destroy()
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

	// Draw icon on the left side (vertically centered)
	if b.icon != nil {
		iconX := 8
		iconY := (height - b.icon.Bounds().Dy()) / 2

		b.hoverMu.Lock()
		hovered := b.hovered
		b.hoverMu.Unlock()

		var iconToDraw image.Image = b.icon
		if hovered {
			// Apply color tint (brighten) when hovered
			iconToDraw = b.tintIcon(b.icon, 1.3)
		}

		draw.Draw(dstImg,
			image.Rect(iconX, iconY, iconX+iconToDraw.Bounds().Dx(), iconY+iconToDraw.Bounds().Dy()),
			iconToDraw,
			image.Point{},
			draw.Over)
	}

	// Calculate text offset (after icon or default margin)
	textX := 8
	if b.icon != nil {
		textX = 8 + b.icon.Bounds().Dx() + 8
	}

	// Draw time string (left-aligned, after icon)
	weekday := t.Weekday().String()[:3]
	timeStr := fmt.Sprintf("%s  %02d:%02d:%02d", weekday, t.Hour(), t.Minute(), t.Second())

	m := b.fontFace.Metrics()
	y := (height + m.Ascent.Ceil() - m.Descent.Ceil()) / 2

	// Draw text (white, fully opaque for clarity)
	drawer := &font.Drawer{
		Dst:  dstImg,
		Src:  image.NewUniform(argb(0xff, 0xff, 0xff, 0xff)),
		Face: b.fontFace,
		Dot:  fixed.Point26_6{X: fixed.I(textX), Y: fixed.I(y)},
	}
	drawer.DrawString(timeStr)

	// Draw distance from mouse to icon center
	if b.icon != nil {
		b.mouseMu.Lock()
		mouseX, mouseY := b.mouseX, b.mouseY
		b.mouseMu.Unlock()

		iconWidth := b.icon.Bounds().Dx()
		iconCenterX := float64(8 + iconWidth/2)
		iconCenterY := float64(height / 2)

		dx := mouseX - iconCenterX
		dy := mouseY - iconCenterY
		distance := math.Sqrt(dx*dx + dy*dy)

		// Position distance text after time string
		timeWidth := font.MeasureString(b.fontFace, timeStr).Round()
		distX := textX + timeWidth + 20

		distDrawer := &font.Drawer{
			Dst:  dstImg,
			Src:  image.NewUniform(argb(0xff, 0x80, 0x80, 0xff)), // Light red for distance
			Face: b.fontFace,
			Dot:  fixed.Point26_6{X: fixed.I(distX), Y: fixed.I(y)},
		}
		distDrawer.DrawString(fmt.Sprintf("%.0fpx", distance))
	}
}

// setHovered sets the hover state and triggers a re-render if changed.
func (b *Bar) setHovered(hovered bool) {
	b.hoverMu.Lock()
	changed := b.hovered != hovered
	b.hovered = hovered
	b.hoverMu.Unlock()

	if changed && b.frame.Ready() {
		b.frame.ManualRender(uint32(time.Now().UnixMilli()))
	}
}

// setMousePos updates the mouse position and triggers a re-render.
func (b *Bar) setMousePos(x, y float64) {
	b.mouseMu.Lock()
	b.mouseX = x
	b.mouseY = y
	b.mouseMu.Unlock()

	if b.frame.Ready() {
		b.frame.ManualRender(uint32(time.Now().UnixMilli()))
	}
}

// argb swaps R and B for ARGB8888 format (memory: B,G,R,A on little-endian)
func argb(r, g, b, a uint8) color.RGBA {
	return color.RGBA{R: b, G: g, B: r, A: a}
}

// tintIcon applies a color tint multiplier to an image.
// factor > 1.0 brightens, factor < 1.0 darkens.
func (b *Bar) tintIcon(img image.Image, factor float64) image.Image {
	bounds := img.Bounds()
	tinted := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			// RGBA returns pre-multiplied alpha values in range [0, 65535]
			// We need to work with non-premultiplied values
			if a > 0 {
				r = uint32(float64(r) * factor)
				g = uint32(float64(g) * factor)
				b = uint32(float64(b) * factor)
				// Clamp to max
				if r > 0xffff {
					r = 0xffff
				}
				if g > 0xffff {
					g = 0xffff
				}
				if b > 0xffff {
					b = 0xffff
				}
			}
			tinted.Set(x, y, color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a >> 8),
			})
		}
	}
	return tinted
}

// loadIcon loads and resizes an image to fit within the bar height
func loadIcon(path string, height int) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	// Calculate new size maintaining aspect ratio
	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	newWidth := srcW * height / srcH

	// Create resized image
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, srcBounds, draw.Over, nil)

	return dst, nil
}
