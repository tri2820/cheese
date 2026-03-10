package main

import "log"

type MicModule struct {
	TextModule
	audio       *AudioService
	unsubscribe func()
}

func NewMicModule(audio *AudioService) *MicModule {
	return &MicModule{
		audio: audio,
	}
}

func (m *MicModule) Start(markDirty func()) {
	m.unsubscribe = m.audio.Subscribe(func(state AudioState) {
		text := micText(state)
		if text == m.Text() {
			return
		}
		m.SetText(text)
		markDirty()
	})
}

func (m *MicModule) Close() {
	if m.unsubscribe != nil {
		m.unsubscribe()
		m.unsubscribe = nil
	}
}

func (m *MicModule) OnClick(button uint32) {
	if button != 0x110 {
		return
	}

	if err := m.audio.ToggleSourceMute(); err != nil {
		log.Printf("Mic toggle error: %v", err)
		return
	}
}

func micText(state AudioState) string {
	if state.SourceMuted {
		return "󰍭"
	}
	return "󰍬"
}
