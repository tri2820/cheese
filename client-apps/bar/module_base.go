package main

import (
	"image"
	"image/color"
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

type PointingModule interface {
	Module
	HoverRect(rect image.Rectangle, point image.Point) (image.Rectangle, bool)
	OnClickAt(button uint32, rect image.Rectangle, point image.Point) bool
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
		Src:  image.NewUniform(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}),
		Face: face,
		// Use ink bounds, not advance width, so icon overhang does not eat padding.
		Dot: fixed.P(rect.Min.X+textModulePadX-bounds.Min.X.Floor(), textBaseline(face, rect)),
	}
	drawer.DrawString(m.Text())
}
