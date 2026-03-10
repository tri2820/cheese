package main

import (
	"image"
	"image/draw"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

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
	return font.MeasureString(face, m.Text()).Round()
}

func (m *TextModule) Draw(dst draw.Image, rect image.Rectangle, face font.Face) {
	drawer := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(argb(0xff, 0xff, 0xff, 0xff)),
		Face: face,
		Dot:  fixed.P(rect.Min.X, textBaseline(face, rect)),
	}
	drawer.DrawString(m.Text())
}
