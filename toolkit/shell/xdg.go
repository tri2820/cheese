package shell

import (
	"log"
	"os"

	"github.com/tri2820/cheese/protocols/xdg_shell"
	"github.com/tri2820/cheese/toolkit/surface"
)

// Window represents an xdg-shell toplevel window.
type Window struct {
	displayXdg *xdg_shell.XdgWmBase
	xdgSurface *xdg_shell.XdgSurface
	toplevel   *xdg_shell.XdgToplevel
	surface    *surface.Surface

	onConfigure  func()
	onClose      func()
	onCloseRequested func()
}

// Config configures a new Window.
type Config struct {
	Title string
	AppId string
	Width int
	Height int
}

// NewToplevel creates a new xdg-shell toplevel window.
func NewToplevel(surf *surface.Surface, xdgWmBase *xdg_shell.XdgWmBase, config Config) (*Window, error) {
	xdgSurface, err := xdgWmBase.GetXdgSurface(surf.WlSurface())
	if err != nil {
		return nil, err
	}

	toplevel, err := xdgSurface.GetToplevel()
	if err != nil {
		return nil, err
	}

	w := &Window{
		displayXdg: xdgWmBase,
		xdgSurface: xdgSurface,
		toplevel:   toplevel,
		surface:    surf,
	}

	// Set up handlers
	w.xdgSurface.SetConfigureHandler(w.handleSurfaceConfigure)
	w.toplevel.SetConfigureHandler(w.handleToplevelConfigure)
	w.toplevel.SetCloseHandler(w.handleToplevelClose)

	// Set title and app id
	if config.Title != "" {
		if err := w.SetTitle(config.Title); err != nil {
			log.Printf("failed to set title: %v", err)
		}
	}
	if config.AppId != "" {
		if err := w.SetAppId(config.AppId); err != nil {
			log.Printf("failed to set app id: %v", err)
		}
	}

	// Commit to trigger configure event
	if err := surf.Commit(); err != nil {
		return nil, err
	}

	return w, nil
}

// handleSurfaceConfigure handles xdg_surface configure events.
func (w *Window) handleSurfaceConfigure(ev xdg_shell.XdgSurfaceConfigureEvent) {
	if err := w.xdgSurface.AckConfigure(ev.Serial); err != nil {
		log.Printf("failed to ack configure: %v", err)
	}

	if w.onConfigure != nil {
		w.onConfigure()
	}
}

// handleToplevelConfigure handles toplevel configure events.
func (w *Window) handleToplevelConfigure(ev xdg_shell.XdgToplevelConfigureEvent) {
	// Can track width/height changes here if needed
}

// handleToplevelClose handles toplevel close requests.
func (w *Window) handleToplevelClose(ev xdg_shell.XdgToplevelCloseEvent) {
	if w.onCloseRequested != nil {
		w.onCloseRequested()
	} else {
		// Default behavior: exit
		log.Println("Window close requested")
		os.Exit(0)
	}
}

// SetTitle sets the window title.
func (w *Window) SetTitle(title string) error {
	return w.toplevel.SetTitle(title)
}

// SetAppId sets the app ID.
func (w *Window) SetAppId(appId string) error {
	return w.toplevel.SetAppId(appId)
}

// SetConfigureHandler sets a handler for configure events.
func (w *Window) SetConfigureHandler(fn func()) {
	w.onConfigure = fn
}

// SetCloseHandler sets a handler for close requests.
func (w *Window) SetCloseHandler(fn func()) {
	w.onCloseRequested = fn
}

// Surface returns the surface for this window.
func (w *Window) Surface() *surface.Surface {
	return w.surface
}

// XdgSurface returns the underlying xdg_surface.
func (w *Window) XdgSurface() *xdg_shell.XdgSurface {
	return w.xdgSurface
}

// Toplevel returns the underlying xdg_toplevel.
func (w *Window) Toplevel() *xdg_shell.XdgToplevel {
	return w.toplevel
}

// SetSize sets the minimum and maximum window size.
func (w *Window) SetSize(minWidth, minHeight, maxWidth, maxHeight int32) error {
	if minWidth > 0 || minHeight > 0 {
		if err := w.toplevel.SetMinSize(minWidth, minHeight); err != nil {
			return err
		}
	}
	if maxWidth > 0 || maxHeight > 0 {
		if err := w.toplevel.SetMaxSize(maxWidth, maxHeight); err != nil {
			return err
		}
	}
	return nil
}
