package main

import (
	"log"
	"os/exec"
	"strings"
)

type WifiModule struct {
	TextModule
	network     *NetworkService
	unsubscribe func()
}

func NewWifiModule(network *NetworkService) *WifiModule {
	return &WifiModule{
		network: network,
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

func (m *WifiModule) OnClick(button uint32) {
	if button != 0x110 {
		return
	}

	cmd := exec.Command("alacritty", "--class=floating", "-e", "nmtui")
	if err := cmd.Start(); err != nil {
		log.Printf("Wifi module click error: %v", err)
		return
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
