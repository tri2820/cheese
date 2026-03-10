package main

import (
	"image"
	"image/draw"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const textModulePadX = 6

type Module interface {
	Start(markDirty func())
	Close()
	Width(face font.Face) int
	Draw(dst draw.Image, rect image.Rectangle, face font.Face)
}

type ClickableModule interface {
	Module
	OnClick(button uint32)
}

type CommandModule interface {
	Module
	HandleCommand(cmd string) bool
}

type TextModule struct {
	mu   sync.RWMutex
	text string
}

func (m *TextModule) SetText(text string) {
	m.mu.Lock()
	m.text = text
	m.mu.Unlock()
}

func (m *TextModule) Text() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.text
}

func (m *TextModule) Width(face font.Face) int {
	bounds, _ := font.BoundString(face, m.Text())
	return (bounds.Max.X - bounds.Min.X).Ceil() + 2*textModulePadX
}

func (m *TextModule) Draw(dst draw.Image, rect image.Rectangle, face font.Face) {
	bounds, _ := font.BoundString(face, m.Text())
	drawer := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(argb(0xff, 0xff, 0xff, 0xff)),
		Face: face,
		// Use ink bounds, not advance width, so icon overhang does not eat padding.
		Dot: fixed.P(rect.Min.X+textModulePadX-bounds.Min.X.Floor(), textBaseline(face, rect)),
	}
	drawer.DrawString(m.Text())
}
