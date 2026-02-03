package render

import (
	"github.com/tri2820/cheese/client-toolkit/surface"
)

// RenderTarget is a surface that can be rendered to (Window or LayerSurface).
type RenderTarget interface {
	SetConfigureHandler(func())
	Surface() *surface.Surface
	Width() int
	Height() int
}
