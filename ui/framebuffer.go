package ui

import "image"

// Framebuffer represents a pixel buffer that drawables can draw into.
// The origin (0,0) is at the visible region of the drawable in the frame.
type Framebuffer struct {
	pixels []byte
	stride int
	width  int
	height int
}

// SetPixel sets a single pixel at the given coordinates.
func (fb Framebuffer) SetPixel(x, y int, r, g, b, a uint8) {
	if x < 0 || x >= fb.width || y < 0 || y >= fb.height {
		return
	}
	offset := y*fb.stride + x*4
	fb.pixels[offset] = b
	fb.pixels[offset+1] = g
	fb.pixels[offset+2] = r
	fb.pixels[offset+3] = a
}

// Width returns the framebuffer width.
func (fb Framebuffer) Width() int { return fb.width }

// Height returns the framebuffer height.
func (fb Framebuffer) Height() int { return fb.height }

// AsImage returns an image.RGBA view of the framebuffer for use with image libraries.
// The returned image shares the same underlying pixel buffer (no copy).
func (fb Framebuffer) AsImage() *image.RGBA {
	return &image.RGBA{
		Pix:    fb.pixels,
		Stride: fb.stride,
		Rect:   image.Rect(0, 0, fb.width, fb.height),
	}
}
