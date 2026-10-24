package main

import "image"

// ControlCenterModule is a single bar module that opens the merged
// mic/volume/bluetooth Control Center dropdown.
type ControlCenterModule struct {
	TextModule
	flyout *ControlCenterFlyout
}

func NewControlCenterModule(flyout *ControlCenterFlyout) *ControlCenterModule {
	m := &ControlCenterModule{flyout: flyout}
	m.SetText("\uf205") // Font Awesome toggle-on (Nerd Font)
	return m
}

func (m *ControlCenterModule) Start(markDirty func()) {}

func (m *ControlCenterModule) Close() {}

func (m *ControlCenterModule) HoverRect(rect image.Rectangle, point image.Point) (image.Rectangle, bool) {
	return rect, true
}

func (m *ControlCenterModule) OnClickAt(button uint32, rect image.Rectangle, point image.Point) bool {
	if button != 0x110 {
		return false
	}
	if m.flyout != nil {
		m.flyout.Toggle(rect)
	}
	return true
}
