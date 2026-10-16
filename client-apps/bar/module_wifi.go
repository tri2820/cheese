package main

import (
	"image"
	"strings"
)

type WifiModule struct {
	TextModule
	network     *NetworkService
	flyout      *WifiFlyout
	unsubscribe func()
}

func NewWifiModule(network *NetworkService, flyout *WifiFlyout) *WifiModule {
	return &WifiModule{
		network: network,
		flyout:  flyout,
	}
}

func (m *WifiModule) Start(markDirty func()) {
	m.unsubscribe = m.network.Subscribe(func(state WifiState) {
		text := wifiText(state)
		if text == m.Text() {
			return
		}
		m.SetText(text)
		markDirty()
	})
}

func (m *WifiModule) Close() {
	if m.unsubscribe != nil {
		m.unsubscribe()
		m.unsubscribe = nil
	}
}

func (m *WifiModule) HoverRect(rect image.Rectangle, point image.Point) (image.Rectangle, bool) {
	return rect, true
}

func (m *WifiModule) OnClickAt(button uint32, rect image.Rectangle, point image.Point) bool {
	if button != 0x110 {
		return false
	}
	if m.flyout != nil {
		m.flyout.Toggle(rect)
	}
	return true
}

func (m *WifiModule) OnClick(button uint32) {
	if button == 0x110 && m.flyout != nil {
		m.flyout.Toggle(image.Rect(0, 0, 0, 0))
	}
}

func wifiIcon(strength uint8, connected bool) string {
	if !connected {
		return "󰤭"
	}
	switch {
	case strength < 20:
		return "󰤯"
	case strength < 40:
		return "󰤟"
	case strength < 60:
		return "󰤢"
	case strength < 80:
		return "󰤥"
	default:
		return "󰤨"
	}
}

func wifiText(state WifiState) string {
	icon := wifiIcon(state.Strength, state.Connected)
	if !state.Connected || state.SSID == "" {
		return icon
	}
	return strings.TrimSpace(state.SSID) + " " + icon
}
