package ui

import (
	"image/color"
	"strconv"

	"github.com/tri2820/cheese/signals"
)

// Frame is a container widget that extends Element with styling and child management.
type Frame struct {
	*Element                  // Embedding: Frame IS-A Element
	Color   signals.Signal[string] // Background color (e.g., "#FF0000")
	layout  *Layout               // Reference to layout for command registration
}

// NewFrame creates a new Frame widget.
func (l *Layout) NewFrame() *Frame {
	f := &Frame{
		Element: l.NewElement(),
		Color:   signals.New("#FFFFFF"), // Default white
		layout:  l,
	}

	// Create effect that runs when bounds or color change
	signals.Effect(func() {
		x := int(f.Left.Get())
		y := int(f.Top.Get())
		w := int(f.Right.Get() - f.Left.Get())
		h := int(f.Bottom.Get() - f.Top.Get())

		// Skip drawing if bounds are invalid (happens during initial setup)
		if w <= 0 || h <= 0 {
			return
		}

		c := parseColor(f.Color.Get())

		if l.cmdList != nil {
			l.cmdList.Add(DrawRect{X: x, Y: y, W: w, H: h, Color: c})
		}

		l.RequestRender()
	}, f.Left, f.Top, f.Right, f.Bottom, f.Color)

	return f
}

// parseColor parses hex color string (e.g., "#FF0000" or "#FF0000FF") to color.RGBA.
// Supports #RGB, #RGBA, #RRGGBB, #RRGGBBAA formats.
// NOTE: Returns color with R and B swapped for ARGB8888 format compatibility.
func parseColor(s string) color.RGBA {
	if len(s) < 2 || s[0] != '#' {
		return color.RGBA{A: 255} // Default opaque black
	}

	hex := s[1:]
	var r, g, b, a uint8
	a = 255

	switch len(hex) {
	case 3: // #RGB
		r = parseHex(hex[0]) * 17
		g = parseHex(hex[1]) * 17
		b = parseHex(hex[2]) * 17
	case 4: // #RGBA
		r = parseHex(hex[0]) * 17
		g = parseHex(hex[1]) * 17
		b = parseHex(hex[2]) * 17
		a = parseHex(hex[3]) * 17
	case 6: // #RRGGBB
		r = parseHexPair(hex[0:2])
		g = parseHexPair(hex[2:4])
		b = parseHexPair(hex[4:6])
	case 8: // #RRGGBBAA
		r = parseHexPair(hex[0:2])
		g = parseHexPair(hex[2:4])
		b = parseHexPair(hex[4:6])
		a = parseHexPair(hex[6:8])
	}

	// Swap R and B for ARGB8888 format (memory: B,G,R,A on little-endian)
	// When color.RGBA is written to memory, it becomes R,G,B,A
	// We want B,G,R,A in memory for ARGB8888, so we swap R and B in the struct
	return color.RGBA{R: b, G: g, B: r, A: a}
}

func parseHex(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0
	}
}

func parseHexPair(s string) uint8 {
	val, _ := strconv.ParseUint(s, 16, 8)
	return uint8(val)
}
