package main

import (
	"image"
)

type BluetoothModule struct {
	TextModule
	bluetooth   *BluetoothService
	flyout      *BluetoothFlyout
	unsubscribe func()
}

func NewBluetoothModule(bluetooth *BluetoothService, flyout *BluetoothFlyout) *BluetoothModule {
	return &BluetoothModule{
		bluetooth: bluetooth,
		flyout:    flyout,
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

func (m *BluetoothModule) HoverRect(rect image.Rectangle, point image.Point) (image.Rectangle, bool) {
	return rect, true
}

func (m *BluetoothModule) OnClickAt(button uint32, rect image.Rectangle, point image.Point) bool {
	if button != 0x110 {
		return false
	}
	if m.flyout != nil {
		m.flyout.Toggle(rect)
	}
	return true
}

func (m *BluetoothModule) OnClick(button uint32) {
	if button == 0x110 && m.flyout != nil {
		m.flyout.Toggle(image.Rect(0, 0, 0, 0))
	}
}

func bluetoothText(state BluetoothState) string {
	if state.Powered && state.Connected {
		return "󰂯"
	}
	return "󰂲"
}
