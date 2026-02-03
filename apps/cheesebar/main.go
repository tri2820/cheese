package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tri2820/cheese/client-toolkit/buffer"
	"github.com/tri2820/cheese/client-toolkit/display"
	"github.com/tri2820/cheese/client-toolkit/shell"
	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func main() {
	log.Println("Starting cheesebar...")

	// Load TTF font
	fontFace, err := loadFont("Arial", 14, 96)
	if err != nil {
		log.Fatalf("Failed to load TTF font: %v", err)
	}
	log.Println("Loaded TTF font successfully")

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
		Height:        24,
		ExclusiveZone: 24,
	})
	if err != nil {
		log.Fatal("Failed to create layer surface:", err)
	}

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

// loadFont loads a TTF font by name using fc-match
func loadFont(fontName string, size float64, dpi float64) (font.Face, error) {
	// Find font path with fc-match
	cmd := exec.Command("fc-match", "-f", "%{file}", fontName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fc-match failed: %w", err)
	}

	fontPath := strings.TrimSpace(string(output))
	if fontPath == "" {
		return nil, fmt.Errorf("fc-match returned empty path")
	}

	// Read font file
	fontBytes, err := os.ReadFile(fontPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read font file: %w", err)
	}

	// Parse the font
	f, err := opentype.Parse(fontBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse font file: %w", err)
	}

	// Create font face at specified size with hinting
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
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
	y := (height + m.Ascent.Ceil()) / 2

	// Draw text (white)
	drawer := &font.Drawer{
		Dst:  dstImg,
		Src:  image.NewUniform(xrgb(0xff, 0xff, 0xff)),
		Face: fontFace,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)},
	}
	drawer.DrawString(timeStr)
}
