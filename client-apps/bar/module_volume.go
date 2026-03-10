package main

import "strconv"

type VolumeModule struct {
	TextModule
	audio       *AudioService
	unsubscribe func()
}

func NewVolumeModule(audio *AudioService) *VolumeModule {
	return &VolumeModule{
		audio: audio,
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
