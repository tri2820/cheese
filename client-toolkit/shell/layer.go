package shell

import (
	"log"

	"github.com/tri2820/cheese/client-toolkit/surface"
	"github.com/tri2820/cheese/protocols/client"
	"github.com/tri2820/cheese/protocols/wlr_layer_shell_unstable_v1"
)

// LayerSurface represents a layer shell surface (for panels, status bars, wallpapers, etc).
type LayerSurface struct {
	display      *wlr_layer_shell_unstable_v1.ZwlrLayerShellV1
	layerSurface *wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1
	surface      *surface.Surface

	width             int
	height            int
	configureHandlers []func()
	closeHandlers     []func()
}

// LayerPosition is the layer position for a layer surface.
type LayerPosition uint32

const (
	LayerPositionBackground LayerPosition = LayerPosition(wlr_layer_shell_unstable_v1.ZwlrLayerShellV1LayerBackground)
	LayerPositionBottom     LayerPosition = LayerPosition(wlr_layer_shell_unstable_v1.ZwlrLayerShellV1LayerBottom)
	LayerPositionTop        LayerPosition = LayerPosition(wlr_layer_shell_unstable_v1.ZwlrLayerShellV1LayerTop)
	LayerPositionOverlay    LayerPosition = LayerPosition(wlr_layer_shell_unstable_v1.ZwlrLayerShellV1LayerOverlay)
)

// LayerAnchor is the anchor position for a layer surface.
type LayerAnchor uint32

const (
	AnchorTop    LayerAnchor = LayerAnchor(wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1AnchorTop)
	AnchorBottom LayerAnchor = LayerAnchor(wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1AnchorBottom)
	AnchorLeft   LayerAnchor = LayerAnchor(wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1AnchorLeft)
	AnchorRight  LayerAnchor = LayerAnchor(wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1AnchorRight)
)

// LayerConfig configures a new layer surface.
type LayerConfig struct {
	// Layer is the layer position (background, bottom, top, overlay)
	Layer LayerPosition

	// Name is the namespace for the layer surface
	Name string

	// Anchor specifies which edges the surface is anchored to
	Anchor LayerAnchor

	// Width and height (0 means full size in that dimension)
	Width  uint32
	Height uint32

	// ExclusiveZone reserves space for the surface (0 = don't reserve, -1 = remove exclusive zone)
	ExclusiveZone int32

	// Output is the wl_output to display on (nil = compositor chooses one, typically the most recently interacted output)
	Output *client.WlOutput
}

// NewLayer creates a new layer shell surface.
func NewLayer(surf *surface.Surface, layerShell *wlr_layer_shell_unstable_v1.ZwlrLayerShellV1, config LayerConfig) (*LayerSurface, error) {
	layerSurf, err := layerShell.GetLayerSurface(
		surf.WlSurface(),
		config.Output, // output (nil = compositor chooses one)
		wlr_layer_shell_unstable_v1.ZwlrLayerShellV1Layer(config.Layer),
		config.Name,
	)
	if err != nil {
		return nil, err
	}

	l := &LayerSurface{
		display:      layerShell,
		layerSurface: layerSurf,
		surface:      surf,
	}

	// Set up handlers
	l.layerSurface.SetConfigureHandler(l.handleConfigure)
	l.layerSurface.SetClosedHandler(l.handleClosed)

	// Configure the layer surface
	if config.Anchor != 0 {
		if err := l.SetAnchor(config.Anchor); err != nil {
			return nil, err
		}
	}
	if config.Width > 0 || config.Height > 0 {
		if err := l.SetSize(config.Width, config.Height); err != nil {
			return nil, err
		}
	}
	if err := l.SetExclusiveZone(config.ExclusiveZone); err != nil {
		return nil, err
	}

	// Commit to trigger configure event
	if err := surf.Commit(); err != nil {
		return nil, err
	}

	return l, nil
}

// handleConfigure handles layer surface configure events.
func (l *LayerSurface) handleConfigure(ev wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1ConfigureEvent) {
	// Track dimensions
	if ev.Width > 0 {
		l.width = int(ev.Width)
	}
	if ev.Height > 0 {
		l.height = int(ev.Height)
	}

	// Ack configure
	if err := l.layerSurface.AckConfigure(ev.Serial); err != nil {
		log.Printf("failed to ack configure: %v", err)
	}

	for _, fn := range append([]func(){}, l.configureHandlers...) {
		if fn != nil {
			fn()
		}
	}
}

// handleClosed handles layer surface closed events.
func (l *LayerSurface) handleClosed(ev wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1ClosedEvent) {
	if len(l.closeHandlers) == 0 {
		log.Printf("Warning: Layer surface closed, no close handler set")
		return
	}
	for _, fn := range append([]func(){}, l.closeHandlers...) {
		if fn != nil {
			fn()
		}
	}
}

// SetAnchor sets the anchor for the layer surface.
func (l *LayerSurface) SetAnchor(anchor LayerAnchor) error {
	return l.layerSurface.SetAnchor(wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1Anchor(anchor))
}

// SetSize sets the size for the layer surface.
// Use 0 for width/height to let the compositor decide (full width/height).
func (l *LayerSurface) SetSize(width, height uint32) error {
	return l.layerSurface.SetSize(width, height)
}

// SetExclusiveZone sets the exclusive zone.
// Positive values reserve space, 0 = don't reserve, -1 = remove exclusive zone.
func (l *LayerSurface) SetExclusiveZone(zone int32) error {
	return l.layerSurface.SetExclusiveZone(zone)
}

// OnConfigure registers a handler for configure events.
func (l *LayerSurface) OnConfigure(fn func()) {
	if fn == nil {
		return
	}
	l.configureHandlers = append(l.configureHandlers, fn)
}

// OnClose registers a handler for close requests.
func (l *LayerSurface) OnClose(fn func()) {
	if fn == nil {
		return
	}
	l.closeHandlers = append(l.closeHandlers, fn)
}

// Surface returns the surface for this layer.
func (l *LayerSurface) Surface() *surface.Surface {
	return l.surface
}

// WlrLayerSurface returns the underlying layer surface.
func (l *LayerSurface) WlrLayerSurface() *wlr_layer_shell_unstable_v1.ZwlrLayerSurfaceV1 {
	return l.layerSurface
}

// Width returns the current width of the layer surface.
func (l *LayerSurface) Width() int {
	return l.width
}

// Height returns the current height of the layer surface.
func (l *LayerSurface) Height() int {
	return l.height
}

// SetMargin sets the margin from the anchor point.
// Can be called after layer creation to update position dynamically.
func (l *LayerSurface) SetMargin(top, right, bottom, left int32) error {
	if err := l.layerSurface.SetMargin(top, right, bottom, left); err != nil {
		return err
	}
	// Commit the change to apply it to the compositor
	return l.surface.Commit()
}

// Close destroys the layer surface.
func (l *LayerSurface) Close() error {
	return l.layerSurface.Destroy()
}
