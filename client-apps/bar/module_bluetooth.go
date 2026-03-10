package main

import (
	"log"
	"os/exec"
)

type BluetoothModule struct {
	TextModule
	bluetooth   *BluetoothService
	unsubscribe func()
}

func NewBluetoothModule(bluetooth *BluetoothService) *BluetoothModule {
	return &BluetoothModule{
		bluetooth: bluetooth,
	}
}

func (m *BluetoothModule) Start(markDirty func()) {
	m.unsubscribe = m.bluetooth.Subscribe(func(state BluetoothState) {
		text := bluetoothText(state)
		if text == m.Text() {
			return
		}
		m.SetText(text)
		markDirty()
	})
}

func (m *BluetoothModule) Close() {
	if m.unsubscribe != nil {
		m.unsubscribe()
		m.unsubscribe = nil
	}
}

func (m *BluetoothModule) OnClick(button uint32) {
	if button != 0x110 {
		return
	}

	cmd := exec.Command("alacritty", "--class=floating", "-e", "bluetui")
	if err := cmd.Start(); err != nil {
		log.Printf("Bluetooth module click error: %v", err)
	}
}

func bluetoothText(state BluetoothState) string {
	if state.Powered && state.Connected {
		return "󰂯"
	}
	return "󰂲"
}
