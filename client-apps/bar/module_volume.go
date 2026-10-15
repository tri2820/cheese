package main

import (
	"image"
	"log"
	"strconv"
)

type VolumeModule struct {
	TextModule
	audio       *AudioService
	flyout      *VolumeFlyout
	unsubscribe func()
}

func NewVolumeModule(audio *AudioService, flyout *VolumeFlyout) *VolumeModule {
	return &VolumeModule{
		audio:  audio,
		flyout: flyout,
	}
}

func (m *VolumeModule) Start(markDirty func()) {
	m.unsubscribe = m.audio.Subscribe(func(state AudioState) {
		text := volumeText(state)
		if text == m.Text() {
			return
		}
		m.SetText(text)
		markDirty()
	})
}

func (m *VolumeModule) Close() {
	if m.unsubscribe != nil {
		m.unsubscribe()
		m.unsubscribe = nil
	}
}

func (m *VolumeModule) OnClickAt(button uint32, rect image.Rectangle, _ image.Point) bool {
	if button == 0x110 {
		if m.flyout != nil {
			m.flyout.Toggle(rect)
		}
		return true
	}
	if button == 0x111 || button == 0x112 {
		if err := m.audio.ToggleSinkMute(); err != nil {
			log.Printf("Volume toggle error: %v", err)
		}
		return true
	}
	return false
}

func (m *VolumeModule) OnClick(button uint32) {
	if button == 0x110 && m.flyout != nil {
		m.flyout.Toggle(image.Rect(0, 0, 100, barHeight))
	}
}

func volumeIcon(level int, muted bool) string {
	if muted {
		return "󰸈"
	}
	if level < 34 {
		return "󰕿"
	}
	if level < 67 {
		return "󰖀"
	}
	return "󰕾"
}

func volumeText(state AudioState) string {
	icon := volumeIcon(state.SinkVolume, state.SinkMuted)
	if state.SinkMuted {
		return icon
	}
	return strconv.Itoa(state.SinkVolume) + "% " + icon
}
