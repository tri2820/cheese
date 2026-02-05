package ui

import (
	"image/color"
	"sync"
)

// DrawCommand represents a single draw operation.
type DrawCommand interface {
	Execute(pixels []byte, stride, width, height int)
}

// DrawRect fills a rectangle with a solid color.
type DrawRect struct {
	X, Y, W, H int
	Color      color.RGBA
}

// Execute fills the rectangle area with the color in ARGB8888 format.
func (d DrawRect) Execute(pixels []byte, stride, width, height int) {
	// Clamp bounds to surface
	if d.X < 0 {
		d.W += d.X
		d.X = 0
	}
	if d.Y < 0 {
		d.H += d.Y
		d.Y = 0
	}
	if d.X+d.W > width {
		d.W = width - d.X
	}
	if d.Y+d.H > height {
		d.H = height - d.Y
	}
	if d.W <= 0 || d.H <= 0 {
		return
	}

	// ARGB8888 format: B,G,R,A on little-endian
	bgra := []byte{d.Color.B, d.Color.G, d.Color.R, d.Color.A}

	for y := d.Y; y < d.Y+d.H; y++ {
		row := pixels[y*stride:]
		for x := d.X; x < d.X+d.W; x++ {
			copy(row[x*4:], bgra)
		}
	}
}

// CommandList holds draw commands for a frame.
type CommandList struct {
	cmds []DrawCommand
	mu   sync.Mutex
}

// Add appends a draw command to the list.
func (cl *CommandList) Add(cmd DrawCommand) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.cmds = append(cl.cmds, cmd)
}

// Execute runs all draw commands in order.
func (cl *CommandList) Execute(pixels []byte, stride, width, height int) {
	cl.mu.Lock()
	cmds := cl.cmds
	cl.mu.Unlock()

	for _, cmd := range cmds {
		cmd.Execute(pixels, stride, width, height)
	}
}

// Clear removes all commands from the list.
func (cl *CommandList) Clear() {
	cl.mu.Lock()
	cl.cmds = cl.cmds[:0]
	cl.mu.Unlock()
}
