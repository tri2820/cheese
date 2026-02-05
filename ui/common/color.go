package common

import (
	"image/color"
	"strconv"
)

// ParseColor parses hex color string (e.g., "#FF0000" or "#FF0000FF") to color.RGBA.
// Supports #RGB, #RGBA, #RRGGBB, #RRGGBBAA formats.
// NOTE: Returns color with R and B swapped for ARGB8888 format compatibility.
func ParseColor(s string) color.RGBA {
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
