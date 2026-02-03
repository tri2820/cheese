package shell

import (
	"log"
	"os"

	"github.com/tri2820/cheese/protocols/xdg_shell"
	"github.com/tri2820/cheese/client-toolkit/surface"
)

// ToplevelSurface represents an xdg-shell toplevel surface.
type ToplevelSurface struct {
	displayXdg *xdg_shell.XdgWmBase
	xdgSurface *xdg_shell.XdgSurface
	toplevel   *xdg_shell.XdgToplevel
	surface    *surface.Surface

	width     int
	height    int
	onConfigure     func()
	onCloseRequested func()
}

// ToplevelConfig configures a new ToplevelSurface.
type ToplevelConfig struct {
	Title string
	AppId string
	Width int
	Height int
}

// NewToplevel creates a new xdg-shell toplevel surface.
func NewToplevel(surf *surface.Surface, xdgWmBase *xdg_shell.XdgWmBase, config ToplevelConfig) (*ToplevelSurface, error) {
	xdgSurface, err := xdgWmBase.GetXdgSurface(surf.WlSurface())
	if err != nil {
		return nil, err
	}

	toplevel, err := xdgSurface.GetToplevel()
	if err != nil {
		return nil, err
	}

	t := &ToplevelSurface{
		displayXdg: xdgWmBase,
		xdgSurface: xdgSurface,
		toplevel:   toplevel,
		surface:    surf,
		width:      config.Width,
		height:     config.Height,
	}

	// Set up handlers
	t.xdgSurface.SetConfigureHandler(t.handleSurfaceConfigure)
	t.toplevel.SetConfigureHandler(t.handleToplevelConfigure)
	t.toplevel.SetCloseHandler(t.handleToplevelClose)

	// Set title and app id
	if config.Title != "" {
		if err := t.SetTitle(config.Title); err != nil {
			log.Printf("failed to set title: %v", err)
		}
	}
	if config.AppId != "" {
		if err := t.SetAppId(config.AppId); err != nil {
			log.Printf("failed to set app id: %v", err)
		}
	}

	// Commit to trigger configure event
	if err := surf.Commit(); err != nil {
		return nil, err
	}

	return t, nil
}

// handleSurfaceConfigure handles xdg_surface configure events.
func (t *ToplevelSurface) handleSurfaceConfigure(ev xdg_shell.XdgSurfaceConfigureEvent) {
	if err := t.xdgSurface.AckConfigure(ev.Serial); err != nil {
		log.Printf("failed to ack configure: %v", err)
	}

	if t.onConfigure != nil {
		t.onConfigure()
	}
}

// handleToplevelConfigure handles toplevel configure events.
func (t *ToplevelSurface) handleToplevelConfigure(ev xdg_shell.XdgToplevelConfigureEvent) {
	if ev.Width > 0 {
		t.width = int(ev.Width)
	}
	if ev.Height > 0 {
		t.height = int(ev.Height)
	}
}

// handleToplevelClose handles toplevel close requests.
func (t *ToplevelSurface) handleToplevelClose(ev xdg_shell.XdgToplevelCloseEvent) {
	if t.onCloseRequested != nil {
		t.onCloseRequested()
	} else {
		// Default behavior: exit
		log.Println("Toplevel surface close requested")
		os.Exit(0)
	}
}

// SetTitle sets the toplevel title.
func (t *ToplevelSurface) SetTitle(title string) error {
	return t.toplevel.SetTitle(title)
}

// SetAppId sets the app ID.
func (t *ToplevelSurface) SetAppId(appId string) error {
	return t.toplevel.SetAppId(appId)
}

// SetConfigureHandler sets a handler for configure events.
func (t *ToplevelSurface) SetConfigureHandler(fn func()) {
	t.onConfigure = fn
}

// SetCloseHandler sets a handler for close requests.
func (t *ToplevelSurface) SetCloseHandler(fn func()) {
	t.onCloseRequested = fn
}

// Surface returns the underlying surface.
func (t *ToplevelSurface) Surface() *surface.Surface {
	return t.surface
}

// XdgSurface returns the underlying xdg_surface.
func (t *ToplevelSurface) XdgSurface() *xdg_shell.XdgSurface {
	return t.xdgSurface
}

// Toplevel returns the underlying xdg_toplevel.
func (t *ToplevelSurface) Toplevel() *xdg_shell.XdgToplevel {
	return t.toplevel
}

// SetSize sets the minimum and maximum surface size.
func (t *ToplevelSurface) SetSize(minWidth, minHeight, maxWidth, maxHeight int32) error {
	if minWidth > 0 || minHeight > 0 {
		if err := t.toplevel.SetMinSize(minWidth, minHeight); err != nil {
			return err
		}
	}
	if maxWidth > 0 || maxHeight > 0 {
		if err := t.toplevel.SetMaxSize(maxWidth, maxHeight); err != nil {
			return err
		}
	}
	return nil
}

// Width returns the current width of the toplevel surface.
func (t *ToplevelSurface) Width() int {
	return t.width
}

// Height returns the current height of the toplevel surface.
func (t *ToplevelSurface) Height() int {
	return t.height
}
