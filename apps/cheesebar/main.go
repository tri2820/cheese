package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"time"

	"github.com/tri2820/cheese/client-toolkit/buffer"
	"github.com/tri2820/cheese/client-toolkit/display"
	toolkitfont "github.com/tri2820/cheese/client-toolkit/font"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

func main() {
	log.Println("Starting cheesebar...")

	// Connect to display with layer shell support
	disp := display.MustConnect(display.Config{
		Required: display.RequiredGlobals{
			Compositor: true,
			Shm:        true,
			LayerShell: true,
		},
	})

	// Check for XRGB format support
	if !disp.HasFormat(client.WlShmFormatXrgb8888) {
		log.Fatal("XRGB8888 not supported")
	}

	// Create surface
	surf, err := surface.New(disp.Compositor())
	if err != nil {
		log.Fatal("Failed to create surface:", err)
	}

	// Create layer surface (bar at top of screen)
	bar, err := shell.NewLayer(surf, disp.LayerShell(), shell.LayerConfig{
		Layer:         shell.LayerPositionTop,
		Name:          "cheesebar",
		Anchor:        shell.AnchorTop | shell.AnchorLeft | shell.AnchorRight,
		Width:         0, // 0 = full width
		Height:        28,
		ExclusiveZone: 28,
	})
	if err != nil {
		log.Fatal("Failed to create layer surface:", err)
	}

	// Wait for surface to enter an output, get DPI, and load font
	output := disp.GetOutputForSurface(surf)
	dpi := output.DPI()
	if dpi == 0 {
		dpi = 96 // fallback
		log.Printf("Output %s has no DPI info, using default 96", output.Name)
	} else {
		log.Printf("Surface entered output: %s (%.1f DPI)", output.Name, dpi)
	}

	fontFace, err := toolkitfont.LoadFont("Liberation Sans", 16, dpi)
	if err != nil {
		log.Fatalf("Failed to load TTF font: %v", err)
	}
	log.Println("Loaded TTF font successfully")

	log.Println("cheesebar running at top of screen")

	// Create renderer with XRGB8888
	renderer, err := buffer.NewRenderer(buffer.RendererConfig{
		Shm:     disp.Shm(),
		Target:  bar,
		Format:  buffer.FormatXRGB8888,
		Buffers: 2,
	})
	if err != nil {
		log.Fatal("Failed to create renderer:", err)
	}
	defer renderer.Close()

	// Set up render callback
	renderer.OnRender(func(w, h int, frameTime uint32, pixels []byte) {
		drawBar(w, h, pixels, time.Now(), fontFace)
	})

	// Run event loop
	if err := disp.Run(); err != nil {
		log.Printf("Dispatch error: %v", err)
	}
}

// xrgb swaps R and B for XRGB8888 format (which stores B,G,R,X in memory on little-endian)
func xrgb(r, g, b uint8) color.RGBA {
	return color.RGBA{R: b, G: g, B: r, A: 0xff}
}

func drawBar(width, height int, pixels []byte, t time.Time, fontFace font.Face) {
	// Wrap pixels as RGBA image (R/B swapped for XRGB8888)
	dstImg := &image.RGBA{
		Pix:    pixels,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}

	// Draw background (dark gray)
	draw.Draw(dstImg, dstImg.Rect, image.NewUniform(xrgb(0x30, 0x30, 0x30)), image.Point{}, draw.Src)

	// Draw time string centered
	weekday := t.Weekday().String()[:3]
	timeStr := fmt.Sprintf("%s  %02d:%02d:%02d", weekday, t.Hour(), t.Minute(), t.Second())

	// Measure and center text
	adv := font.MeasureString(fontFace, timeStr)
	x := (width - adv.Ceil()) / 2
	if x < 0 {
		x = 0
	}
	m := fontFace.Metrics()
	y := (height + m.Ascent.Ceil() - m.Descent.Ceil()) / 2

	// Draw text (white)
	drawer := &font.Drawer{
		Dst:  dstImg,
		Src:  image.NewUniform(xrgb(0xff, 0xff, 0xff)),
		Face: fontFace,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)},
	}
	drawer.DrawString(timeStr)
}
